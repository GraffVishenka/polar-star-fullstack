package kb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MySQLRepository — хранилище дерева БЗ на MySQL 8.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий базы знаний.
func NewMySQLRepository(db *sql.DB) *MySQLRepository { return &MySQLRepository{db: db} }

// nodeColumns — единый список колонок для чтения (порядок важен для scanNode).
const nodeColumns = `id, title, content_html, parent_id, sort_order, is_hidden_for_anon,
	created_by, updated_by, created_at, updated_at, deleted_at`

// List возвращает все узлы вне корзины. Сортировка окончательная делается при
// построении дерева (sort_order, title, id на каждом уровне), здесь — стабильный
// порядок для предсказуемого результата.
func (r *MySQLRepository) List(ctx context.Context) ([]Node, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+nodeColumns+`
		FROM kb_nodes
		WHERE deleted_at IS NULL
		ORDER BY parent_id IS NOT NULL, parent_id, sort_order, title, id`)
	if err != nil {
		return nil, fmt.Errorf("kb: список узлов: %w", err)
	}
	defer func() { _ = rows.Close() }()

	nodes := make([]Node, 0, 64)
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kb: чтение узлов: %w", err)
	}
	return nodes, nil
}

// Get возвращает живой узел; удалённый в корзину неотличим от отсутствующего.
func (r *MySQLRepository) Get(ctx context.Context, id int64) (*Node, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+nodeColumns+`
		FROM kb_nodes WHERE id = ? AND deleted_at IS NULL`, id)
	node, err := scanNode(row)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("kb: чтение узла %d: %w", id, err)
	}
	return node, nil
}

// ParentStatus сообщает существование узла и признак корзины (KB-2).
func (r *MySQLRepository) ParentStatus(ctx context.Context, id int64) (bool, bool, error) {
	var deletedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT deleted_at FROM kb_nodes WHERE id = ?`, id).Scan(&deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("kb: статус узла %d: %w", id, err)
	}
	return true, deletedAt.Valid, nil
}

// Create вставляет узел и возвращает его с проставленным id.
func (r *MySQLRepository) Create(ctx context.Context, params CreateParams) (*Node, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO kb_nodes
		(title, content_html, parent_id, sort_order, is_hidden_for_anon,
		 created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, UTC_TIMESTAMP(), UTC_TIMESTAMP())`,
		params.Title, params.ContentHTML, params.ParentID, params.SortOrder,
		boolToInt(params.IsHiddenForAnon), nullableID(params.ActorID), nullableID(params.ActorID))
	if err != nil {
		return nil, fmt.Errorf("kb: создание узла: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("kb: идентификатор созданного узла: %w", err)
	}
	return r.Get(ctx, id)
}

// Update применяет частичные изменения (KB-1, KB-2, KB-6).
func (r *MySQLRepository) Update(ctx context.Context, params UpdateParams) (*Node, error) {
	sets := []string{"updated_at = UTC_TIMESTAMP()"}
	args := make([]any, 0, 8)
	if params.ActorID > 0 {
		sets = append(sets, "updated_by = ?")
		args = append(args, params.ActorID)
	}
	if params.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *params.Title)
	}
	if params.ContentHTML != nil {
		sets = append(sets, "content_html = ?")
		args = append(args, *params.ContentHTML)
	}
	if params.ParentIDSet {
		sets = append(sets, "parent_id = ?")
		args = append(args, params.ParentID)
	}
	if params.SortOrder != nil {
		sets = append(sets, "sort_order = ?")
		args = append(args, *params.SortOrder)
	}
	args = append(args, params.ID)

	res, err := r.db.ExecContext(ctx, `UPDATE kb_nodes SET `+strings.Join(sets, ", ")+`
		WHERE id = ? AND deleted_at IS NULL`, args...)
	if err != nil {
		return nil, fmt.Errorf("kb: изменение узла: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("kb: изменение узла: %w", err)
	}
	if affected == 0 {
		// MySQL считает RowsAffected по фактически изменённым строкам: повторный
		// PATCH с теми же значениями в ту же секунду (updated_at DATETIME) даёт 0.
		// Отличаем «узла нет/в корзине» от «нечего менять».
		if _, getErr := r.Get(ctx, params.ID); getErr != nil {
			return nil, ErrNotFound
		}
	}
	return r.Get(ctx, params.ID)
}

