package files

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// testUser — авторизованный сотрудник для handler-тестов.
func testUser() *auth.User { return &auth.User{ID: 7, Role: auth.RoleUser} }

// newTestHandler собирает handler с фейковым репозиторием и подменённым
// источником пользователя (без MySQL и JWT).
func newTestHandler(t *testing.T, opts Options) (*Handler, *fakeRepo, *Storage) {
	t.Helper()
	svc, repo, store := newTestService(t, opts)
	h := NewHandler(svc, discardLogger())
	h.UserFrom = func(context.Context) (*auth.User, bool) { return testUser(), true }
	return h, repo, store
}

// testRouter повторяет регистрацию маршрутов файлов из httpserver
// (префикс /api, чтобы проверять те же URL, что отдаёт API).
func testRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api", func(api chi.Router) {
		api.Post("/files", h.Upload)
		api.Get("/files/{id}", h.Get)
		api.Head("/files/{id}", h.Get)
		api.Get("/public/files/{id}", h.PublicGet)
		api.Head("/public/files/{id}", h.PublicGet)
		api.Post("/public/files", h.PublicUpload)
	})
	return r
}

// multipartUpload формирует multipart-запрос. Порядок частей настраивается:
// браузеры и клиенты вправе прислать файл раньше scope.
func multipartUpload(t *testing.T, target, fieldName, filename string, content []byte, scope string, fileFirst bool) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	writeScope := func() {
		if scope == "" {
			return
		}
		part, err := writer.CreateFormField("scope")
		if err != nil {
			t.Fatalf("CreateFormField: %v", err)
		}
		if _, err := part.Write([]byte(scope)); err != nil {
			t.Fatalf("запись scope: %v", err)
		}
	}
	writeFile := func() {
		part, err := writer.CreateFormFile(fieldName, filename)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("запись файла: %v", err)
		}
	}
	if fileFirst {
		writeFile()
		writeScope()
	} else {
		writeScope()
		writeFile()
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("multipart.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, target, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// decodeFileInfo разбирает ответ загрузки.
func decodeFileInfo(t *testing.T, rec *httptest.ResponseRecorder) FileInfo {
	t.Helper()
	var info FileInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	return info
}

// decodeErrorCode достаёт код ошибки из единого формата (PLAN §3.13).
func decodeErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	return body.Code
}

// POST /api/files: 201 + метаданные, GET выдаёт тот же файл.
func TestHandler_UploadAndGet(t *testing.T) {
	h, repo, _ := newTestHandler(t, Options{})
	router := testRouter(h)
	payload := jpegPayload(300)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "photo.jpg", payload, "ticket", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d (%s)", rec.Code, rec.Body.String())
	}
	info := decodeFileInfo(t, rec)
	if info.ID == 0 || info.Filename != "photo.jpg" || info.MimeType != "image/jpeg" ||
		info.Size != int64(len(payload)) || info.Scope != ScopeTicket {
		t.Fatalf("info = %+v", info)
	}
	if info.URL != "/api/files/"+strconv.FormatInt(info.ID, 10) {
		t.Fatalf("url = %q", info.URL)
	}

	// GET отдаёт содержимое, тип из БД и обязательные заголовки Б3.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/"+strconv.FormatInt(info.ID, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d (%s)", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), payload) {
		t.Fatal("выданное содержимое отличается от загруженного")
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "inline") {
		t.Fatalf("Content-Disposition = %q, want inline", disposition)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q", got)
	}

	stored, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if !strings.HasSuffix(stored.StoredPath, ".jpg") {
		t.Fatalf("stored_path = %q, ожидалось расширение .jpg", stored.StoredPath)
	}
}

