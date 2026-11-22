package kb

import (
	"regexp"
	"strconv"

	"github.com/microcosm-cc/bluemonday"
)

// Ограничения на ссылки в content_html.
var (
	// mediaSrcPattern — медиа статьи всегда локальное (KB-5): только
	// `/api/files/{id}`. Внешние картинки запрещены, чтобы браузер читателя не
	// ходил на сторонние хосты (плюс их нельзя проверить на scope=kb).
	mediaSrcPattern = regexp.MustCompile(`^/api/files/[0-9]+$`)
	// mediaRefPattern — поиск ссылок на файлы в сохранённом HTML (KB-5).
	mediaRefPattern = regexp.MustCompile(`/api/files/([0-9]+)`)
	// linkURLPattern — ссылки: http/https или внутренний абсолютный путь.
	linkURLPattern = regexp.MustCompile(`^(?:https?://[^\s]+|/[^\s]*)$`)
	// mediaTypePattern — MIME в атрибуте type у <source> (video/mp4, video/webm).
	mediaTypePattern = regexp.MustCompile(`^[a-zA-Z0-9.+-]+/[a-zA-Z0-9.+-]+$`)
)

// maxMediaRefs — предел числа уникальных медиа-ссылок в одной статье: защита от
// «статьи» с тысячами ссылок, каждая из которых требует запроса к БД.
const maxMediaRefs = 200

// sanitizer — политика bluemonday для KB-4: строгий allowlist разрешённых
// тегов и атрибутов. Всё остальное (script, style, iframe, on*-атрибуты,
// javascript:-ссылки) вырезается.
var sanitizer = newSanitizer()

// newSanitizer собирает allowlist KB-4 + медиа-теги KB-5.
func newSanitizer() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	// URL-атрибуты проверяются: только http/https и относительные пути.
	// Без этого bluemonday по умолчанию не валидирует схемы вовсе.
	p.RequireParseableURLs(true)
	p.AllowURLSchemes("http", "https")
	p.AllowRelativeURLs(true)
	// Внешние ссылки получают rel="noreferrer" (безопасность перехода).
	p.RequireNoReferrerOnLinks(true)

	// Форматирование текста и структура статьи (KB-4).
	p.AllowElements(
		"p", "br", "hr", "div", "span",
		"strong", "b", "em", "i", "u", "s", "sub", "sup",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "blockquote",
		"code", "pre", "kbd", "samp",
		"table", "thead", "tbody", "tfoot", "tr", "th", "td",
		"figure", "figcaption",
	)

	// Ссылки: адрес ограничен http/https и внутренними путями.
	p.AllowAttrs("href").Matching(linkURLPattern).OnElements("a")
	p.AllowAttrs("title").OnElements("a")

	// Медиа между абзацами (KB-5): только локальные файлы с scope=kb.
	p.AllowAttrs("src").Matching(mediaSrcPattern).OnElements("img")
	p.AllowAttrs("alt", "title", "width", "height").OnElements("img")

	p.AllowAttrs("src").Matching(mediaSrcPattern).OnElements("video")
	p.AllowAttrs("controls", "preload", "width", "height", "loop", "muted", "playsinline").
		OnElements("video")

	p.AllowAttrs("src").Matching(mediaSrcPattern).OnElements("source")
	p.AllowAttrs("type").Matching(mediaTypePattern).OnElements("source")

	// Таблицы: только объединение ячеек, без стилей и inline-обработчиков.
	p.AllowAttrs("colspan", "rowspan").OnElements("th", "td")

	return p
}

// Sanitize очищает пользовательский HTML по allowlist KB-4, сохраняя медиа-теги
// (img/video/source), которые разрешены политикой.
func Sanitize(contentHTML string) string {
	if contentHTML == "" {
		return ""
	}
	return sanitizer.Sanitize(contentHTML)
}

// ExtractMediaIDs возвращает уникальные id файлов, на которые ссылается
// content_html (`/api/files/{id}`, включая src у img/video/source).
//
// Ограничение maxMediaRefs защищает от статьи с тысячами ссылок: лишние
// идентификаторы не проверяются, и сервис отвечает ошибкой валидации.
func ExtractMediaIDs(contentHTML string) (ids []int64, overflow bool) {
	if contentHTML == "" {
		return nil, false
	}
	matches := mediaRefPattern.FindAllStringSubmatch(contentHTML, maxMediaRefs+1)
	seen := make(map[int64]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		id, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) > maxMediaRefs {
			return ids[:maxMediaRefs], true
		}
	}
	return ids, false
}

// ContentReferencesFile сообщает, что HTML содержит точную ссылку на файл
// `/api/files/{id}` (а не, например, `/api/files/12` при id=1).
func ContentReferencesFile(contentHTML string, fileID int64) bool {
	for _, match := range mediaRefPattern.FindAllStringSubmatch(contentHTML, -1) {
		if len(match) < 2 {
			continue
		}
		id, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			continue
		}
		if id == fileID {
			return true
		}
	}
	return false
}
