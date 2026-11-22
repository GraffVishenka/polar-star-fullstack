package tickets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ticketColumns — единый набор полей заявки с раскрытыми именами.
const ticketColumns = `
	t.id, t.building_id, b.name, t.applicant_name, t.cabinet,
	t.problem_type_id, pt.name, t.description, t.deadline,
	t.executor_id, u.full_name, t.status, t.completed_at,
	t.created_at, t.updated_at, t.deleted_at, t.access_token_hash,
	t.created_by_user_id`

// ticketFrom — общие JOIN'ы. problem_types обязателен (FK), buildings тоже;
// исполнитель может отсутствовать.
const ticketFrom = `
	FROM tickets t
	JOIN buildings b ON b.id = t.building_id
	JOIN problem_types pt ON pt.id = t.problem_type_id
	LEFT JOIN users u ON u.id = t.executor_id`

// CreateParams — данные INSERT заявки.
type CreateParams struct {
	BuildingID      int64
	ApplicantName   string
	Cabinet         string
	ProblemTypeID   int64
	Description     string
	Deadline        *time.Time
	ExecutorID      *int64
	AccessTokenHash *string
	CreatedByUserID *int64
}

// UpdateParams — итоговые значения UPDATE (после проверки прав в Service).
type UpdateParams struct {
	ApplicantName  *string
	Cabinet        *string
	ProblemTypeID  *int64
	Description    *string
	Status         *Status
	ExecutorID     *int64
	ExecutorIDSet  bool
	Deadline       *time.Time
	DeadlineSet    bool
	BuildingID     *int64
	BuildingIDSet  bool
	CompletedAt    *time.Time
	CompletedAtSet bool
}

// ListParams — нормализованные параметры авторизованного списка.
type ListParams struct {
	BuildingID    *int64
	Statuses      []Status
	ExecutorID    *int64
	ExecutorNone  bool
	ProblemTypeID *int64
	DateFrom      *time.Time // UTC, включительно
	DateTo        *time.Time // UTC, не включительно
	Search        string
	Sort          string
	Order         string
	Page          int
	Size          int
}

// CommentParams — данные INSERT комментария.
type CommentParams struct {
	TicketID     int64
	AuthorUserID *int64
	AuthorName   string
	Body         string
}

// Repository — хранилище заявок, комментариев и справочных данных.
type Repository interface {
	Create(ctx context.Context, params CreateParams) (*Ticket, error)
	Get(ctx context.Context, id int64) (*Ticket, error)
	Update(ctx context.Context, id int64, params UpdateParams) (*Ticket, error)
	SoftDelete(ctx context.Context, id int64) error
	List(ctx context.Context, params ListParams) ([]*Ticket, int, error)
	PublicList(ctx context.Context, buildingID *int64) ([]*Ticket, error)
	PublicBuildings(ctx context.Context) ([]*BuildingCount, error)
	Building(ctx context.Context, id int64) (*Building, error)
	ProblemTypeActive(ctx context.Context, id int64) (bool, error)
	ByAccessTokenHash(ctx context.Context, tokenHash string) (*Ticket, error)
	CreateComment(ctx context.Context, params CommentParams) (*Comment, error)
	GetComment(ctx context.Context, id int64) (*Comment, error)
	ListComments(ctx context.Context, ticketID int64, page, size int) ([]*Comment, int, error)
	DeleteComment(ctx context.Context, id int64) error
	// ListCommentAttachments возвращает вложения для набора комментариев
	// (comment_files + files из Б3); ключ — id комментария.
	ListCommentAttachments(ctx context.Context, commentIDs []int64) (map[int64][]*Attachment, error)
}

// MySQLRepository — реализация Repository на database/sql.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий заявок.
func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

