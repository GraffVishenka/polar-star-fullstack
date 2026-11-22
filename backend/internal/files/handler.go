package files

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// envelopeSlack — запас на multipart-обёртку (границы, имена частей,
// дополнительные поля формы) сверх лимита самого файла.
const envelopeSlack int64 = 1 << 20

// Handler — HTTP-слой файлов (Б3).
type Handler struct {
	svc    *Service
	logger *slog.Logger
	// UserFrom — источник аутентифицированного пользователя. В API это
	// auth.UserFrom; тесты подменяют его, чтобы не поднимать MySQL/сессию.
	UserFrom func(context.Context) (*auth.User, bool)
}

// NewHandler создаёт HTTP-handler'ы файлов.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger, UserFrom: auth.UserFrom}
}

// Upload — POST /api/files (USER/ADMIN): multipart `file` + `scope`.
//
// Б3: MIME определяется по сигнатуре, поэтому `.exe` под видом jpeg получает
// 415, а фото больше 10 МБ — 413.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := h.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	if err := h.limitBody(w, r); err != nil {
		h.fail(w, r, err)
		return
	}

	pending, meta, err := h.receiveMultipart(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if !meta.scope.Valid() {
		pending.Discard()
		h.fail(w, r, apierr.Validation("Область файла должна быть ticket или kb",
			map[string]any{"field": "scope", "value": string(meta.scope)}))
		return
	}

	uploaderID := user.ID
	file, err := h.svc.Save(r.Context(), pending, UploadMeta{
		Scope:      meta.scope,
		UploaderID: &uploaderID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	info := NewFileInfo(file)
	info.URL = "/api/files/" + strconv.FormatInt(file.ID, 10)
	h.writeJSON(w, http.StatusCreated, info)
}

// Get — GET|HEAD /api/files/{id} (USER/ADMIN). Выдача inline с поддержкой
// Range/206 и HEAD. Доступ к файлам заявок дополнительно проверяет
// FileAccessAuthorizer (TICKET-7, реализация Б5); медиа БЗ доступны всем
// авторизованным.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	id, ok := fileIDFromPath(r)
	if !ok {
		h.fail(w, r, apierr.NotFound("Файл не найден"))
		return
	}
	file, body, err := h.svc.OpenForUser(r.Context(), user, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.serveFile(w, r, file, body)
}

// PublicGet — GET|HEAD /api/public/files/{id}?token= (TICKET-13).
// Доступен только файл, привязанный к комментарию заявки с этим токеном;
// любой чужой/битый токен даёт 404 (не раскрываем существование файла).
// Пока Б5 не подключил реализацию TicketTokenAuthorizer — всегда 404.
func (h *Handler) PublicGet(w http.ResponseWriter, r *http.Request) {
	id, ok := fileIDFromPath(r)
	if !ok {
		h.fail(w, r, apierr.NotFound("Файл не найден"))
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		h.fail(w, r, apierr.NotFound("Файл не найден"))
		return
	}
	file, body, err := h.svc.OpenPublic(r.Context(), id, token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.serveFile(w, r, file, body)
}

// PublicUpload — POST /api/public/files?token= (аддитивное расширение
// контракта Б3).
//
// Закрывает пробел ТЗ: TICKET-14/PUB-6 разрешают анониму прикладывать файлы к
// комментарию, но отдельного эндпоинта загрузки для анонима не было. Права
// проверяются по access_token заявки; scope всегда ticket, автор анонимный.
// Rate limit публичных заявок (5 запросов / 10 минут на IP) навешивается
// маршрутом в httpserver.
func (h *Handler) PublicUpload(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		h.fail(w, r, apierr.NotFound("Заявка не найдена"))
		return
	}
	if err := h.limitBody(w, r); err != nil {
		h.fail(w, r, err)
		return
	}

	pending, _, err := h.receiveMultipart(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	file, err := h.svc.SaveAnonymous(r.Context(), token, pending)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	info := NewFileInfo(file)
	info.URL = "/api/public/files/" + strconv.FormatInt(file.ID, 10) + "?token=" + token
	h.writeJSON(w, http.StatusCreated, info)
}

// receivedMeta — поля multipart, влияющие на сохранение.
type receivedMeta struct {
	scope Scope
}

// receiveMultipart разбирает multipart-тело потоком (multipart.Reader, без
// буферизации 200 МБ в память или os.TempDir) и сразу принимает часть `file`
// в хранилище. Поле `scope` может идти после файла, поэтому проверка области
// выполняется вызывающим уже после разбора.
//
// Любая ошибка после приёма файла удаляет временный файл.
func (h *Handler) receiveMultipart(r *http.Request) (*PendingUpload, receivedMeta, error) {
	var meta receivedMeta

	reader, err := r.MultipartReader()
	if err != nil {
		return nil, meta, apierr.BadRequest(apierr.CodeValidation, "Ожидается multipart/form-data")
	}

	var pending *PendingUpload
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			pending.Discard()
			return nil, meta, h.mapBodyError(partErr)
		}

		switch part.FormName() {
		case "file":
			if pending != nil {
				_ = part.Close()
				pending.Discard()
				return nil, meta, apierr.Validation("За один запрос можно загрузить только один файл",
					map[string]any{"field": "file"})
			}
			uploaded, uploadErr := h.svc.Upload(r.Context(), part, part.FileName())
			_ = part.Close()
			if uploadErr != nil {
				return nil, meta, uploadErr
			}
			pending = uploaded
		case "scope":
			// Значение крошечное, но читаем с ограничением: тело под контролем
			// клиента, доверять Content-Length части нельзя.
			value, readErr := io.ReadAll(io.LimitReader(part, 64))
			_ = part.Close()
			if readErr != nil {
				pending.Discard()
				return nil, meta, h.mapBodyError(readErr)
			}
			meta.scope = Scope(strings.TrimSpace(string(value)))
		default:
			// Прочие поля (например, ticket_id из контракта) не влияют на
			// хранение: привязка к заявке идёт через комментарий (Б5).
			_, _ = io.Copy(io.Discard, io.LimitReader(part, 1<<20))
			_ = part.Close()
		}
	}

	if pending == nil {
		return nil, meta, apierr.Validation("Файл не передан", map[string]any{"field": "file"})
	}
	return pending, meta, nil
}

// limitBody ограничивает тело запроса общим лимитом загрузки. Ранний отказ по
// Content-Length экономит трафик при заведомо огромных файлах.
func (h *Handler) limitBody(w http.ResponseWriter, r *http.Request) error {
	limit := h.maxBodyBytes()
	if r.ContentLength > limit {
		return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge,
			"Файл слишком большой").WithDetail("limit_bytes", limit)
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return nil
}

// maxBodyBytes — общий предел тела: MAX_UPLOAD_BYTES (по умолчанию 200 МБ) плюс
// запас на multipart-обёртку.
func (h *Handler) maxBodyBytes() int64 {
	limit := h.svc.MaxUploadBytes()
	if limit <= 0 {
		limit = MaxVideoBytes
	}
	return limit + envelopeSlack
}

// mapBodyError переводит ошибку чтения тела в 413, если сработал MaxBytesReader.
func (h *Handler) mapBodyError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge,
			"Файл слишком большой").WithDetail("limit_bytes", maxErr.Limit)
	}
	return apierr.BadRequest(apierr.CodeValidation, "Некорректное тело запроса")
}

