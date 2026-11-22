package files

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MySQLRepository — реализация Repository на MySQL 8 (PLAN §4).
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository создаёт репозиторий файлов.
func NewMySQLRepository(db *sql.DB) *MySQLRepository { return &MySQLRepository{db: db} }

// fileColumns — единый список колонок для чтения (порядок важен для scanFile).
const fileColumns = `id, original_name, stored_path, mime, size_bytes, sha256, scope,
	uploader_id, is_anonymous, linked_at, created_at`

// Insert сохраняет метаданные файла и проставляет f.ID.
func (r *MySQLRepository) Insert(ctx context.Context, f *File) error {
	const q = `INSERT INTO files
		(original_name, stored_path, mime, size_bytes, sha256, scope,
		 uploader_id, is_anonymous, linked_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		f.OriginalName, f.StoredPath, f.MIME, f.SizeBytes, f.SHA256, string(f.Scope),
		f.UploaderID, f.IsAnonymous, f.LinkedAt, f.CreatedAt)
	if err != nil {
		return fmt.Errorf("files: вставка метаданных: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("files: идентификатор вставленной записи: %w", err)
	}
	f.ID = id
	return nil
}

// Get возвращает файл по id; отсутствие — ErrNotFound.
func (r *MySQLRepository) Get(ctx context.Context, id int64) (*File, error) {
	q := `SELECT ` + fileColumns + ` FROM files WHERE id = ?`
	f, err := scanFile(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("files: чтение файла %d: %w", id, err)
	}
	return f, nil
}

// MarkLinked проставляет linked_at файлам, у которых он ещё пуст.
func (r *MySQLRepository) MarkLinked(ctx context.Context, fileIDs []int64, at time.Time) error {
	if len(fileIDs) == 0 {
		return nil
	}
	q := `UPDATE files SET linked_at = ? WHERE linked_at IS NULL AND id IN (` +
		placeholders(len(fileIDs)) + `)`
	args := make([]any, 0, len(fileIDs)+1)
	args = append(args, at)
	for _, id := range fileIDs {
		args = append(args, id)
	}
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("files: отметка привязки: %w", err)
	}
	return nil
}

// AttachToComment связывает файлы с комментарием в одной транзакции.
//
// Правила:
//   - TICKET-14: суммарно не более 5 вложений у комментария (учитываются уже
//     привязанные файлы);
//   - все file_ids должны существовать, иначе ErrNotFound;
//   - пара (comment_id, file_id) уникальна — повторный вызов идемпотентен;
//   - linked_at проставляется только тем файлам, у которых он пуст;
//   - привязывать можно только свежие файлы scope=ticket: уже привязанный
//     файл нельзя «перехватить» другим комментарием даже при угаданном id
//     (иначе аноним с валидным токеном мог бы прочитать чужое вложение).
//
// FK на comments появится в Б5; до этого comment_id проверяется вызывающим
// доменом (Б5).
func (r *MySQLRepository) AttachToComment(ctx context.Context, commentID int64, fileIDs []int64, at time.Time) error {
	if len(fileIDs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("files: начало транзакции привязки: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existing int
	countQuery := `SELECT COUNT(*) FROM comment_files WHERE comment_id = ? AND file_id NOT IN (` +
		placeholders(len(fileIDs)) + `)`
	countArgs := append([]any{commentID}, int64Args(fileIDs)...)
	if err := tx.QueryRowContext(ctx, countQuery, countArgs...).Scan(&existing); err != nil {
		return fmt.Errorf("files: подсчёт вложений комментария: %w", err)
	}
	if existing+len(fileIDs) > MaxAttachmentsPerComment {
		return ErrTooManyAttachments
	}

	// Блокируем строки файлов: параллельная очистка orphan не должна удалить
	// файл между проверкой существования и вставкой связи.
	lockQuery := `SELECT id, scope, linked_at FROM files WHERE id IN (` +
		placeholders(len(fileIDs)) + `) FOR UPDATE`
	rows, err := tx.QueryContext(ctx, lockQuery, int64Args(fileIDs)...)
	if err != nil {
		return fmt.Errorf("files: проверка файлов: %w", err)
	}
	found := 0
	var alreadyLinked []int64
	for rows.Next() {
		var (
			id       int64
			scope    string
			linkedAt sql.NullTime
		)
		if err := rows.Scan(&id, &scope, &linkedAt); err != nil {
			_ = rows.Close()
			return fmt.Errorf("files: чтение файла при привязке: %w", err)
		}
		found++
		// Вложением комментария может быть только файл заявки.
		if Scope(scope) != ScopeTicket {
			_ = rows.Close()
			return ErrNotFound
		}
		if linkedAt.Valid {
			alreadyLinked = append(alreadyLinked, id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("files: проверка файлов: %w", err)
	}
	_ = rows.Close()
	if found != len(fileIDs) {
		return ErrNotFound
	}

	// Уже привязанные файлы допустимы, только если они привязаны к этому же
	// комментарию (повторный вызов идемпотентен).
	if len(alreadyLinked) > 0 {
		ownQuery := `SELECT file_id FROM comment_files WHERE comment_id = ? AND file_id IN (` +
			placeholders(len(alreadyLinked)) + `)`
		ownArgs := append([]any{commentID}, int64Args(alreadyLinked)...)
		ownRows, err := tx.QueryContext(ctx, ownQuery, ownArgs...)
		if err != nil {
			return fmt.Errorf("files: проверка прежних связей файла: %w", err)
		}
		own := make(map[int64]struct{}, len(alreadyLinked))
		for ownRows.Next() {
			var id int64
			if err := ownRows.Scan(&id); err != nil {
				_ = ownRows.Close()
				return fmt.Errorf("files: чтение прежних связей файла: %w", err)
			}
			own[id] = struct{}{}
		}
		if err := ownRows.Err(); err != nil {
			_ = ownRows.Close()
			return fmt.Errorf("files: чтение прежних связей файла: %w", err)
		}
		_ = ownRows.Close()
		for _, id := range alreadyLinked {
			if _, ok := own[id]; !ok {
				return ErrNotFound
			}
		}
	}

	const insert = `INSERT IGNORE INTO comment_files (comment_id, file_id, created_at) VALUES (?, ?, ?)`
	for _, id := range fileIDs {
		if _, err := tx.ExecContext(ctx, insert, commentID, id, at); err != nil {
			return fmt.Errorf("files: вставка связи комментарий-файл: %w", err)
		}
	}
	update := `UPDATE files SET linked_at = ? WHERE linked_at IS NULL AND id IN (` +
		placeholders(len(fileIDs)) + `)`
	args := make([]any, 0, len(fileIDs)+1)
	args = append(args, at)
	args = append(args, int64Args(fileIDs)...)
	if _, err := tx.ExecContext(ctx, update, args...); err != nil {
		return fmt.Errorf("files: отметка привязки: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("files: фиксация привязки: %w", err)
	}
	return nil
}

// OrphanFiles возвращает непривязанные файлы, созданные раньше cutoff.
// Порядок — от старых к новым, чтобы очистка шла равномерно.
func (r *MySQLRepository) OrphanFiles(ctx context.Context, cutoff time.Time, limit int) ([]File, error) {
	if limit <= 0 {
		limit = 100
	}
	q := `SELECT ` + fileColumns + ` FROM files
		WHERE linked_at IS NULL AND created_at < ?
		ORDER BY created_at ASC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, q, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("files: выборка orphan-файлов: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("files: чтение orphan-файла: %w", err)
		}
		out = append(out, *f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("files: чтение orphan-файлов: %w", err)
	}
	return out, nil
}

// Delete удаляет запись файла; связи comment_files уходят каскадом (FK).
func (r *MySQLRepository) Delete(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id); err != nil {
		return fmt.Errorf("files: удаление файла %d: %w", id, err)
	}
	return nil
}

// scanner — общий интерфейс *sql.Row и *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanFile читает строку files в модель.
func scanFile(src scanner) (*File, error) {
	var (
		f         File
		scope     string
		uploader  sql.NullInt64
		linkedAt  sql.NullTime
		createdAt time.Time
	)
	if err := src.Scan(&f.ID, &f.OriginalName, &f.StoredPath, &f.MIME, &f.SizeBytes, &f.SHA256,
		&scope, &uploader, &f.IsAnonymous, &linkedAt, &createdAt); err != nil {
		return nil, err
	}
	f.Scope = Scope(scope)
	if uploader.Valid {
		id := uploader.Int64
		f.UploaderID = &id
	}
	if linkedAt.Valid {
		t := linkedAt.Time
		f.LinkedAt = &t
	}
	f.CreatedAt = createdAt
	return &f, nil
}

// placeholders возвращает «?, ?, ?» для IN (...).
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// int64Args превращает срез идентификаторов в аргументы запроса.
func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
