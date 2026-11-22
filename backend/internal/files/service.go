package files

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Repository — хранилище метаданных файлов (таблицы files и comment_files).
// Интерфейс позволяет тестировать сервис и handler'ы без MySQL (в окружении
// разработки БД нет, docs/ENVIRONMENT.md).
type Repository interface {
	// Insert сохраняет метаданные и проставляет File.ID.
	Insert(ctx context.Context, f *File) error
	// Get возвращает файл по id; отсутствие — ErrNotFound.
	Get(ctx context.Context, id int64) (*File, error)
	// MarkLinked проставляет linked_at файлам, у которых он ещё пуст.
	MarkLinked(ctx context.Context, fileIDs []int64, at time.Time) error
	// AttachToComment связывает файлы с комментарием и помечает их
	// привязанными. Проверяет TICKET-14 (не более 5 вложений) и наличие файлов.
	AttachToComment(ctx context.Context, commentID int64, fileIDs []int64, at time.Time) error
	// OrphanFiles возвращает непривязанные файлы, созданные раньше cutoff.
	OrphanFiles(ctx context.Context, cutoff time.Time, limit int) ([]File, error)
	// Delete удаляет запись файла (связи comment_files удаляются каскадом).
	Delete(ctx context.Context, id int64) error
}

// TicketTokenAuthorizer — проверка access_token анонимной заявки (TICKET-13).
//
// Реализацию подключает Б5 (у него появляются таблицы tickets/comments).
// Пока авторизатор не задан, публичные файловые маршруты отвечают 404, чтобы не
// раскрывать существование файлов.
type TicketTokenAuthorizer interface {
	// TicketIDByToken возвращает id заявки по бессрочному токену. Если токена
	// нет — ErrTokenNotFound.
	TicketIDByToken(ctx context.Context, token string) (int64, error)
	// FileBelongsToTicketToken сообщает, привязан ли файл к комментарию заявки
	// с этим токеном. Чужой/битый токен и чужая заявка одинаково дают false.
	FileBelongsToTicketToken(ctx context.Context, fileID int64, token string) (bool, error)
}

// FileAccessAuthorizer — проверка доступа сотрудника к файлу заявки
// (TICKET-7: USER видит только заявки своего здания, ADMIN — любые).
//
// Файлы области kb доступны всем авторизованным (как и сама БЗ, PLAN §3.1) и
// этой проверке не подлежат. Пока авторизатор не задан (до Б5), файлы заявок
// доступны любому авторизованному — контракт `GET /api/files/{id}` (USER/ADMIN).
// Б5 обязана подключить реализацию через SetFileAccessAuthorizer, иначе
// сохраняется возможность прочитать вложение чужой заявки по id (NFR-2).
type FileAccessAuthorizer interface {
	// CanReadFile сообщает, вправе ли сотрудник читать файл. Ответ false
	// превращается в 404, чтобы не раскрывать существование файла.
	CanReadFile(ctx context.Context, user *auth.User, file *File) (bool, error)
}

// Options — зависимости сервиса.
type Options struct {
	// Clock — источник времени (NFR-7: без time.Now() в бизнес-логике).
	Clock  platform.Clock
	Logger *slog.Logger
	// MaxUploadBytes — общий предел размера из конфигурации (MAX_UPLOAD_BYTES).
	// 0 — использовать только лимиты типа файла.
	MaxUploadBytes int64
	// TicketAuthorizer — проверка токена анонимной заявки (реализация Б5).
	TicketAuthorizer TicketTokenAuthorizer
	// FileAccessAuthorizer — проверка доступа сотрудника к файлу заявки
	// (реализация Б5: здание заявки, ADMIN — любое).
	FileAccessAuthorizer FileAccessAuthorizer
}

// Service — бизнес-логика файлов: приём, сохранение метаданных, выдача,
// привязка к комментариям.
type Service struct {
	repo       Repository
	store      *Storage
	clock      platform.Clock
	logger     *slog.Logger
	maxUpload  int64
	authorizer TicketTokenAuthorizer
	access     FileAccessAuthorizer
}

