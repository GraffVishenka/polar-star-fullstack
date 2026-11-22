package files

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// Б3: Cleaner удаляет только непривязанные файлы старше суток (диск + БД).
func TestCleaner_RunOnce(t *testing.T) {
	svc, repo, store := newTestService(t, Options{})
	ctx := context.Background()
	clock := platform.FixedClock{Time: baseTime}

	old := mustUploadFile(t, svc, "old.jpg")
	repo.setCreatedAt(old.ID, baseTime.Add(-48*time.Hour))
	fresh := mustUploadFile(t, svc, "fresh.jpg")

	linked := mustUploadFile(t, svc, "linked.jpg")
	if err := svc.MarkLinked(ctx, []int64{linked.ID}); err != nil {
		t.Fatalf("MarkLinked: %v", err)
	}
	repo.setCreatedAt(linked.ID, baseTime.Add(-48*time.Hour))

	cleaner := NewCleaner(repo, store, discardLogger(), clock, CleanerOptions{
		Interval:  time.Hour,
		Retention: DefaultOrphanRetention,
	})
	deleted, err := cleaner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, err := repo.Get(ctx, old.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("запись orphan-файла не удалена: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir(), filepath.FromSlash(old.StoredPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan-файл остался на диске: %v", err)
	}
	for _, keep := range []*File{fresh, linked} {
		if _, err := repo.Get(ctx, keep.ID); err != nil {
			t.Fatalf("файл %d удалён ошибочно: %v", keep.ID, err)
		}
		if _, err := os.Stat(filepath.Join(store.Dir(), filepath.FromSlash(keep.StoredPath))); err != nil {
			t.Fatalf("файл %d пропал с диска: %v", keep.ID, err)
		}
	}
}

// Ошибка БД в очистке не паникует и возвращается как ошибка (API не страдает).
func TestCleaner_RunOnceRepoError(t *testing.T) {
	_, repo, store := newTestService(t, Options{})
	cleaner := NewCleaner(repo, store, discardLogger(), platform.FixedClock{Time: baseTime}, CleanerOptions{})
	repo.orphanErr = errors.New("db is down")

	deleted, err := cleaner.RunOnce(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка выборки orphan-файлов")
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0", deleted)
	}
}

// Run завершается по отмене контекста.
func TestCleaner_RunStopsOnCancel(t *testing.T) {
	_, repo, store := newTestService(t, Options{})
	cleaner := NewCleaner(repo, store, discardLogger(), platform.FixedClock{Time: baseTime}, CleanerOptions{})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		cleaner.Run(ctx)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run не остановился по отмене контекста")
	}
}

// mustUploadFile загружает и сохраняет файл заявки.
func mustUploadFile(t *testing.T, svc *Service, name string) *File {
	t.Helper()
	pending, err := svc.Upload(context.Background(), bytes.NewReader(jpegPayload(16)), name)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	f, err := svc.Save(context.Background(), pending, UploadMeta{Scope: ScopeTicket})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	return f
}
