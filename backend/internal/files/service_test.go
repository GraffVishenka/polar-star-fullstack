package files

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// apiErrorStatus достаёт HTTP-статус из ошибки API.
func apiErrorStatus(t *testing.T, err error) int {
	t.Helper()
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ожидалась ошибка API, получено %v", err)
	}
	return apiErr.Status
}

// Б3: загрузка и сохранение файла заявки — метаданные, sha256, файл на диске.
func TestService_SaveTicketFile(t *testing.T) {
	svc, repo, store := newTestService(t, Options{})
	ctx := context.Background()
	payload := jpegPayload(512)
	uploaderID := int64(7)

	pending, err := svc.Upload(ctx, bytes.NewReader(payload), "фото объекта.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	f, err := svc.Save(ctx, pending, UploadMeta{Scope: ScopeTicket, UploaderID: &uploaderID})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if f.ID == 0 {
		t.Fatal("не проставлен id файла")
	}
	if f.OriginalName != "фото объекта.jpg" {
		t.Fatalf("original_name = %q", f.OriginalName)
	}
	if f.MIME != "image/jpeg" || f.Scope != ScopeTicket {
		t.Fatalf("file = %+v", f)
	}
	if f.SizeBytes != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", f.SizeBytes, len(payload))
	}
	if f.LinkedAt != nil {
		t.Fatal("вложение заявки не должно быть привязано до AttachToComment")
	}
	if f.IsAnonymous || f.UploaderID == nil || *f.UploaderID != uploaderID {
		t.Fatalf("автор загрузки сохранён неверно: %+v", f)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), filepath.FromSlash(f.StoredPath))); err != nil {
		t.Fatalf("файла нет на диске: %v", err)
	}
	if repo.count() != 1 {
		t.Fatalf("записей в репозитории = %d, want 1", repo.count())
	}

	stored, err := repo.Get(ctx, f.ID)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if stored.SHA256 != f.SHA256 || stored.StoredPath != f.StoredPath {
		t.Fatalf("метаданные в репозитории не совпадают: %+v", stored)
	}
}

// KB-5: медиа статьи привязано сразу — иначе автор не успевает дописать текст,
// а файл удаляется как orphan через 24 часа.
func TestService_SaveKBFile_LinkedImmediately(t *testing.T) {
	svc, _, _ := newTestService(t, Options{})
	ctx := context.Background()
	uploaderID := int64(3)

	pending, err := svc.Upload(ctx, bytes.NewReader(pngHead), "image.png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	f, err := svc.Save(ctx, pending, UploadMeta{Scope: ScopeKB, UploaderID: &uploaderID})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if f.LinkedAt == nil {
		t.Fatal("медиа БЗ должно быть помечено привязанным при сохранении")
	}
	if !f.LinkedAt.Equal(f.CreatedAt) {
		t.Fatalf("linked_at = %v, created_at = %v", f.LinkedAt, f.CreatedAt)
	}
}

// Б3: `.exe` под видом jpeg → 415, запись не создаётся.
func TestService_UploadSpoofedMimeRejected(t *testing.T) {
	svc, repo, store := newTestService(t, Options{})
	payload := append([]byte("MZ\x90\x00"), bytes.Repeat([]byte{0x01}, 128)...)

	_, err := svc.Upload(context.Background(), bytes.NewReader(payload), "photo.jpg")
	if status := apiErrorStatus(t, err); status != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", status)
	}
	if repo.count() != 0 {
		t.Fatalf("созданы записи при отклонённой загрузке: %d", repo.count())
	}
	assertNoStaging(t, store.Dir())
}

// Б3: фото больше лимита → 413 с limit_bytes в details.
func TestService_UploadOversizedPhoto(t *testing.T) {
	svc, repo, store := newTestService(t, Options{MaxUploadBytes: 1024})
	payload := jpegPayload(2048)

	_, err := svc.Upload(context.Background(), bytes.NewReader(payload), "big.jpg")
	if status := apiErrorStatus(t, err); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", status)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ожидалась *apierr.Error, получено %v", err)
	}
	if _, ok := apiErr.Details["limit_bytes"]; !ok {
		t.Fatalf("в details нет limit_bytes: %v", apiErr.Details)
	}
	if repo.count() != 0 {
		t.Fatal("создана запись при превышении лимита")
	}
	assertNoStaging(t, store.Dir())
}