// NewService создаёт сервис файлов.
func NewService(repo Repository, store *Storage, opts Options) *Service {
	if opts.Clock == nil {
		opts.Clock = platform.RealClock{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Service{
		repo:       repo,
		store:      store,
		clock:      opts.Clock,
		logger:     opts.Logger,
		maxUpload:  opts.MaxUploadBytes,
		authorizer: opts.TicketAuthorizer,
		access:     opts.FileAccessAuthorizer,
	}
}

// SetTicketAuthorizer подключает проверку токенов заявки (вызывает Б5 после
// инициализации tickets-домена; до этого публичная выдача отдаёт 404).
func (s *Service) SetTicketAuthorizer(a TicketTokenAuthorizer) { s.authorizer = a }

// SetFileAccessAuthorizer подключает проверку доступа сотрудника к файлам
// заявок (TICKET-7). Без неё файлы заявок доступны любому авторизованному.
func (s *Service) SetFileAccessAuthorizer(a FileAccessAuthorizer) { s.access = a }

// MaxUploadBytes — общий предел размера тела загрузки (для MaxBytesReader).
func (s *Service) MaxUploadBytes() int64 { return s.maxUpload }

// PendingUpload — принятый на диск файл до сохранения метаданных.
//
// Двухфазность нужна из-за multipart: поле scope может прийти после файла,
// поэтому поток пишется на диск сразу, а запись в БД делается после разбора
// всех частей формы.
type PendingUpload struct {
	staged       *Staged
	originalName string
}

// Discard удаляет принятый файл, если он так и не был сохранён.
func (p *PendingUpload) Discard() {
	if p == nil {
		return
	}
	p.staged.Discard()
}

// Upload принимает поток файла: определяет MIME по сигнатуре, применяет лимиты
// Б3 и пишет содержимое на диск. Ошибки: ErrUnsupportedMedia (415),
// *SizeLimitError (413).
func (s *Service) Upload(ctx context.Context, src io.Reader, originalName string) (*PendingUpload, error) {
	if err := ctx.Err(); err != nil {
		return nil, apierr.Internal(err)
	}
	staged, err := s.store.Stage(src, s.maxUpload)
	if err != nil {
		return nil, mapUploadError(err)
	}
	return &PendingUpload{staged: staged, originalName: SafeOriginalName(originalName)}, nil
}

// UploadMeta — параметры сохранения файла.
type UploadMeta struct {
	Scope Scope
	// UploaderID — автор загрузки; nil для анонима (TICKET-13).
	UploaderID  *int64
	IsAnonymous bool
}

// Save переносит принятый файл в итоговый путь и записывает метаданные.
//
// Для scope=kb файл сразу помечается привязанным: статья ссылается на него по
// URL `/api/files/{id}` (KB-5), и он не должен исчезнуть как orphan, пока
// автор дописывает текст. Вложения заявок остаются linked_at=NULL до
// AttachToComment (TICKET-14) и попадают в часовую очистку только через сутки.
func (s *Service) Save(ctx context.Context, pending *PendingUpload, meta UploadMeta) (*File, error) {
	if pending == nil || pending.staged == nil {
		return nil, apierr.Internal(ErrNoUpload)
	}
	if !meta.Scope.Valid() {
		pending.Discard()
		return nil, apierr.Validation("Область файла должна быть ticket или kb",
			map[string]any{"field": "scope", "value": string(meta.Scope)})
	}

	now := s.clock.Now().UTC()
	rel, err := s.store.Commit(pending.staged, now)
	if err != nil {
		pending.Discard()
		return nil, apierr.Internal(err)
	}

	f := &File{
		OriginalName: pending.originalName,
		StoredPath:   rel,
		MIME:         pending.staged.Media.MIME,
		SizeBytes:    pending.staged.Size,
		SHA256:       pending.staged.SHA256,
		Scope:        meta.Scope,
		UploaderID:   meta.UploaderID,
		IsAnonymous:  meta.IsAnonymous,
		CreatedAt:    now,
	}
	if meta.Scope == ScopeKB {
		linkedAt := now
		f.LinkedAt = &linkedAt
	}
	if err := s.repo.Insert(ctx, f); err != nil {
		// Файл на диске без записи в БД недостижим — убираем его сразу.
		_ = s.store.Remove(rel)
		return nil, apierr.Internal(err)
	}
	s.logger.Info("files: файл сохранён",
		"file_id", f.ID, "scope", f.Scope, "mime", f.MIME,
		"size_bytes", f.SizeBytes, "anonymous", f.IsAnonymous)
	return f, nil
}

// SaveAnonymous сохраняет файл, загруженный анонимом по токену заявки
// (TICKET-14: комментарий анонима может содержать вложения).
// До готовности Б5 авторизатор не задан, и маршрут отвечает 404.
func (s *Service) SaveAnonymous(ctx context.Context, token string, pending *PendingUpload) (*File, error) {
	token = strings.TrimSpace(token)
	if s.authorizer == nil || token == "" {
		pending.Discard()
		return nil, apierr.NotFound("Заявка не найдена")
	}
	if _, err := s.authorizer.TicketIDByToken(ctx, token); err != nil {
		pending.Discard()
		if errors.Is(err, ErrTokenNotFound) {
			return nil, apierr.NotFound("Заявка не найдена")
		}
		return nil, apierr.Internal(err)
	}
	return s.Save(ctx, pending, UploadMeta{Scope: ScopeTicket, IsAnonymous: true})
}

// Open возвращает метаданные и открытый файл для выдачи. Файл должен быть
// закрыт вызывающим (handler делает defer).
func (s *Service) Open(ctx context.Context, id int64) (*File, io.ReadSeekCloser, error) {
	f, err := s.fileByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return s.openContent(f)
}

// OpenForUser выдаёт файл сотруднику с проверкой доступа к заявке (TICKET-7):
// файлы области ticket проверяются FileAccessAuthorizer (реализация Б5 —
// здание заявки), медиа БЗ доступны всем авторизованным (PLAN §3.1).
func (s *Service) OpenForUser(ctx context.Context, user *auth.User, id int64) (*File, io.ReadSeekCloser, error) {
	f, err := s.fileByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if f.Scope == ScopeTicket && s.access != nil && user != nil {
		allowed, aerr := s.access.CanReadFile(ctx, user, f)
		if aerr != nil {
			return nil, nil, apierr.Internal(aerr)
		}
		if !allowed {
			// 404 вместо 403: существование чужого вложения не раскрывается.
			return nil, nil, apierr.NotFound("Файл не найден")
		}
	}
	return s.openContent(f)
}

// fileByID читает метаданные файла и переводит ошибки репозитория в коды API.
func (s *Service) fileByID(ctx context.Context, id int64) (*File, error) {
	f, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, mapRepoError(err)
	}
	return f, nil
}

// openContent открывает файл хранилища по метаданным.
func (s *Service) openContent(f *File) (*File, io.ReadSeekCloser, error) {
	fh, _, err := s.store.Open(f.StoredPath)
	if err != nil {
		if errors.Is(err, ErrUnsafePath) {
			s.logger.Error("files: недопустимый путь хранения в БД",
				"file_id", f.ID, "stored_path", f.StoredPath)
			return nil, nil, apierr.Internal(err)
		}
		// Запись есть, файла нет — для клиента это 404, детали только в лог.
		s.logger.Error("files: файл отсутствует на диске", "file_id", f.ID, "error", err)
		return nil, nil, apierr.NotFound("Файл не найден")
	}
	return f, fh, nil
}

// OpenPublic открывает файл по токену анонимной заявки (TICKET-13): доступен
// только файл, привязанный к комментарию этой заявки. Любой чужой/битый токен
// даёт 404, чтобы не раскрывать существование файла (NFR-2).
func (s *Service) OpenPublic(ctx context.Context, id int64, token string) (*File, io.ReadSeekCloser, error) {
	token = strings.TrimSpace(token)
	if s.authorizer == nil || token == "" {
		return nil, nil, apierr.NotFound("Файл не найден")
	}
	ok, err := s.authorizer.FileBelongsToTicketToken(ctx, id, token)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return nil, nil, apierr.NotFound("Файл не найден")
		}
		return nil, nil, apierr.Internal(err)
	}
	if !ok {
		return nil, nil, apierr.NotFound("Файл не найден")
	}
	return s.Open(ctx, id)
}