// Move перемещает узел и переиндексирует sort_order соседей с шагом 10 в одной
// транзакции (KB-6). Строки блокируются в порядке id, чтобы параллельные
// перемещения не приводили к взаимоблокировкам.
func (r *MySQLRepository) Move(ctx context.Context, nodeID int64, parentID *int64, beforeNodeID *int64, actorID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("kb: транзакция перемещения: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Блокируем перемещаемый узел и проверяем, что он жив.
	var currentParent sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT parent_id FROM kb_nodes
		WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, nodeID).Scan(&currentParent)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("kb: блокировка перемещаемого узла: %w", err)
	}

	// 2. Проверяем нового родителя: существует, не в корзине, не потомок узла.
	if parentID != nil {
		var deletedAt sql.NullTime
		err = tx.QueryRowContext(ctx, `SELECT deleted_at FROM kb_nodes
			WHERE id = ? FOR UPDATE`, *parentID).Scan(&deletedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrParentNotFound
		}
		if err != nil {
			return fmt.Errorf("kb: блокировка родителя: %w", err)
		}
		if deletedAt.Valid {
			return ErrParentDeleted
		}
		cycle, cerr := isAncestorOrSelfTx(ctx, tx, nodeID, *parentID)
		if cerr != nil {
			return cerr
		}
		if cycle {
			return ErrCycle
		}
	}

	// 3. Блокируем соседей нового родителя в порядке id и собираем их порядок.
	siblings, err := lockSiblings(ctx, tx, parentID)
	if err != nil {
		return err
	}
	ordered, err := orderWithMove(siblings, nodeID, beforeNodeID)
	if err != nil {
		return err
	}

	// 4. Переиндексируем sort_order шагом 10 (10, 20, 30, …). Перемещаемый узел
	// фиксируется отдельным UPDATE ниже (п. 5), чтобы не писать дважды.
	for index, id := range ordered {
		if id == nodeID {
			continue
		}
		newOrder := sortOrderAt(index)
		if old, ok := siblingOrder(siblings, id); ok && old == newOrder {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE kb_nodes SET sort_order = ? WHERE id = ?`, newOrder, id); err != nil {
			return fmt.Errorf("kb: переиндексация sort_order: %w", err)
		}
	}

	// 5. Финально фиксируем родителя и позицию перемещаемого узла.
	nodeOrder := sortOrderAt(indexOfID(ordered, nodeID))
	sets := []string{"parent_id = ?", "sort_order = ?", "updated_at = UTC_TIMESTAMP()"}
	args := []any{parentID, nodeOrder}
	if actorID > 0 {
		sets = append(sets, "updated_by = ?")
		args = append(args, actorID)
	}
	args = append(args, nodeID)
	if _, err := tx.ExecContext(ctx, `UPDATE kb_nodes SET `+strings.Join(sets, ", ")+`
		WHERE id = ? AND deleted_at IS NULL`, args...); err != nil {
		return fmt.Errorf("kb: фиксация перемещения: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("kb: фиксация транзакции перемещения: %w", err)
	}
	return nil
}

// SoftDelete отправляет узел и всё поддерево в корзину (KB-3). Возвращает число
// удалённых потомков без самого узла.
func (r *MySQLRepository) SoftDelete(ctx context.Context, id int64, at time.Time) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("kb: транзакция удаления: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var marker int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM kb_nodes
		WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, id).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("kb: блокировка удаляемого узла: %w", err)
	}

	// Один и тот же recursive CTE: сначала считаем размер поддерева, затем
	// помечаем его целиком.
	const subtree = `WITH RECURSIVE sub AS (
		SELECT id FROM kb_nodes WHERE id = ? AND deleted_at IS NULL
		UNION ALL
		SELECT child.id FROM kb_nodes child JOIN sub ON child.parent_id = sub.id
		WHERE child.deleted_at IS NULL
	)`
	var total int
	if err := tx.QueryRowContext(ctx, subtree+` SELECT COUNT(*) FROM sub`, id).Scan(&total); err != nil {
		return 0, fmt.Errorf("kb: подсчёт поддерева: %w", err)
	}
	if _, err := tx.ExecContext(ctx, subtree+`
		UPDATE kb_nodes SET deleted_at = ?, updated_at = UTC_TIMESTAMP()
		WHERE id IN (SELECT id FROM sub)`, id, at); err != nil {
		return 0, fmt.Errorf("kb: удаление поддерева: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("kb: фиксация удаления: %w", err)
	}
	if total > 0 {
		total--
	}
	return total, nil
}

// SetVisibility переключает видимость корневого раздела (KB-7).
func (r *MySQLRepository) SetVisibility(ctx context.Context, id int64, hidden bool, actorID int64) (*Node, error) {
	set := "updated_at = UTC_TIMESTAMP()"
	args := []any{boolToInt(hidden)}
	sets := []string{"is_hidden_for_anon = ?"}
	if actorID > 0 {
		sets = append(sets, "updated_by = ?")
		args = append(args, actorID)
	}
	args = append(args, id)
	res, err := r.db.ExecContext(ctx, `UPDATE kb_nodes SET `+strings.Join(sets, ", ")+`, `+set+`
		WHERE id = ? AND deleted_at IS NULL`, args...)
	if err != nil {
		return nil, fmt.Errorf("kb: смена видимости: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("kb: смена видимости: %w", err)
	}
	if affected == 0 {
		// См. Update: 0 затронутых строк может означать «значение не изменилось».
		if _, getErr := r.Get(ctx, id); getErr != nil {
			return nil, ErrNotFound
		}
	}
	return r.Get(ctx, id)
}

// Ancestors возвращает путь от корня до узла включительно (хлебные крошки).
func (r *MySQLRepository) Ancestors(ctx context.Context, id int64) ([]Breadcrumb, error) {
	rows, err := r.db.QueryContext(ctx, `WITH RECURSIVE chain AS (
		SELECT id, parent_id, title, 0 AS depth FROM kb_nodes WHERE id = ?
		UNION ALL
		SELECT parent.id, parent.parent_id, parent.title, chain.depth + 1
		FROM kb_nodes parent JOIN chain ON parent.id = chain.parent_id
	) SELECT id, title, depth FROM chain ORDER BY depth DESC`, id)
	if err != nil {
		return nil, fmt.Errorf("kb: путь до узла: %w", err)
	}
	defer func() { _ = rows.Close() }()

	crumbs := make([]Breadcrumb, 0, 8)
	for rows.Next() {
		var (
			item  Breadcrumb
			depth int
		)
		if err := rows.Scan(&item.ID, &item.Title, &depth); err != nil {
			return nil, fmt.Errorf("kb: чтение пути узла: %w", err)
		}
		crumbs = append(crumbs, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kb: чтение пути узла: %w", err)
	}
	if len(crumbs) == 0 {
		return nil, ErrNotFound
	}
	return crumbs, nil
}

// IsAncestorOrSelf сообщает, является ли candidate предком nodeID или самим
// узлом (запрет циклов, KB-2).
func (r *MySQLRepository) IsAncestorOrSelf(ctx context.Context, candidate, nodeID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `WITH RECURSIVE chain AS (
		SELECT id, parent_id FROM kb_nodes WHERE id = ?
		UNION ALL
		SELECT parent.id, parent.parent_id
		FROM kb_nodes parent JOIN chain ON parent.id = chain.parent_id
	) SELECT EXISTS(SELECT 1 FROM chain WHERE id = ?)`, nodeID, candidate).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("kb: проверка предка: %w", err)
	}
	return exists, nil
}

// MaxSortOrder — максимальный sort_order среди живых детей parentID.
func (r *MySQLRepository) MaxSortOrder(ctx context.Context, parentID *int64) (int, error) {
	var maxOrder int
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), 0) FROM kb_nodes
		WHERE parent_id <=> ? AND deleted_at IS NULL`, parentID).Scan(&maxOrder)
	if err != nil {
		return 0, fmt.Errorf("kb: максимальный sort_order: %w", err)
	}
	return maxOrder, nil
}