// Б3: HEAD и Range/206 на файле (перемотка видео).
func TestHandler_HeadAndRange(t *testing.T) {
	h, _, _ := newTestHandler(t, Options{})
	router := testRouter(h)
	payload := jpegPayload(1000)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "photo.jpg", payload, "ticket", false))
	info := decodeFileInfo(t, rec)
	path := "/api/files/" + strconv.FormatInt(info.ID, 10)

	// HEAD: заголовки как у GET, тела нет.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD вернул тело %d байт", rec.Body.Len())
	}
	if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(payload)) {
		t.Fatalf("HEAD Content-Length = %q, want %d", got, len(payload))
	}

	// Range: 206, Content-Range и запрошенный кусок.
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Range", "bytes=10-19")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("Range status = %d", rec.Code)
	}
	if got, want := rec.Body.Bytes(), payload[10:20]; !bytes.Equal(got, want) {
		t.Fatalf("Range body = %x, want %x", got, want)
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 10-19/"+strconv.Itoa(len(payload)) {
		t.Fatalf("Content-Range = %q", got)
	}
}

// Б3: `.exe` под видом jpeg → 415, файл не сохранён.
func TestHandler_UploadRejectsSpoofedMime(t *testing.T) {
	h, repo, store := newTestHandler(t, Options{})
	payload := append([]byte("MZ\x90\x00"), bytes.Repeat([]byte{0x01}, 256)...)

	rec := httptest.NewRecorder()
	testRouter(h).ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "photo.jpg", payload, "ticket", true))
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d (%s), want 415", rec.Code, rec.Body.String())
	}
	if code := decodeErrorCode(t, rec); code != "UNSUPPORTED_MEDIA_TYPE" {
		t.Fatalf("code = %q", code)
	}
	if repo.count() != 0 {
		t.Fatal("запись создана при отклонённой загрузке")
	}
	assertNoStaging(t, store.Dir())
}

// Б3: фото больше лимита → 413 (лимит из конфигурации в тесте уменьшен).
func TestHandler_UploadRejectsOversized(t *testing.T) {
	h, repo, _ := newTestHandler(t, Options{MaxUploadBytes: 1024})
	payload := jpegPayload(4096)

	rec := httptest.NewRecorder()
	testRouter(h).ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "big.jpg", payload, "ticket", true))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d (%s), want 413", rec.Code, rec.Body.String())
	}
	if code := decodeErrorCode(t, rec); code != "PAYLOAD_TOO_LARGE" {
		t.Fatalf("code = %q", code)
	}
	if repo.count() != 0 {
		t.Fatal("запись создана при превышении лимита")
	}
}

// Без аутентификации загрузка недоступна (маршрут закрыт RequireAuth).
func TestHandler_UploadRequiresAuth(t *testing.T) {
	h := NewHandler(nil, discardLogger())
	h.UserFrom = func(context.Context) (*auth.User, bool) { return nil, false }

	rec := httptest.NewRecorder()
	h.Upload(rec, multipartUpload(t, "/api/files", "file", "photo.jpg", jpegHead, "ticket", true))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// scope обязателен (контракт: required [file, scope]).
func TestHandler_UploadRequiresScope(t *testing.T) {
	h, repo, store := newTestHandler(t, Options{})

	rec := httptest.NewRecorder()
	testRouter(h).ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "photo.jpg", jpegPayload(16), "", true))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d (%s), want 422", rec.Code, rec.Body.String())
	}
	if repo.count() != 0 {
		t.Fatal("запись создана без scope")
	}
	assertNoStaging(t, store.Dir())
}

// Некорректный scope → 422.
func TestHandler_UploadRejectsUnknownScope(t *testing.T) {
	h, _, _ := newTestHandler(t, Options{})

	rec := httptest.NewRecorder()
	testRouter(h).ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "photo.jpg", jpegPayload(16), "profile", true))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

// Расширение клиента не влияет ни на MIME, ни на путь.
func TestHandler_UploadIgnoresClientExtension(t *testing.T) {
	h, repo, _ := newTestHandler(t, Options{})

	rec := httptest.NewRecorder()
	testRouter(h).ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "evil.exe", jpegPayload(16), "kb", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	info := decodeFileInfo(t, rec)
	if info.MimeType != "image/jpeg" {
		t.Fatalf("mime = %q, want image/jpeg", info.MimeType)
	}
	stored, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if filepath.Ext(stored.StoredPath) != ".jpg" {
		t.Fatalf("stored_path = %q, ожидалось .jpg", stored.StoredPath)
	}
}

