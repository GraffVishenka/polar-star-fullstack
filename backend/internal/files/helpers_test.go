package files

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// baseTime — фиксированный момент времени для детерминированных тестов.
var baseTime = time.Date(2026, 5, 20, 9, 30, 0, 0, time.UTC)

// fakeRepo — реализация Repository в памяти: handler'ы и сервис тестируются
// без MySQL (в окружении разработки БД нет, docs/ENVIRONMENT.md).
type fakeRepo struct {
	mu           sync.Mutex
	files        map[int64]*File
	commentFiles map[int64]map[int64]struct{}
	nextID       int64

	insertErr error
	getErr    error
	orphanErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		files:        make(map[int64]*File),
		commentFiles: make(map[int64]map[int64]struct{}),
	}
}

func (r *fakeRepo) Insert(_ context.Context, f *File) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.insertErr != nil {
		return r.insertErr
	}
	r.nextID++
	f.ID = r.nextID
	copied := *f
	r.files[f.ID] = &copied
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (*File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.getErr != nil {
		return nil, r.getErr
	}
	f, ok := r.files[id]
	if !ok {
		return nil, ErrNotFound
	}
	copied := *f
	return &copied, nil
}

func (r *fakeRepo) MarkLinked(_ context.Context, fileIDs []int64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range fileIDs {
		if f, ok := r.files[id]; ok && f.LinkedAt == nil {
			linkedAt := at
			f.LinkedAt = &linkedAt
		}
	}
	return nil
}

func (r *fakeRepo) AttachToComment(_ context.Context, commentID int64, fileIDs []int64, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	linked, ok := r.commentFiles[commentID]
	if !ok {
		linked = make(map[int64]struct{}, len(fileIDs))
		r.commentFiles[commentID] = linked
	}
	// Повторная привязка тех же файлов идемпотентна: считаем только «прочие»
	// существующие вложения (как в MySQL-реализации).
	incoming := make(map[int64]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		incoming[id] = struct{}{}
	}
	others := 0
	for id := range linked {
		if _, ok := incoming[id]; !ok {
			others++
		}
	}
	if others+len(fileIDs) > MaxAttachmentsPerComment {
		return ErrTooManyAttachments
	}
	for _, id := range fileIDs {
		f, exists := r.files[id]
		if !exists {
			return ErrNotFound
		}
		// Вложением комментария может быть только файл заявки; уже привязанный
		// файл допустим лишь в «своём» комментарии (защита от перехвата по id).
		if f.Scope != ScopeTicket {
			return ErrNotFound
		}
		if f.LinkedAt != nil {
			if _, ok := linked[id]; !ok {
				return ErrNotFound
			}
		}
	}
	for _, id := range fileIDs {
		linked[id] = struct{}{}
		if f := r.files[id]; f.LinkedAt == nil {
			linkedAt := at
			f.LinkedAt = &linkedAt
		}
	}
	return nil
}

func (r *fakeRepo) OrphanFiles(_ context.Context, cutoff time.Time, limit int) ([]File, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.orphanErr != nil {
		return nil, r.orphanErr
	}
	out := make([]File, 0, len(r.files))
	for _, f := range r.files {
		if f.LinkedAt == nil && f.CreatedAt.Before(cutoff) {
			out = append(out, *f)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.files, id)
	for commentID, linked := range r.commentFiles {
		delete(linked, id)
		if len(linked) == 0 {
			delete(r.commentFiles, commentID)
		}
	}
	return nil
}

// setCreatedAt меняет дату создания записи (имитация «старого» файла).
func (r *fakeRepo) setCreatedAt(id int64, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f, ok := r.files[id]; ok {
		f.CreatedAt = at
	}
}

// count возвращает число записей (для проверок «файл не сохранён»).
func (r *fakeRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.files)
}

// attachments возвращает число вложений комментария.
func (r *fakeRepo) attachments(commentID int64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.commentFiles[commentID])
}

// fakeAuthorizer — заглушка TicketTokenAuthorizer (реализацию даёт Б5).
type fakeAuthorizer struct {
	tokens     map[string]int64
	accessible map[int64]bool
}

func newFakeAuthorizer() *fakeAuthorizer {
	return &fakeAuthorizer{tokens: map[string]int64{}, accessible: map[int64]bool{}}
}

func (a *fakeAuthorizer) TicketIDByToken(_ context.Context, token string) (int64, error) {
	if id, ok := a.tokens[token]; ok {
		return id, nil
	}
	return 0, ErrTokenNotFound
}

func (a *fakeAuthorizer) FileBelongsToTicketToken(_ context.Context, fileID int64, token string) (bool, error) {
	if _, ok := a.tokens[token]; !ok {
		return false, nil
	}
	return a.accessible[fileID], nil
}

// fakeKBMedia — заглушка files.KBMediaAuthorizer (реализация — kb.Service в Б6).
// Отвечает «файл вставлен в публичную статью БЗ».
type fakeKBMedia struct {
	public map[int64]bool
	err    error
}

func newFakeKBMedia() *fakeKBMedia {
	return &fakeKBMedia{public: map[int64]bool{}}
}

func (a *fakeKBMedia) PublicKBFile(_ context.Context, fileID int64) (bool, error) {
	if a.err != nil {
		return false, a.err
	}
	return a.public[fileID], nil
}

// fixedID возвращает детерминированный генератор идентификаторов.
func fixedID(id string) IDGenerator {
	return func() (string, error) { return id, nil }
}

// seqID возвращает генератор UUID-подобных идентификаторов с инкрементом.
func seqID() IDGenerator {
	var (
		mu  sync.Mutex
		seq int
	)
	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		seq++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", seq), nil
	}
}

// discardLogger — логгер, не засоряющий вывод тестов.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newTestService собирает сервис с фейковым репозиторием и временным каталогом.
func newTestService(t *testing.T, opts Options) (*Service, *fakeRepo, *Storage) {
	t.Helper()
	repo := newFakeRepo()
	clock := platform.FixedClock{Time: baseTime}
	if opts.Clock == nil {
		opts.Clock = clock
	}
	if opts.Logger == nil {
		opts.Logger = discardLogger()
	}
	store := NewStorage(t.TempDir(), yekaterinburg(t), clock).WithIDGenerator(seqID())
	return NewService(repo, store, opts), repo, store
}

// jpegPayload — минимальный «jpeg»: сигнатура + полезная нагрузка.
func jpegPayload(size int) []byte {
	payload := make([]byte, 0, len(jpegHead)+size)
	payload = append(payload, jpegHead...)
	for i := 0; i < size; i++ {
		payload = append(payload, byte(i%251))
	}
	return payload
}

// webmPayload — минимальный «webm» с сигнатурой EBML и DocType webm.
func webmPayload(size int) []byte {
	payload := make([]byte, 0, len(webmHead)+size)
	payload = append(payload, webmHead...)
	for i := 0; i < size; i++ {
		payload = append(payload, byte(i%251))
	}
	return payload
}