// Create вставляет заявку и возвращает её с раскрытыми именами.
func (r *MySQLRepository) Create(ctx context.Context, params CreateParams) (*Ticket, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO tickets (building_id, applicant_name, cabinet, problem_type_id,
		                     description, deadline, executor_id, status,
		                     access_token_hash, created_by_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'new', ?, ?, UTC_TIMESTAMP(), UTC_TIMESTAMP())`,
		params.BuildingID, params.ApplicantName, params.Cabinet, params.ProblemTypeID,
		params.Description, params.Deadline, params.ExecutorID,
		params.AccessTokenHash, params.CreatedByUserID)
	if err != nil {
		return nil, fmt.Errorf("tickets: создание: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("tickets: создание: %w", err)
	}
	return r.Get(ctx, id)
}

// Get загружает неудалённую заявку по id.
func (r *MySQLRepository) Get(ctx context.Context, id int64) (*Ticket, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+ticketColumns+ticketFrom+`
		WHERE t.id = ? AND t.deleted_at IS NULL
		LIMIT 1`, id)
	return scanTicket(row)
}

// Update применяет изменения и возвращает обновлённую заявку. CompletedAt
// выставляется сервисом (TICKET-10); здание/исполнитель — тоже итоговыми.
func (r *MySQLRepository) Update(ctx context.Context, id int64, params UpdateParams) (*Ticket, error) {
	sets := make([]string, 0, 10)
	args := make([]any, 0, 12)
	sets = append(sets, "updated_at = UTC_TIMESTAMP()")

	if params.ApplicantName != nil {
		sets = append(sets, "applicant_name = ?")
		args = append(args, *params.ApplicantName)
	}
	if params.Cabinet != nil {
		sets = append(sets, "cabinet = ?")
		args = append(args, *params.Cabinet)
	}
	if params.ProblemTypeID != nil {
		sets = append(sets, "problem_type_id = ?")
		args = append(args, *params.ProblemTypeID)
	}
	if params.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *params.Description)
	}
	if params.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, string(*params.Status))
	}
	if params.ExecutorIDSet {
		sets = append(sets, "executor_id = ?")
		args = append(args, params.ExecutorID)
	}
	if params.DeadlineSet {
		sets = append(sets, "deadline = ?")
		args = append(args, params.Deadline)
	}
	if params.BuildingIDSet {
		sets = append(sets, "building_id = ?")
		args = append(args, params.BuildingID)
	}
	if params.CompletedAtSet {
		sets = append(sets, "completed_at = ?")
		args = append(args, params.CompletedAt)
	}
	args = append(args, id)

	if _, err := r.db.ExecContext(ctx, `UPDATE tickets SET `+strings.Join(sets, ", ")+`
		WHERE id = ? AND deleted_at IS NULL`, args...); err != nil {
		return nil, fmt.Errorf("tickets: изменение: %w", err)
	}
	return r.Get(ctx, id)
}

// SoftDelete перемещает заявку в корзину (TICKET-12).
func (r *MySQLRepository) SoftDelete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE tickets
		SET deleted_at = UTC_TIMESTAMP(), updated_at = UTC_TIMESTAMP()
		WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("tickets: удаление в корзину: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("tickets: удаление в корзину: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// List — авторизованный список с фильтрами и пагинацией (TICKET-15).
func (r *MySQLRepository) List(ctx context.Context, params ListParams) ([]*Ticket, int, error) {
	where := []string{"t.deleted_at IS NULL"}
	args := make([]any, 0, 8)

	if params.BuildingID != nil {
		where = append(where, "t.building_id = ?")
		args = append(args, *params.BuildingID)
	}
	if len(params.Statuses) > 0 {
		placeholders := make([]string, len(params.Statuses))
		for i, status := range params.Statuses {
			placeholders[i] = "?"
			args = append(args, string(status))
		}
		where = append(where, "t.status IN ("+strings.Join(placeholders, ", ")+")")
	}
	if params.ExecutorID != nil {
		where = append(where, "t.executor_id = ?")
		args = append(args, *params.ExecutorID)
	}
	if params.ExecutorNone {
		// Ф2: сентинел `executor_id=none` — только заявки без исполнителя.
		where = append(where, "t.executor_id IS NULL")
	}
	if params.ProblemTypeID != nil {
		where = append(where, "t.problem_type_id = ?")
		args = append(args, *params.ProblemTypeID)
	}
	if params.DateFrom != nil {
		where = append(where, "t.created_at >= ?")
		args = append(args, *params.DateFrom)
	}
	if params.DateTo != nil {
		where = append(where, "t.created_at < ?")
		args = append(args, *params.DateTo)
	}
	if params.Search != "" {
		where = append(where, "(t.applicant_name LIKE ? OR t.cabinet LIKE ?)")
		like := "%" + params.Search + "%"
		args = append(args, like, like)
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tickets t`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("tickets: подсчёт списка: %w", err)
	}

	page, size := params.Page, params.Size
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	// Сортировка — только белый список колонок/направлений (без подстановки
	// пользовательского ввода в SQL).
	column := "t.created_at"
	if params.Sort == "deadline" {
		column = "t.deadline"
	}
	direction := "DESC"
	if params.Order == "asc" {
		direction = "ASC"
	}
	query := `SELECT ` + ticketColumns + ticketFrom + whereSQL +
		` ORDER BY ` + column + ` ` + direction + `, t.id ` + direction +
		` LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx, query, append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, fmt.Errorf("tickets: список: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*Ticket, 0, size)
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("tickets: чтение списка: %w", err)
	}
	return items, total, nil
}

// PublicList — заявки трёх публичных статусов по одному или всем активным
// зданиям (TICKET-5). Описание и комментарии в публичные поля не попадают.
func (r *MySQLRepository) PublicList(ctx context.Context, buildingID *int64) ([]*Ticket, error) {
	where := []string{
		"t.deleted_at IS NULL",
		"b.is_active = 1",
		"t.status IN ('new', 'in_progress', 'deferred')",
	}
	args := make([]any, 0, 1)
	if buildingID != nil {
		where = append(where, "t.building_id = ?")
		args = append(args, *buildingID)
	}
	query := `SELECT ` + ticketColumns + ticketFrom +
		` WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY t.created_at DESC, t.id DESC LIMIT 500`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tickets: публичный список: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*Ticket, 0, 64)
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, ticket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tickets: чтение публичного списка: %w", err)
	}
	return items, nil
}

// PublicBuildings — активные здания со счётчиками new/in_progress/deferred
// (TICKET-6). done и корзина не учитываются.
func (r *MySQLRepository) PublicBuildings(ctx context.Context) ([]*BuildingCount, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.name,
		       COUNT(CASE WHEN t.status = 'new' THEN 1 END),
		       COUNT(CASE WHEN t.status = 'in_progress' THEN 1 END),
		       COUNT(CASE WHEN t.status = 'deferred' THEN 1 END)
		FROM buildings b
		LEFT JOIN tickets t
		       ON t.building_id = b.id
		      AND t.deleted_at IS NULL
		      AND t.status IN ('new', 'in_progress', 'deferred')
		WHERE b.is_active = 1
		GROUP BY b.id, b.name
		ORDER BY b.name, b.id`)
	if err != nil {
		return nil, fmt.Errorf("tickets: публичные здания: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*BuildingCount, 0, 8)
	for rows.Next() {
		var item BuildingCount
		if err := rows.Scan(&item.ID, &item.Name, &item.New, &item.InProgress, &item.Deferred); err != nil {
			return nil, fmt.Errorf("tickets: чтение публичных зданий: %w", err)
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tickets: публичные здания: %w", err)
	}
	return items, nil
}

// Building возвращает здание (существование/активность/имя).
func (r *MySQLRepository) Building(ctx context.Context, id int64) (*Building, error) {
	var (
		building Building
		active   int
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, is_active FROM buildings WHERE id = ? LIMIT 1`, id).
		Scan(&building.ID, &building.Name, &active)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrBuildingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tickets: чтение здания: %w", err)
	}
	building.IsActive = active != 0
	return &building, nil
}

// ProblemTypeActive сообщает, существует ли вид проблемы и активен ли он.
func (r *MySQLRepository) ProblemTypeActive(ctx context.Context, id int64) (bool, error) {
	var active int
	err := r.db.QueryRowContext(ctx, `SELECT is_active FROM problem_types WHERE id = ? LIMIT 1`, id).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrProblemTypeNotFound
	}
	if err != nil {
		return false, fmt.Errorf("tickets: чтение вида проблемы: %w", err)
	}
	return active != 0, nil
}

// ByAccessTokenHash ищет заявку по SHA-256 токену (TICKET-13).
func (r *MySQLRepository) ByAccessTokenHash(ctx context.Context, tokenHash string) (*Ticket, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+ticketColumns+ticketFrom+`
		WHERE t.access_token_hash = ? AND t.deleted_at IS NULL
		LIMIT 1`, tokenHash)
	ticket, err := scanTicket(row)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrAccessTokenNotFound
	}
	return ticket, err
}

// CreateComment вставляет комментарий (TICKET-14).
func (r *MySQLRepository) CreateComment(ctx context.Context, params CommentParams) (*Comment, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO ticket_comments (ticket_id, author_user_id, author_name, body, created_at)
		VALUES (?, ?, ?, ?, UTC_TIMESTAMP())`,
		params.TicketID, params.AuthorUserID, params.AuthorName, params.Body)
	if err != nil {
		return nil, fmt.Errorf("tickets: создание комментария: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("tickets: создание комментария: %w", err)
	}
	return r.GetComment(ctx, id)
}

// GetComment загружает неудалённый комментарий.
func (r *MySQLRepository) GetComment(ctx context.Context, id int64) (*Comment, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, ticket_id, author_user_id, author_name, body, created_at, deleted_at
		FROM ticket_comments
		WHERE id = ? AND deleted_at IS NULL
		LIMIT 1`, id)
	return scanComment(row)
}

// ListComments — плоский список комментариев заявки по 50 на страницу.
func (r *MySQLRepository) ListComments(ctx context.Context, ticketID int64, page, size int) ([]*Comment, int, error) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 50
	}
	if size > 100 {
		size = 100
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ticket_comments
		WHERE ticket_id = ? AND deleted_at IS NULL`, ticketID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("tickets: подсчёт комментариев: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, ticket_id, author_user_id, author_name, body, created_at, deleted_at
		FROM ticket_comments
		WHERE ticket_id = ? AND deleted_at IS NULL
		ORDER BY created_at ASC, id ASC
		LIMIT ? OFFSET ?`, ticketID, size, (page-1)*size)
	if err != nil {
		return nil, 0, fmt.Errorf("tickets: список комментариев: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*Comment, 0, size)
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("tickets: чтение комментариев: %w", err)
	}
	return items, total, nil
}

// DeleteComment удаляет комментарий физически — используется для отката, если
// FileAttacher не смог привязать вложения (комментарий без файлов не остаётся).
func (r *MySQLRepository) DeleteComment(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM ticket_comments WHERE id = ?`, id); err != nil {
		return fmt.Errorf("tickets: откат комментария: %w", err)
	}
	return nil
}

// ListCommentAttachments возвращает вложения комментариев (TICKET-14).
// Один запрос на страницу комментариев; файлы области kb к комментариям не
// привязываются, но фильтр scope не вредит.
func (r *MySQLRepository) ListCommentAttachments(ctx context.Context, commentIDs []int64) (map[int64][]*Attachment, error) {
	result := make(map[int64][]*Attachment, len(commentIDs))
	if len(commentIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(commentIDs))
	args := make([]any, len(commentIDs))
	for i, id := range commentIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `
		SELECT cf.comment_id, f.id, f.original_name, f.mime, f.size_bytes, f.scope, f.created_at
		FROM comment_files cf
		JOIN files f ON f.id = cf.file_id
		WHERE cf.comment_id IN (` + strings.Join(placeholders, ", ") + `)
		ORDER BY cf.comment_id, f.id`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("tickets: вложения комментариев: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			commentID  int64
			attachment Attachment
		)
		if err := rows.Scan(&commentID, &attachment.ID, &attachment.Filename, &attachment.MimeType,
			&attachment.Size, &attachment.Scope, &attachment.CreatedAt); err != nil {
			return nil, fmt.Errorf("tickets: чтение вложения: %w", err)
		}
		result[commentID] = append(result[commentID], &attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tickets: вложения комментариев: %w", err)
	}
	return result, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTicket(row rowScanner) (*Ticket, error) {
	var (
		ticket      Ticket
		status      string
		deadline    sql.NullTime
		executorID  sql.NullInt64
		executorNm  sql.NullString
		completedAt sql.NullTime
		deletedAt   sql.NullTime
		accessHash  sql.NullString
		createdBy   sql.NullInt64
	)
	err := row.Scan(
		&ticket.ID, &ticket.BuildingID, &ticket.BuildingName,
		&ticket.ApplicantName, &ticket.Cabinet, &ticket.ProblemTypeID,
		&ticket.ProblemTypeName, &ticket.Description, &deadline,
		&executorID, &executorNm, &status, &completedAt,
		&ticket.CreatedAt, &ticket.UpdatedAt, &deletedAt, &accessHash, &createdBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tickets: чтение заявки: %w", err)
	}
	ticket.Status = Status(status)
	if deadline.Valid {
		ticket.Deadline = &deadline.Time
	}
	if executorID.Valid {
		ticket.ExecutorID = &executorID.Int64
	}
	if executorNm.Valid {
		ticket.ExecutorName = &executorNm.String
	}
	if completedAt.Valid {
		ticket.CompletedAt = &completedAt.Time
	}
	if deletedAt.Valid {
		ticket.DeletedAt = &deletedAt.Time
	}
	if accessHash.Valid {
		ticket.AccessTokenHash = &accessHash.String
	}
	if createdBy.Valid {
		ticket.CreatedByUserID = &createdBy.Int64
	}
	return &ticket, nil
}

func scanComment(row rowScanner) (*Comment, error) {
	var (
		comment  Comment
		authorID sql.NullInt64
		deleted  sql.NullTime
	)
	err := row.Scan(&comment.ID, &comment.TicketID, &authorID, &comment.AuthorName,
		&comment.Body, &comment.CreatedAt, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCommentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tickets: чтение комментария: %w", err)
	}
	if authorID.Valid {
		comment.AuthorUserID = &authorID.Int64
	}
	if deleted.Valid {
		comment.DeletedAt = &deleted.Time
	}
	return &comment, nil
}
