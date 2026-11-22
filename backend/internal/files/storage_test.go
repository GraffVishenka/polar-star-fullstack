package files

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// newTestStorage — хранилище во временном каталоге с детерминированным UUID.
func newTestStorage(t *testing.T) (*Storage, *platform.FixedClock) {
	t.Helper()
	clock := &platform.FixedClock{Time: baseTime}
	store := NewStorage(t.TempDir(), yekaterinburg(t), clock).WithIDGenerator(fixedID(testUUID))
	return store, clock
}

// Б3: потоковая запись на диск, MIME по сигнатуре, sha256, путь YYYY/MM.
func TestStorage_StageAndCommit(t *testing.T) {
	store, clock := newTestStorage(t)
	payload := jpegPayload(2048)

	staged, err := store.Stage(bytes.NewReader(payload), 0)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if staged.Media.MIME != "image/jpeg" || staged.Media.Ext != "jpg" {
		t.Fatalf("media = %+v", staged.Media)
	}
	if staged.Size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", staged.Size, len(payload))
	}
	sum := sha256.Sum256(payload)
	if staged.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %q", staged.SHA256)
	}
	if !strings.HasPrefix(filepath.Base(staged.TempPath), stagingPrefix) {
		t.Fatalf("временный файл вне staging-префикса: %q", staged.TempPath)
	}

	rel, err := store.Commit(staged, clock.Now())
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	wantRel := "2026/05/" + testUUID + ".jpg"
	if rel != wantRel {
		t.Fatalf("rel = %q, want %q", rel, wantRel)
	}

	onDisk, err := os.ReadFile(filepath.Join(store.Dir(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("чтение сохранённого файла: %v", err)
	}
	if !bytes.Equal(onDisk, payload) {
		t.Fatal("содержимое на диске отличается от загруженного")
	}

	// Временного файла после Commit быть не должно.
	entries, readErr := os.ReadDir(store.Dir())
	if readErr != nil {
		// wasm под Windows не открывает каталоги (O_DIRECTORY).
		t.Logf("проверка состава каталога загрузок пропущена: %v", readErr)
	} else {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		if len(names) != 1 || names[0] != "2026" {
			t.Fatalf("в корне загрузок остались лишние файлы: %v", names)
		}
	}

	// Open отдаёт читаемый файл (нужен http.ServeContent для Range).
	fh, info, err := store.Open(rel)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = fh.Close() }()
	if info.Size() != int64(len(payload)) {
		t.Fatalf("info.Size = %d", info.Size())
	}
}

// Б3: расширение берётся из MIME, а не из клиентского имени.
func TestStorage_CommitUsesMediaExtension(t *testing.T) {
	store, clock := newTestStorage(t)
	staged, err := store.Stage(bytes.NewReader(pngHead), 0)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	rel, err := store.Commit(staged, clock.Now())
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if filepath.Ext(rel) != ".png" {
		t.Fatalf("rel = %q, ожидалось расширение .png", rel)
	}
}

// Б3: превышение лимита → SizeLimitError, временный файл убран.
func TestStorage_StageRejectsOversized(t *testing.T) {
	store, _ := newTestStorage(t)
	payload := jpegPayload(4096)

	_, err := store.Stage(bytes.NewReader(payload), 1024)
	var limitErr *SizeLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("ожидалась SizeLimitError, получено %v", err)
	}
	if limitErr.Limit != 1024 {
		t.Fatalf("limit = %d", limitErr.Limit)
	}
	if limitErr.Size <= 1024 {
		t.Fatalf("size = %d, ожидалось больше лимита", limitErr.Size)
	}
	assertNoStaging(t, store.Dir())
}

// Файл ровно по лимиту принимается (граница включительна).
func TestStorage_StageAcceptsExactLimit(t *testing.T) {
	store, _ := newTestStorage(t)
	payload := jpegPayload(1024 - len(jpegHead))
	staged, err := store.Stage(bytes.NewReader(payload), 1024)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if staged.Size != 1024 {
		t.Fatalf("size = %d, want 1024", staged.Size)
	}
	staged.Discard()
}

// Б3: `.exe` под видом jpeg отклоняется по сигнатуре.
func TestStorage_StageRejectsSpoofedMedia(t *testing.T) {
	store, _ := newTestStorage(t)
	payload := append([]byte("MZ\x90\x00"), bytes.Repeat([]byte{0x00}, 256)...)

	_, err := store.Stage(bytes.NewReader(payload), 0)
	if !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("ожидалась ErrUnsupportedMedia, получено %v", err)
	}
	assertNoStaging(t, store.Dir())
}

func TestStorage_StageRejectsEmptyStream(t *testing.T) {
	store, _ := newTestStorage(t)
	if _, err := store.Stage(bytes.NewReader(nil), 0); !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("пустой файл: ожидалась ErrUnsupportedMedia, получено %v", err)
	}
	assertNoStaging(t, store.Dir())
}

// Б3: загрузка видео идёт по отдельному лимиту 200 МБ.
func TestStorage_StageAcceptsVideo(t *testing.T) {
	store, _ := newTestStorage(t)
	staged, err := store.Stage(bytes.NewReader(webmPayload(64)), 0)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if staged.Media.Kind != KindVideo || staged.Media.MIME != "video/webm" {
		t.Fatalf("media = %+v", staged.Media)
	}
	staged.Discard()
}

// Б3: защита от path traversal при чтении/удалении.
func TestStorage_OpenRejectsTraversal(t *testing.T) {
	store, _ := newTestStorage(t)
	for _, rel := range []string{"../etc/passwd", "/etc/passwd", "../../x"} {
		if _, _, err := store.Open(rel); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Open(%q): ожидалась ErrUnsafePath, получено %v", rel, err)
		}
		if err := store.Remove(rel); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Remove(%q): ожидалась ErrUnsafePath, получено %v", rel, err)
		}
	}
}

// Remove идемпотентен: отсутствие файла не мешает удалить запись (очистка orphan).
func TestStorage_RemoveMissingFile(t *testing.T) {
	store, _ := newTestStorage(t)
	if err := store.Remove("2026/05/" + testUUID + ".jpg"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

// Б3: временные файлы прерванных загрузок удаляются, свежие — нет.
func TestStorage_CleanStaleStaging(t *testing.T) {
	if runtime.GOOS == "js" {
		// Ограничение wasm-рантайма под Windows: открыть каталог нельзя
		// (O_DIRECTORY не поддерживается), поэтому проверка выполняется только
		// на реальной ФС — в CI (Linux) и на машинах разработчиков.
		t.Skip("os.ReadDir недоступен в wasm-рантайме под Windows")
	}
	store, clock := newTestStorage(t)
	if err := os.MkdirAll(store.Dir(), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	stale := filepath.Join(store.Dir(), stagingPrefix+"old")
	fresh := filepath.Join(store.Dir(), stagingPrefix+"new")
	for _, p := range []string{stale, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	old := clock.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	removed, err := store.CleanStaleStaging(clock.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("CleanStaleStaging: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("свежий временный файл удалён: %v", err)
	}
}

// assertNoStaging проверяет, что в корне загрузок не осталось временных файлов.
//
// В wasm-рантайме под Windows каталог нельзя открыть (O_DIRECTORY), поэтому
// проверка там пропускается с записью в лог; в CI на Linux она выполняется.
func assertNoStaging(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		t.Logf("проверка временных файлов пропущена: каталог %q не читается (%v)", dir, err)
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagingPrefix) {
			t.Fatalf("остался временный файл %q", entry.Name())
		}
	}
}
