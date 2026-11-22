package tickets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/files"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// ErrFileNotAttached — файл ещё не привязан ни к одному комментарию заявки.
var ErrFileNotAttached = errors.New("tickets: файл не привязан к заявке")

// FileAccessStore — запросы, нужные авторизаторам файлов (реализуется
// MySQLRepository). Вынесено отдельным интерфейсом, чтобы тестировать
// авторизаторы без MySQL и не расширять основной Repository.
type FileAccessStore interface {
	// TicketIDByAccessTokenHash возвращает id заявки по SHA-256 токену.
	TicketIDByAccessTokenHash(ctx context.Context, tokenHash string) (int64, error)
	// FileBelongsToTicket сообщает, привязан ли файл к комментарию заявки.
	FileBelongsToTicket(ctx context.Context, fileID, ticketID int64) (bool, error)
	// TicketIDByFileID возвращает заявку, к комментарию которой привязан файл.
	TicketIDByFileID(ctx context.Context, fileID int64) (int64, error)
}

// TicketTokenAuthorizer реализует files.TicketTokenAuthorizer (TICKET-13):
// публичный доступ к файлу разрешён только по токену его заявки.
type TicketTokenAuthorizer struct {
	store FileAccessStore
}

// NewTicketTokenAuthorizer создаёт авторизатор публичных файлов.
func NewTicketTokenAuthorizer(store FileAccessStore) *TicketTokenAuthorizer {
	return &TicketTokenAuthorizer{store: store}
}

// TicketIDByToken возвращает id заявки по бессрочному токену.
func (a *TicketTokenAuthorizer) TicketIDByToken(ctx context.Context, token string) (int64, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, files.ErrTokenNotFound
	}
	ticketID, err := a.store.TicketIDByAccessTokenHash(ctx, auth.HashToken(token))
	if err != nil {
		if errors.Is(err, ErrAccessTokenNotFound) || errors.Is(err, ErrNotFound) {
			return 0, files.ErrTokenNotFound
		}
		return 0, err
	}
	return ticketID, nil
}

// FileBelongsToTicketToken проверяет, что файл привязан к заявке этого токена.
func (a *TicketTokenAuthorizer) FileBelongsToTicketToken(ctx context.Context, fileID int64, token string) (bool, error) {
	ticketID, err := a.TicketIDByToken(ctx, token)
	if err != nil {
		if errors.Is(err, files.ErrTokenNotFound) {
			return false, nil
		}
		return false, err
	}
	return a.store.FileBelongsToTicket(ctx, fileID, ticketID)
}

// FileAccessAuthorizer реализует files.FileAccessAuthorizer (TICKET-7):
// сотрудник читает вложения только заявок своего здания, ADMIN — любых;
// медиа БЗ доступны всем авторизованным (PLAN §3.1).
type FileAccessAuthorizer struct {
	repo  Repository
	store FileAccessStore
}

// NewFileAccessAuthorizer создаёт авторизатор авторизованной выдачи файлов.
func NewFileAccessAuthorizer(repo Repository, store FileAccessStore) *FileAccessAuthorizer {
	return &FileAccessAuthorizer{repo: repo, store: store}
}

// CanReadFile проверяет доступ сотрудника к файлу.
func (a *FileAccessAuthorizer) CanReadFile(ctx context.Context, user *auth.User, file *files.File) (bool, error) {
	if user == nil || file == nil {
		return false, nil
	}
	// Медиа статей БЗ доступны всем авторизованным (KB-5).
	if file.Scope == files.ScopeKB {
		return true, nil
	}

	ticketID, err := a.store.TicketIDByFileID(ctx, file.ID)
	if err != nil {
		if errors.Is(err, ErrFileNotAttached) {
			// Файл загружен, но ещё не привязан к комментарию: доступен только
			// своему загрузчику (сценарий «загрузил → создаёт комментарий»).
			return file.UploaderID != nil && *file.UploaderID == user.ID, nil
		}
		return false, err
	}
	ticket, err := a.repo.Get(ctx, ticketID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if user.Role == auth.RoleAdmin {
		return true, nil
	}
	if user.BuildingID == nil {
		return false, nil
	}
	return ticket.BuildingID == *user.BuildingID, nil
}

// FilesAttacher реализует FileAttacher через files.Service (Б3).
type FilesAttacher struct {
	service *files.Service
}

// NewFilesAttacher создаёт адаптер привязки файлов к комментариям.
func NewFilesAttacher(service *files.Service) *FilesAttacher {
	return &FilesAttacher{service: service}
}

// ValidateScope проверяет количество вложений и область до создания комментария.
// Существование файлов и запрет перепривязки проверяет files.Service при attach.
func (a *FilesAttacher) ValidateScope(_ context.Context, fileIDs []int64, scope string) error {
	if len(fileIDs) > files.MaxAttachmentsPerComment {
		return apierr.Validation("Слишком много вложений",
			map[string]any{"file_ids": "не более 5"})
	}
	if scope != string(files.ScopeTicket) {
		return apierr.Validation("Недопустимая область файла",
			map[string]any{"scope": "ожидается ticket"})
	}
	return nil
}

// ValidateAndAttach связывает файлы с комментарием (comments_files, Б3).
func (a *FilesAttacher) ValidateAndAttach(ctx context.Context, fileIDs []int64, commentID int64) error {
	if len(fileIDs) == 0 {
		return nil
	}
	return a.service.AttachToComment(ctx, commentID, fileIDs)
}

// TicketIDByAccessTokenHash находит заявку по SHA-256 токену (TICKET-13).
func (r *MySQLRepository) TicketIDByAccessTokenHash(ctx context.Context, tokenHash string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM tickets
		WHERE access_token_hash = ? AND deleted_at IS NULL
		LIMIT 1`, tokenHash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrAccessTokenNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("tickets: чтение заявки по токену: %w", err)
	}
	return id, nil
}

// FileBelongsToTicket проверяет привязку файла к комментарию заявки.
func (r *MySQLRepository) FileBelongsToTicket(ctx context.Context, fileID, ticketID int64) (bool, error) {
	var exists int
	err := r.db.QueryRowContext(ctx, `
		SELECT 1
		FROM comment_files cf
		JOIN ticket_comments tc ON tc.id = cf.comment_id
		JOIN tickets t ON t.id = tc.ticket_id
		WHERE cf.file_id = ? AND tc.ticket_id = ?
		  AND tc.deleted_at IS NULL AND t.deleted_at IS NULL
		LIMIT 1`, fileID, ticketID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("tickets: проверка файла заявки: %w", err)
	}
	return true, nil
}

// TicketIDByFileID возвращает заявку, к которой привязан файл.
func (r *MySQLRepository) TicketIDByFileID(ctx context.Context, fileID int64) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		SELECT t.id
		FROM comment_files cf
		JOIN ticket_comments tc ON tc.id = cf.comment_id
		JOIN tickets t ON t.id = tc.ticket_id
		WHERE cf.file_id = ?
		  AND tc.deleted_at IS NULL AND t.deleted_at IS NULL
		ORDER BY cf.comment_id
		LIMIT 1`, fileID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrFileNotAttached
	}
	if err != nil {
		return 0, fmt.Errorf("tickets: заявка файла: %w", err)
	}
	return id, nil
}

var (
	_ files.TicketTokenAuthorizer = (*TicketTokenAuthorizer)(nil)
	_ files.FileAccessAuthorizer  = (*FileAccessAuthorizer)(nil)
	_ FileAttacher                = (*FilesAttacher)(nil)
	_ FileAccessStore             = (*MySQLRepository)(nil)
)