// Некорректный/несуществующий id → 404 (не раскрываем существование).
func TestHandler_GetNotFound(t *testing.T) {
	h, _, _ := newTestHandler(t, Options{})
	router := testRouter(h)

	for _, path := range []string{"/api/files/999", "/api/files/abc", "/api/files/0"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

// Без TicketTokenAuthorizer публичные маршруты отвечают 404 (до Б5).
func TestHandler_PublicRoutesWithoutAuthorizer(t *testing.T) {
	h, repo, _ := newTestHandler(t, Options{})
	router := testRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/files/1?token=any", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public GET = %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, multipartUpload(t, "/api/public/files?token=any", "file", "photo.jpg", jpegPayload(16), "", true))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("public upload = %d, want 404", rec.Code)
	}
	if repo.count() != 0 {
		t.Fatal("файл сохранён без проверки токена")
	}
}

// TICKET-13/TICKET-14: аноним загружает вложение по токену и читает его.
func TestHandler_PublicUploadAndGet(t *testing.T) {
	authorizer := newFakeAuthorizer()
	authorizer.tokens["live-token"] = 55
	h, repo, store := newTestHandler(t, Options{TicketAuthorizer: authorizer})
	router := testRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, multipartUpload(t, "/api/public/files?token=live-token", "file", "anon.jpg", jpegPayload(64), "", true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("public upload = %d (%s)", rec.Code, rec.Body.String())
	}
	info := decodeFileInfo(t, rec)
	if !strings.Contains(info.URL, "/public/files/") || !strings.Contains(info.URL, "token=live-token") {
		t.Fatalf("url = %q", info.URL)
	}
	stored, err := repo.Get(context.Background(), info.ID)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if !stored.IsAnonymous || stored.UploaderID != nil {
		t.Fatalf("файл анонима сохранён неверно: %+v", stored)
	}
	if stored.Scope != ScopeTicket {
		t.Fatalf("scope = %q, want ticket", stored.Scope)
	}

	// Владелец токена читает файл, пока файл привязан к его заявке.
	authorizer.accessible[info.ID] = true
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, info.URL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public GET = %d (%s)", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), jpegPayload(64)) {
		t.Fatal("аноним получил другое содержимое")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff отсутствует: %q", got)
	}

	// Чужой токен → 404.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/files/"+strconv.FormatInt(info.ID, 10)+"?token=other", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("чужой токен: %d, want 404", rec.Code)
	}
	// Пустой токен → 404.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/files/"+strconv.FormatInt(info.ID, 10), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("без токена: %d, want 404", rec.Code)
	}

	assertNoStaging(t, store.Dir())
}

// Неизвестный токен при публичной загрузке → 404, файл не сохранён.
func TestHandler_PublicUploadUnknownToken(t *testing.T) {
	authorizer := newFakeAuthorizer()
	h, repo, store := newTestHandler(t, Options{TicketAuthorizer: authorizer})

	rec := httptest.NewRecorder()
	testRouter(h).ServeHTTP(rec, multipartUpload(t, "/api/public/files?token=dead", "file", "anon.jpg", jpegPayload(16), "", true))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d (%s), want 404", rec.Code, rec.Body.String())
	}
	if repo.count() != 0 {
		t.Fatal("файл сохранён при неизвестном токене")
	}
	assertNoStaging(t, store.Dir())
}

// Кириллица в имени файла кодируется по RFC 2231, но остаётся inline.
func TestHandler_ContentDispositionCyrillic(t *testing.T) {
	h, _, _ := newTestHandler(t, Options{})
	router := testRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, multipartUpload(t, "/api/files", "file", "фото объекта.jpg", jpegPayload(16), "ticket", true))
	info := decodeFileInfo(t, rec)
	if info.Filename != "фото объекта.jpg" {
		t.Fatalf("filename = %q", info.Filename)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/"+strconv.FormatInt(info.ID, 10), nil))
	disposition := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "inline") {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
	if !strings.Contains(disposition, "filename*=") {
		t.Fatalf("ожидалось RFC 2231-кодирование имени: %q", disposition)
	}
	if strings.Contains(disposition, "\r") || strings.Contains(disposition, "\n") {
		t.Fatalf("в заголовке управляющие символы: %q", disposition)
	}
}