// Б3: видео проходит по лимиту 200 МБ (проверяем на малом образце сигнатуры).
func TestService_UploadVideo(t *testing.T) {
	svc, _, _ := newTestService(t, Options{})
	pending, err := svc.Upload(context.Background(), bytes.NewReader(webmPayload(32)), "clip.webm")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	defer pending.Discard()
	if pending.staged.Media.MIME != "video/webm" {
		t.Fatalf("mime = %q", pending.staged.Media.MIME)
	}
}

// Save с неизвестной областью → 422 и временный файл удалён.
func TestService_SaveInvalidScope(t *testing.T) {
	svc, repo, store := newTestService(t, Options{})
	pending, err := svc.Upload(context.Background(), bytes.NewReader(jpegHead), "a.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	_, err = svc.Save(context.Background(), pending, UploadMeta{Scope: Scope("other")})
	if status := apiErrorStatus(t, err); status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", status)
	}
	if repo.count() != 0 {
		t.Fatal("создана запись с недопустимой областью")
	}
	assertNoStaging(t, store.Dir())
}

// Публичная выдача до подключения Б5 всегда 404 (не раскрываем файлы).
func TestService_OpenPublicRequiresAuthorizer(t *testing.T) {
	svc, _, _ := newTestService(t, Options{})
	_, _, err := svc.OpenPublic(context.Background(), 1, "token")
	if status := apiErrorStatus(t, err); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// TICKET-13: доступ по токену только к файлу, привязанному к заявке этого токена.
func TestService_OpenPublicWithAuthorizer(t *testing.T) {
	authorizer := newFakeAuthorizer()
	authorizer.tokens["live"] = 42
	svc, _, _ := newTestService(t, Options{TicketAuthorizer: authorizer})
	ctx := context.Background()

	pending, err := svc.Upload(ctx, bytes.NewReader(jpegPayload(16)), "a.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	f, err := svc.Save(ctx, pending, UploadMeta{Scope: ScopeTicket})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Файл ещё не привязан к комментарию заявки → 404.
	if _, _, err := svc.OpenPublic(ctx, f.ID, "live"); apiErrorStatus(t, err) != http.StatusNotFound {
		t.Fatalf("непривязанный файл должен давать 404, получено %v", err)
	}
	// Чужой/битый токен → 404.
	authorizer.accessible[f.ID] = true
	if _, _, err := svc.OpenPublic(ctx, f.ID, "dead"); apiErrorStatus(t, err) != http.StatusNotFound {
		t.Fatalf("чужой токен должен давать 404, получено %v", err)
	}
	// Привязанный к заявке файл с валидным токеном доступен.
	file, body, err := svc.OpenPublic(ctx, f.ID, "live")
	if err != nil {
		t.Fatalf("OpenPublic: %v", err)
	}
	defer func() { _ = body.Close() }()
	if file.ID != f.ID {
		t.Fatalf("file.ID = %d, want %d", file.ID, f.ID)
	}
}

// TICKET-14: аноним загружает вложение по токену заявки; файл анонимный.
func TestService_SaveAnonymous(t *testing.T) {
	authorizer := newFakeAuthorizer()
	authorizer.tokens["anon"] = 10
	svc, _, _ := newTestService(t, Options{TicketAuthorizer: authorizer})
	ctx := context.Background()

	pending, err := svc.Upload(ctx, bytes.NewReader(jpegPayload(16)), "anon.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	f, err := svc.SaveAnonymous(ctx, "anon", pending)
	if err != nil {
		t.Fatalf("SaveAnonymous: %v", err)
	}
	if !f.IsAnonymous {
		t.Fatal("файл должен быть помечен анонимным")
	}
	if f.UploaderID != nil {
		t.Fatalf("uploader_id должен быть NULL, получено %v", *f.UploaderID)
	}
	if f.Scope != ScopeTicket {
		t.Fatalf("scope = %q, want ticket", f.Scope)
	}

	// Неизвестный токен → 404 и ничего не сохранено.
	pending, err = svc.Upload(ctx, bytes.NewReader(jpegPayload(16)), "anon.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if _, err := svc.SaveAnonymous(ctx, "bad", pending); apiErrorStatus(t, err) != http.StatusNotFound {
		t.Fatalf("неизвестный токен: ожидался 404, получено %v", err)
	}
}

// TICKET-14: до 5 вложений на комментарий, повторная привязка идемпотентна.
func TestService_AttachToComment(t *testing.T) {
	svc, repo, _ := newTestService(t, Options{})
	ctx := context.Background()
	ids := make([]int64, 0, MaxAttachmentsPerComment)
	for i := 0; i < MaxAttachmentsPerComment; i++ {
		pending, err := svc.Upload(ctx, bytes.NewReader(jpegPayload(8)), "a.jpg")
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		f, err := svc.Save(ctx, pending, UploadMeta{Scope: ScopeTicket})
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		ids = append(ids, f.ID)
	}

	if err := svc.AttachToComment(ctx, 100, ids); err != nil {
		t.Fatalf("AttachToComment: %v", err)
	}
	if got := repo.attachments(100); got != MaxAttachmentsPerComment {
		t.Fatalf("вложений = %d, want %d", got, MaxAttachmentsPerComment)
	}
	for _, id := range ids {
		f, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("repo.Get(%d): %v", id, err)
		}
		if f.LinkedAt == nil {
			t.Fatalf("файл %d не помечен привязанным", id)
		}
	}

	// Повторная привязка тех же файлов идемпотентна (пара уникальна).
	if err := svc.AttachToComment(ctx, 100, ids); err != nil {
		t.Fatalf("повторная привязка: %v", err)
	}

	// Шестой файл к тому же комментарию → 422.
	extra, err := svc.Upload(ctx, bytes.NewReader(jpegPayload(8)), "extra.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	extraFile, err := svc.Save(ctx, extra, UploadMeta{Scope: ScopeTicket})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.AttachToComment(ctx, 100, []int64{extraFile.ID}); apiErrorStatus(t, err) != http.StatusUnprocessableEntity {
		t.Fatalf("шестое вложение: ожидался 422, получено %v", err)
	}

	// Больше 5 файлов за один вызов → 422 без обращения к репозиторию.
	if err := svc.AttachToComment(ctx, 101, append(ids, extraFile.ID)); apiErrorStatus(t, err) != http.StatusUnprocessableEntity {
		t.Fatalf("6 файлов в одном вызове: ожидался 422, получено %v", err)
	}
	// Несуществующий файл → 404.
	if err := svc.AttachToComment(ctx, 102, []int64{9999}); apiErrorStatus(t, err) != http.StatusNotFound {
		t.Fatalf("несуществующий файл: ожидался 404, получено %v", err)
	}
	// Некорректный комментарий → 422.
	if err := svc.AttachToComment(ctx, 0, ids); apiErrorStatus(t, err) != http.StatusUnprocessableEntity {
		t.Fatalf("comment_id=0: ожидался 422, получено %v", err)
	}
}

// AttachToComment убирает дубли и некорректные id.
func TestService_AttachToCommentDedupes(t *testing.T) {
	svc, repo, _ := newTestService(t, Options{})
	ctx := context.Background()
	pending, err := svc.Upload(ctx, bytes.NewReader(jpegPayload(8)), "a.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	f, err := svc.Save(ctx, pending, UploadMeta{Scope: ScopeTicket})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.AttachToComment(ctx, 7, []int64{f.ID, f.ID, 0, -1}); err != nil {
		t.Fatalf("AttachToComment: %v", err)
	}
	if got := repo.attachments(7); got != 1 {
		t.Fatalf("вложений = %d, want 1", got)
	}
}

// MarkLinked используется Б6/Б5 для файлов, привязанных иначе, чем комментарий.
func TestService_MarkLinked(t *testing.T) {
	svc, repo, _ := newTestService(t, Options{})
	ctx := context.Background()
	pending, err := svc.Upload(ctx, bytes.NewReader(jpegPayload(8)), "a.jpg")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	f, err := svc.Save(ctx, pending, UploadMeta{Scope: ScopeTicket})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := svc.MarkLinked(ctx, []int64{f.ID}); err != nil {
		t.Fatalf("MarkLinked: %v", err)
	}
	stored, err := repo.Get(ctx, f.ID)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if stored.LinkedAt == nil {
		t.Fatal("linked_at не проставлен")
	}
}

func TestService_OpenNotFound(t *testing.T) {
	svc, _, _ := newTestService(t, Options{})
	if _, _, err := svc.Open(context.Background(), 404); apiErrorStatus(t, err) != http.StatusNotFound {
		t.Fatalf("ожидался 404, получено %v", err)
	}
}

// NewFileInfo отдаёт поля контракта (openapi FileInfo).
func TestNewFileInfo(t *testing.T) {
	f := &File{ID: 5, OriginalName: "a.jpg", MIME: "image/jpeg", SizeBytes: 10, Scope: ScopeTicket, CreatedAt: baseTime}
	info := NewFileInfo(f)
	if info.ID != 5 || info.Filename != "a.jpg" || info.MimeType != "image/jpeg" ||
		info.Size != 10 || info.Scope != ScopeTicket || !info.CreatedAt.Equal(baseTime) {
		t.Fatalf("info = %+v", info)
	}
	if info.URL != "" {
		t.Fatalf("url должен заполнять HTTP-слой, получено %q", info.URL)
	}
}

// fakeAccess — заглушка FileAccessAuthorizer (реализация Б5).
type fakeAccess struct {
	allowed map[int64]bool
	err     error
}

func (a *fakeAccess) CanReadFile(_ context.Context, _ *auth.User, f *File) (bool, error) {
	if a.err != nil {
		return false, a.err
	}
	return a.allowed[f.ID], nil
}

// TICKET-7: доступ сотрудника к вложению чужой заявки закрыт (404), к своим —
// открыт; медиа БЗ проверке не подлежит (PLAN §3.1).
func TestService_OpenForUserAccess(t *testing.T) {
	access := &fakeAccess{allowed: map[int64]bool{}}
	svc, _, _ := newTestService(t, Options{FileAccessAuthorizer: access})
	ctx := context.Background()
	user := &auth.User{ID: 1, Role: auth.RoleUser}

	ticketFile := mustUploadFile(t, svc, "ticket.jpg")
	kbPending, err := svc.Upload(ctx, bytes.NewReader(pngHead), "kb.png")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	uploaderID := user.ID
	kbFile, err := svc.Save(ctx, kbPending, UploadMeta{Scope: ScopeKB, UploaderID: &uploaderID})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Чужое вложение заявки → 404 (существование не раскрывается).
	if _, _, err := svc.OpenForUser(ctx, user, ticketFile.ID); apiErrorStatus(t, err) != http.StatusNotFound {
		t.Fatalf("чужой файл заявки: ожидался 404, получено %v", err)
	}
	// Разрешённое вложение открывается.
	access.allowed[ticketFile.ID] = true
	if _, body, err := svc.OpenForUser(ctx, user, ticketFile.ID); err != nil {
		t.Fatalf("OpenForUser: %v", err)
	} else {
		_ = body.Close()
	}
	// Медиа БЗ доступно независимо от результата проверки по заявке.
	if _, body, err := svc.OpenForUser(ctx, user, kbFile.ID); err != nil {
		t.Fatalf("медиа БЗ должно быть доступно: %v", err)
	} else {
		_ = body.Close()
	}
	// Ошибка авторизатора → 500.
	access.err = errors.New("db is down")
	if _, _, err := svc.OpenForUser(ctx, user, ticketFile.ID); apiErrorStatus(t, err) != http.StatusInternalServerError {
		t.Fatalf("ошибка авторизатора: ожидался 500, получено %v", err)
	}
}

// Без FileAccessAuthorizer (до Б5) файлы заявок доступны авторизованным —
// контракт GET /api/files/{id}.
func TestService_OpenForUserWithoutAuthorizer(t *testing.T) {
	svc, _, _ := newTestService(t, Options{})
	ctx := context.Background()
	f := mustUploadFile(t, svc, "ticket.jpg")
	_, body, err := svc.OpenForUser(ctx, &auth.User{ID: 9, Role: auth.RoleUser}, f.ID)
	if err != nil {
		t.Fatalf("OpenForUser: %v", err)
	}
	_ = body.Close()
}
