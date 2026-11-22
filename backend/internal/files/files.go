// Package files реализует блок Б3 «Файлы и медиа»: потоковую загрузку на
// локальный диск (UPLOAD_DIR/YYYY/MM/{uuid}.{ext}), определение MIME по
// сигнатуре (magic bytes) вместо расширения, лимиты «фото ≤ 10 МБ, видео
// ≤ 200 МБ», sha256, метаданные в MySQL, выдачу под проверкой прав с
// поддержкой Range/206, привязку файлов к комментариям заявок (TICKET-14) и
// фоновую очистку «осиротевших» загрузок (linked_at IS NULL старше 24 часов).
package files

import (
	"errors"
	"fmt"
	"time"
)

// Scope — область использования файла (колонка files.scope).
type Scope string

const (
	// ScopeTicket — вложение комментария заявки (TICKET-14), доступ по правам
	// на заявку или по access_token анонима (TICKET-13).
	ScopeTicket Scope = "ticket"
	// ScopeKB — медиа внутри статьи базы знаний (KB-5), доступно всем
	// авторизованным, как и сама БЗ (PLAN §3.1).
	ScopeKB Scope = "kb"
)

// Valid сообщает, что область известна контракту.
func (s Scope) Valid() bool { return s == ScopeTicket || s == ScopeKB }

// MaxAttachmentsPerComment — TICKET-14: не более 5 вложений у комментария.
const MaxAttachmentsPerComment = 5

// File — метаданные загруженного файла (строка таблицы files).
type File struct {
	ID           int64
	OriginalName string
	// StoredPath — путь относительно UPLOAD_DIR: YYYY/MM/{uuid}.{ext}. В БД и
	// наружу отдаётся только он: абсолютные пути не покидают сервер.
	StoredPath string
	// MIME определён по сигнатуре файла (magic bytes), а не по расширению.
	MIME      string
	SizeBytes int64
	SHA256    string
	Scope     Scope
	// UploaderID пуст для анонимной загрузки по токену заявки (TICKET-13).
	UploaderID *int64
	// IsAnonymous — файл загружен анонимом (uploader_id IS NULL).
	IsAnonymous bool
	// LinkedAt пуст, пока файл не привязан: такой файл старше 24 часов
	// удаляется фоновой очисткой.
	LinkedAt  *time.Time
	CreatedAt time.Time
}

// IsLinked сообщает, что файл использован (привязан к комментарию или статье).
func (f *File) IsLinked() bool { return f != nil && f.LinkedAt != nil }

// Ошибки домена. Handler/сервисный слой превращает их в коды контракта
// (PLAN §3.13: 404 NOT_FOUND, 415 UNSUPPORTED_MEDIA_TYPE, 413 PAYLOAD_TOO_LARGE).
var (
	// ErrNotFound — файл отсутствует в БД (или на диске — тогда 404, чтобы не
	// раскрывать состояние хранилища).
	ErrNotFound = errors.New("files: файл не найден")
	// ErrTooManyAttachments — TICKET-14: больше 5 вложений у комментария.
	ErrTooManyAttachments = errors.New("files: превышено число вложений у комментария")
	// ErrTokenNotFound — в интерфейсе TicketTokenAuthorizer: токен заявки не
	// найден. Для клиента это всегда 404 (TICKET-13).
	ErrTokenNotFound = errors.New("files: токен заявки не найден")
	// ErrUnsupportedMedia — сигнатура не совпала ни с одним разрешённым
	// форматом (фото jpeg/png/webp/gif, видео mp4/webm) → 415.
	ErrUnsupportedMedia = errors.New("files: формат файла не поддерживается")
	// ErrUnsafePath — путь хранения выходит за пределы UPLOAD_DIR или содержит
	// недопустимые элементы (защита от path traversal).
	ErrUnsafePath = errors.New("files: недопустимый путь хранения")
	// ErrNoUpload — попытка сохранить незавершённую загрузку (внутренняя ошибка).
	ErrNoUpload = errors.New("files: загрузка не начата")
)

// SizeLimitError — превышен лимит размера (413). Отдельный тип нужен, чтобы
// ответ содержал limit_bytes/size_bytes в details (PLAN §3.13).
type SizeLimitError struct {
	// Limit — разрешённый размер в байтах.
	Limit int64
	// Size — фактически прочитанный размер в байтах.
	Size int64
	// Kind — тип медиа, к которому применён лимит (image/video).
	Kind Kind
}

// Error реализует error.
func (e *SizeLimitError) Error() string {
	return fmt.Sprintf("files: размер %d байт превышает лимит %d байт (%s)", e.Size, e.Limit, e.Kind)
}