// AttachToComment привязывает до 5 файлов к комментарию заявки (TICKET-14).
// Вызывает Б5 после создания комментария; повторный вызов идемпотентен по паре
// (comment_id, file_id).
func (s *Service) AttachToComment(ctx context.Context, commentID int64, fileIDs []int64) error {
	if commentID <= 0 {
		return apierr.Validation("Не указан комментарий", map[string]any{"field": "comment_id"})
	}
	ids := NormalizeIDs(fileIDs)
	if len(ids) == 0 {
		return apierr.Validation("Не переданы вложения", map[string]any{"field": "file_ids"})
	}
	if len(ids) > MaxAttachmentsPerComment {
		return apierr.Validation("К комментарию можно приложить не более 5 файлов",
			map[string]any{"field": "file_ids", "max": MaxAttachmentsPerComment})
	}
	if err := s.repo.AttachToComment(ctx, commentID, ids, s.clock.Now().UTC()); err != nil {
		return mapRepoError(err)
	}
	return nil
}

// MarkLinked помечает файлы использованными, не создавая связей (например,
// медиа статьи БЗ после сохранения узла — KB-5).
func (s *Service) MarkLinked(ctx context.Context, fileIDs []int64) error {
	ids := NormalizeIDs(fileIDs)
	if len(ids) == 0 {
		return nil
	}
	if err := s.repo.MarkLinked(ctx, ids, s.clock.Now().UTC()); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

// NormalizeIDs убирает дубли и некорректные идентификаторы.
func NormalizeIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// mapUploadError переводит ошибки приёма потока в коды контракта:
// неподдерживаемый формат — 415, превышение лимита — 413.
func mapUploadError(err error) error {
	var limitErr *SizeLimitError
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, ErrUnsupportedMedia):
		return apierr.New(http.StatusUnsupportedMediaType, apierr.CodeUnsupportedMedia,
			"Формат файла не поддерживается: разрешены jpeg, png, gif, webp, mp4, webm")
	case errors.As(err, &limitErr):
		return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge,
			"Файл слишком большой").
			WithDetails(map[string]any{
				"limit_bytes": limitErr.Limit,
				"size_bytes":  limitErr.Size,
			})
	case errors.As(err, &maxErr):
		return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge,
			"Файл слишком большой").WithDetail("limit_bytes", maxErr.Limit)
	default:
		return apierr.Internal(err)
	}
}

// mapRepoError переводит ошибки репозитория в коды контракта.
func mapRepoError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound("Файл не найден")
	case errors.Is(err, ErrTooManyAttachments):
		return apierr.Validation("К комментарию можно приложить не более 5 файлов",
			map[string]any{"field": "file_ids", "max": MaxAttachmentsPerComment})
	default:
		return apierr.Internal(err)
	}
}