// serveFile выдаёт содержимое через http.ServeContent: он сам обрабатывает
// Range (206 Partial Content), HEAD, Content-Length и If-Range, что нужно для
// перемотки видео до 200 МБ (Б3).
func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, file *File, body io.ReadSeekCloser) {
	defer func() { _ = body.Close() }()

	header := w.Header()
	// MIME берётся из БД (определён по сигнатуре при загрузке) и не угадывается
	// по расширению; nosniff запрещает браузеру переопределять тип.
	header.Set("Content-Type", file.MIME)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Disposition", contentDisposition("inline", file.OriginalName))
	header.Set("Accept-Ranges", "bytes")
	// Файлы приватны: общий кеш не должен сохранять содержимое чужой заявки.
	header.Set("Cache-Control", "private, max-age=0, must-revalidate")

	http.ServeContent(w, r, file.OriginalName, file.CreatedAt, body)
}

// contentDisposition формирует заголовок с корректным кодированием не-ASCII
// имён (RFC 2231) — в именах бывают кириллица и пробелы.
func contentDisposition(disposition, filename string) string {
	if strings.TrimSpace(filename) == "" {
		return disposition
	}
	if value := mime.FormatMediaType(disposition, map[string]string{"filename": filename}); value != "" {
		return value
	}
	return disposition
}

// fileIDFromPath разбирает {id}. Некорректный id — это 404, а не 500: чужие и
// несуществующие идентификаторы неразличимы (NFR-2).
func fileIDFromPath(r *http.Request) (int64, bool) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// writeJSON отдаёт JSON-ответ (формат API, PLAN §3.13).
func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("files: запись JSON-ответа", "error", err)
	}
}

// fail отдаёт ошибку в едином формате и логирует серверные сбои.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) && apiErr.Status >= http.StatusInternalServerError {
		h.logger.Error("files: ошибка запроса",
			"route", r.URL.Path, "status", apiErr.Status, "error", apiErr.Error())
	}
	apierr.Respond(w, r, err)
}