// MediaCandidates возвращает живые узлы, в content_html которых встречается
// подстрока like (предварительный отбор для проверки медиа БЗ).
func (r *MySQLRepository) MediaCandidates(ctx context.Context, like string) ([]MediaCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, content_html FROM kb_nodes
		WHERE deleted_at IS NULL AND content_html LIKE ?`, like)
	if err != nil {
		return nil, fmt.Errorf("kb: поиск ссылок на медиа: %w", err)
	}
	defer func() { _ = rows.Close() }()

	candidates := make([]MediaCandidate, 0, 4)
	for rows.Next() {
		var item MediaCandidate
		if err := rows.Scan(&item.ID, &item.ContentHTML); err != nil {
			return nil, fmt.Errorf("kb: чтение ссылок на медиа: %w", err)
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kb: чтение ссылок на медиа: %w", err)
	}
	return candidates, nil
}

// NodePublic сообщает, что узел и все его предки живы, а корень раздела не
// скрыт от анонимов (KB-7).
func (r *MySQLRepository) NodePublic(ctx context.Context, id int64) (bool, error) {
	rows, err := r.db.QueryContext(ctx, `WITH RECURSIVE chain AS (
		SELECT id, parent_id, deleted_at, is_hidden_for_anon
		FROM kb_nodes WHERE id = ?
		UNION ALL
		SELECT parent.id, parent.parent_id, parent.deleted_at, parent.is_hidden_for_anon
		FROM kb_nodes parent JOIN chain ON parent.id = chain.parent_id
	) SELECT id, parent_id, deleted_at, is_hidden_for_anon FROM chain`, id)
	if err != nil {
		return false, fmt.Errorf("kb: проверка публичности узла: %w", err)
	}
	defer func() { _ = rows.Close() }()

	rootSeen := false
	hiddenRoot := false
	for rows.Next() {
		var (
			nodeID     int64
			parentID   sql.NullInt64
			deletedAt  sql.NullTime
			hiddenFlag int
		)
		if err := rows.Scan(&nodeID, &parentID, &deletedAt, &hiddenFlag); err != nil {
			return false, fmt.Errorf("kb: чтение цепочки предков: %w", err)
		}
		if deletedAt.Valid {
			return false, nil
		}
		if !parentID.Valid {
			rootSeen = true
			hiddenRoot = hiddenFlag != 0
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("kb: чтение цепочки предков: %w", err)
	}
	return rootSeen && !hiddenRoot, nil
}

// lockSiblings блокирует детей родителя (parentID=nil — корень) в порядке id и
// возвращает их для переиндексации.
func lockSiblings(ctx context.Context, tx *sql.Tx, parentID *int64) ([]sibling, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, sort_order, title FROM kb_nodes
		WHERE parent_id <=> ? AND deleted_at IS NULL
		ORDER BY id FOR UPDATE`, parentID)
	if err != nil {
		return nil, fmt.Errorf("kb: блокировка соседей: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]sibling, 0, 16)
	for rows.Next() {
		var item sibling
		if err := rows.Scan(&item.ID, &item.SortOrder, &item.Title); err != nil {
			return nil, fmt.Errorf("kb: чтение соседей: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kb: чтение соседей: %w", err)
	}
	return items, nil
}

// isAncestorOrSelfTx — проверка предка внутри транзакции (защита от гонки при
// параллельных перемещениях).
func isAncestorOrSelfTx(ctx context.Context, tx *sql.Tx, candidate, nodeID int64) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `WITH RECURSIVE chain AS (
		SELECT id, parent_id FROM kb_nodes WHERE id = ?
		UNION ALL
		SELECT parent.id, parent.parent_id
		FROM kb_nodes parent JOIN chain ON parent.id = chain.parent_id
	) SELECT EXISTS(SELECT 1 FROM chain WHERE id = ?)`, nodeID, candidate).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("kb: проверка предка в транзакции: %w", err)
	}
	return exists, nil
}

