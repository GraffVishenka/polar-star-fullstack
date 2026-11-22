package kb

import (
	"strings"
	"testing"
)

// KB-4: script/style/iframe/on*-атрибуты вырезаются, безопасная разметка
// сохраняется.
func TestSanitize_RemovesDangerousMarkup(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		forbidden []string
		required  []string
	}{
		{
			name:      "script",
			input:     `<p>текст</p><script>alert(1)</script>`,
			forbidden: []string{"script", "alert(1)"},
			required:  []string{"<p>текст</p>"},
		},
		{
			name:      "onclick",
			input:     `<p onclick="steal()">текст</p>`,
			forbidden: []string{"onclick", "steal()"},
			required:  []string{"<p>текст</p>"},
		},
		{
			name:      "iframe",
			input:     `<iframe src="https://evil.example"></iframe><p>ok</p>`,
			forbidden: []string{"iframe", "evil.example"},
			required:  []string{"<p>ok</p>"},
		},
		{
			name:      "javascript href",
			input:     `<a href="javascript:alert(1)">клик</a>`,
			forbidden: []string{"javascript:", "href"},
			required:  []string{"клик"},
		},
		{
			name:      "style attribute and tag",
			input:     `<p style="position:fixed">x</p><style>body{display:none}</style>`,
			forbidden: []string{"style", "position", "display:none"},
			required:  []string{"<p>x</p>"},
		},
		{
			name:      "data attribute обращение к on*",
			input:     `<img src="/api/files/5" onerror="alert(1)" alt="фото">`,
			forbidden: []string{"onerror", "alert(1)"},
			required:  []string{`src="/api/files/5"`, `alt="фото"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Sanitize(c.input)
			lower := strings.ToLower(got)
			for _, bad := range c.forbidden {
				if strings.Contains(lower, strings.ToLower(bad)) {
					t.Fatalf("в результате осталось %q: %s", bad, got)
				}
			}
			for _, good := range c.required {
				if !strings.Contains(got, good) {
					t.Fatalf("потерян ожидаемый фрагмент %q: %s", good, got)
				}
			}
		})
	}
}

// KB-4/KB-5: разрешённые теги и медиа сохраняются без искажений.
func TestSanitize_KeepsAllowedMarkupAndMedia(t *testing.T) {
	input := `<h2>Заголовок</h2>
<p>Абзац со <strong>смелым</strong> текстом и <a href="https://example.com/doc">ссылкой</a>.</p>
<ul><li>первый</li><li>второй</li></ul>
<pre><code>x := 1</code></pre>
<img src="/api/files/12" alt="схема" width="640">
<video controls preload="metadata" width="640">
  <source src="/api/files/34" type="video/mp4">
</video>
<table><thead><tr><th colspan="2">Заголовок</th></tr></thead><tbody><tr><td>a</td><td>b</td></tr></tbody></table>`
	got := Sanitize(input)

	required := []string{
		"<h2>Заголовок</h2>",
		"<strong>смелым</strong>",
		`href="https://example.com/doc"`,
		"<ul>", "<li>первый</li>",
		"<pre>", "<code>",
		`src="/api/files/12"`,
		`alt="схема"`,
		`<video controls`,
		`<source src="/api/files/34"`,
		`type="video/mp4"`,
		"<table>", `<th colspan="2">`,
	}
	for _, fragment := range required {
		if !strings.Contains(got, fragment) {
			t.Fatalf("потерян фрагмент %q:\n%s", fragment, got)
		}
	}
}

// Медиа статьи допускает только локальные файлы: внешние картинки и пути с
// выходом из каталога вырезаются.
func TestSanitize_RejectsExternalAndTraversalMedia(t *testing.T) {
	for _, src := range []string{
		"https://evil.example/x.png",
		"//evil.example/x.png",
		"/api/files/../../etc/passwd",
		"/api/files/1?x=1",
		"/other/files/1",
	} {
		got := Sanitize(`<img src="` + src + `" alt="x">`)
		if strings.Contains(got, "src=") {
			t.Fatalf("недопустимый src %q сохранён: %s", src, got)
		}
	}
}

func TestSanitize_Empty(t *testing.T) {
	if got := Sanitize(""); got != "" {
		t.Fatalf("Sanitize(\"\") = %q", got)
	}
}

// KB-5: из HTML извлекаются уникальные обычные id файлов (без префиксов и
// повторов), включая источники video.
func TestExtractMediaIDs(t *testing.T) {
	content := `<p><img src="/api/files/12" alt="a"></p>
<video controls><source src="/api/files/34" type="video/mp4"></video>
<p>дубль <img src="/api/files/12"></p>
<a href="/api/files/56">файл</a>`
	ids, overflow := ExtractMediaIDs(content)
	if overflow {
		t.Fatal("неожиданное переполнение")
	}
	want := []int64{12, 34, 56}
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
}

func TestExtractMediaIDs_Empty(t *testing.T) {
	if ids, overflow := ExtractMediaIDs(""); ids != nil || overflow {
		t.Fatalf("ids = %v, overflow = %v", ids, overflow)
	}
	if ids, _ := ExtractMediaIDs("<p>без медиа</p>"); len(ids) != 0 {
		t.Fatalf("ids = %v", ids)
	}
}

// Слишком много ссылок в статье — признак переполнения для валидации.
func TestExtractMediaIDs_Overflow(t *testing.T) {
	var builder strings.Builder
	for id := 1; id <= maxMediaRefs+5; id++ {
		builder.WriteString(`<img src="/api/files/`)
		builder.WriteString(intToString(id))
		builder.WriteString(`">`)
	}
	ids, overflow := ExtractMediaIDs(builder.String())
	if !overflow {
		t.Fatalf("ожидалось переполнение, ids=%d", len(ids))
	}
	if len(ids) != maxMediaRefs {
		t.Fatalf("len(ids) = %d, want %d", len(ids), maxMediaRefs)
	}
}

// Точное совпадение id: /api/files/12 не должно считаться ссылкой на файл 1.
func TestContentReferencesFile(t *testing.T) {
	content := `<img src="/api/files/12">`
	if ContentReferencesFile(content, 1) {
		t.Fatal("id=1 найден в ссылке на файл 12")
	}
	if !ContentReferencesFile(content, 12) {
		t.Fatal("id=12 не найден")
	}
	if ContentReferencesFile(content, 123) {
		t.Fatal("id=123 найден без ссылки")
	}
}

// intToString — маленький помощник, чтобы не тянуть strconv в тест-хелпер.
func intToString(value int) string {
	if value == 0 {
		return "0"
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
