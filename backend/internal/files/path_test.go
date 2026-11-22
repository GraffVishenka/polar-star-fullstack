package files

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// yekaterinburg загружает пояс приложения (CAL-2).
func yekaterinburg(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Yekaterinburg")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Yekaterinburg): %v", err)
	}
	return loc
}

const testUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"

// Б3: имя файла на диске — {uuid}.{ext}, расширение из MIME, не из имени
// клиента. Клиентское имя не может повлиять на путь (path traversal).
func TestStoredName(t *testing.T) {
	name, err := StoredName(testUUID, "jpg")
	if err != nil {
		t.Fatalf("StoredName: %v", err)
	}
	if name != testUUID+".jpg" {
		t.Fatalf("StoredName = %q", name)
	}

	for _, ext := range []string{"", "..", "../jpg", "php", "jpeg", "JPG", "jpg/../../x"} {
		if _, err := StoredName(testUUID, ext); err == nil {
			t.Fatalf("StoredName принял недопустимое расширение %q", ext)
		}
	}
	for _, id := range []string{"", "..", "../x", "not-a-uuid", testUUID + "/../x"} {
		if _, err := StoredName(id, "png"); err == nil {
			t.Fatalf("StoredName принял недопустимый id %q", id)
		}
	}
}

// Б3: путь /data/uploads/YYYY/MM/{uuid}.{ext}, где YYYY/MM — в часовом поясе
// приложения, а не UTC (CAL-2 / NFR-7).
func TestRelPath_UsesAppTimezone(t *testing.T) {
	loc := yekaterinburg(t)

	// 31 октября 20:00 UTC = 1 ноября 01:00 по Екатеринбургу (+5) → уже ноябрь.
	at := time.Date(2026, 10, 31, 20, 0, 0, 0, time.UTC)
	got, err := RelPath(at, loc, testUUID, "jpg")
	if err != nil {
		t.Fatalf("RelPath: %v", err)
	}
	want := "2026/11/" + testUUID + ".jpg"
	if got != want {
		t.Fatalf("RelPath = %q, want %q", got, want)
	}

	// Тот же момент в UTC дал бы октябрь — проверяем, что пояс учтён.
	if strings.Contains(got, "2026/10") {
		t.Fatalf("путь построен по UTC: %q", got)
	}
}

func TestRelPath_ExtAndNilLocation(t *testing.T) {
	at := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	got, err := RelPath(at, nil, testUUID, "webm")
	if err != nil {
		t.Fatalf("RelPath: %v", err)
	}
	if got != "2026/01/"+testUUID+".webm" {
		t.Fatalf("RelPath = %q", got)
	}
	if _, err := RelPath(at, nil, testUUID, "exe"); err == nil {
		t.Fatalf("RelPath принял расширение exe")
	}
}

// Б3: безопасное имя файла — без каталогов, «..» и управляющих символов.
func TestSafeOriginalName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"photo.jpg", "photo.jpg"},
		{"../../etc/passwd", "passwd"},
		{`..\..\windows\win.ini`, "win.ini"},
		{"/absolute/path/file.png", "file.png"},
		{"..", fallbackName},
		{".", fallbackName},
		{"", fallbackName},
		{"   ", fallbackName},
		{"..hidden..", "hidden"},
		{"file\x00name.jpg", "filename.jpg"},
		{"a\nb\tc.jpg", "abc.jpg"},
		{"  report  .pdf  ", "report  .pdf"},
		{"имя файла.jpg", "имя файла.jpg"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := SafeOriginalName(c.in)
			if got != c.want {
				t.Fatalf("SafeOriginalName(%q) = %q, want %q", c.in, got, c.want)
			}
			if strings.ContainsAny(got, `/\`) {
				t.Fatalf("в безопасном имени остался разделитель пути: %q", got)
			}
		})
	}
}

func TestSafeOriginalName_TruncatesLongNames(t *testing.T) {
	long := strings.Repeat("я", 400) + ".jpg"
	got := SafeOriginalName(long)
	if runes := []rune(got); len(runes) > maxOriginalNameRunes {
		t.Fatalf("имя не обрезано: %d рун", len(runes))
	}
	if !strings.HasSuffix(got, ".jpg") {
		t.Fatalf("потеряно расширение при обрезке: %q", got[len(got)-10:])
	}

	// Очень длинное расширение не должно ломать обрезку.
	got = SafeOriginalName(strings.Repeat("a", 300) + "." + strings.Repeat("b", 300))
	if runes := []rune(got); len(runes) > maxOriginalNameRunes {
		t.Fatalf("имя не обрезано: %d рун", len(runes))
	}
}

// Б3: защита от path traversal при чтении/удалении — путь из БД не должен
// выводить за пределы UPLOAD_DIR.
func TestResolvePath(t *testing.T) {
	base := t.TempDir()

	ok, err := ResolvePath(base, "2026/11/"+testUUID+".jpg")
	if err != nil {
		t.Fatalf("ResolvePath(валидный): %v", err)
	}
	want := filepath.Join(base, "2026", "11", testUUID+".jpg")
	if ok != want {
		t.Fatalf("ResolvePath = %q, want %q", ok, want)
	}

	bad := []string{
		"",
		"..",
		"../secret.txt",
		"../../etc/passwd",
		"2026/../../../etc/passwd",
		"/etc/passwd",
		".",
	}
	for _, rel := range bad {
		if _, err := ResolvePath(base, rel); err == nil {
			t.Fatalf("ResolvePath принял недопустимый путь %q", rel)
		}
	}
}

func TestResolvePath_RejectsEscapeAfterClean(t *testing.T) {
	base := t.TempDir()
	// «безобидный» с виду путь, который после Clean выходит наружу.
	if _, err := ResolvePath(base, "2026/11/../../../../x"); err == nil {
		t.Fatal("ResolvePath не отклонил выход за пределы UPLOAD_DIR")
	}
}