// siblingOrder возвращает текущий sort_order соседа.
func siblingOrder(items []sibling, id int64) (int, bool) {
	for _, item := range items {
		if item.ID == id {
			return item.SortOrder, true
		}
	}
	return 0, false
}

// indexOfID возвращает позицию id в списке (или -1).
func indexOfID(ids []int64, id int64) int {
	for index, value := range ids {
		if value == id {
			return index
		}
	}
	return -1
}

// scanner — общий интерфейс *sql.Row и *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanNode читает строку kb_nodes в модель.
func scanNode(src scanner) (*Node, error) {
	var (
		node      Node
		parentID  sql.NullInt64
		createdBy sql.NullInt64
		updatedBy sql.NullInt64
		deletedAt sql.NullTime
	)
	err := src.Scan(&node.ID, &node.Title, &node.ContentHTML, &parentID, &node.SortOrder,
		&node.IsHiddenForAnon, &createdBy, &updatedBy, &node.CreatedAt, &node.UpdatedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if parentID.Valid {
		node.ParentID = &parentID.Int64
	}
	if createdBy.Valid {
		node.CreatedBy = &createdBy.Int64
	}
	if updatedBy.Valid {
		node.UpdatedBy = &updatedBy.Int64
	}
	if deletedAt.Valid {
		node.DeletedAt = &deletedAt.Time
	}
	return &node, nil
}

// nullableID превращает 0 в SQL NULL (автор неизвестен).
func nullableID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// boolToInt — представление bool в MySQL TINYINT(1).
func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
