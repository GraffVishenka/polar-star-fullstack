package files

import (
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Внутренние ограничения имён.
const (
	// maxOriginalNameRunes — files.original_name VARCHAR(255) в utf8mb4:
	// MySQL считает длину в символах, поэтому ограничиваем по рунам.
	maxOriginalNameRunes = 255
	// fallbackName — имя, если клиент не передал ничего осмысленного.
	fallbackName = "file"
	// stagingPrefix — префикс временных файлов незавершённых загрузок.
	stagingPrefix = ".staging-"
)

// SafeOriginalName приводит клиентское имя файла к безопасному виду для
// хранения в БД и выдачи в Content-Disposition.
//
// Защита от path traversal (Б3): убираются каталоги (`/`, `\`), элементы «..»,
// управляющие и NUL-символы. Результат никогда не содержит разделителей пути,
// поэтому не может выйти за пределы UPLOAD_DIR.
func SafeOriginalName(name string) string {
	// Windows-разделители приводим к прямым, затем берём только базовое имя —
	// так «..\..\etc\passwd» и «/etc/passwd» превращаются в «passwd».
	normalized := strings.ReplaceAll(name, `\`, "/")
	base := path.Base(normalized)

	// Удаляем NUL и прочие управляющие символы (в имени файла их быть не может,
	// но они ломают заголовки и файловые API).
	base = strings.Map(func(r rune) rune {
		if r == 0 || unicode.IsControl(r) {
			return -1
		}
		return r
	}, base)
	base = strings.TrimSpace(base)
	// Точки в конце и в начале недопустимы: «.»/«..» — специальные элементы,
	// а имя, оканчивающееся точкой, на Windows не создаётся.
	base = strings.Trim(base, " .")
	if base == "" || base == "." || base == ".." {
		return fallbackName
	}

	runes := []rune(base)
	if len(runes) > maxOriginalNameRunes {
		// Обрезаем по рунам с сохранением расширения, если оно есть.
		if ext := path.Ext(base); len(ext) > 0 && len([]rune(ext)) < maxOriginalNameRunes {
			stemRunes := maxOriginalNameRunes - len([]rune(ext))
			base = string(runes[:stemRunes]) + ext
		} else {
			base = string(runes[:maxOriginalNameRunes])
		}
	}
	return base
}

// StoredName формирует имя файла на диске: {id}.{ext}. Идентификатор —
// случайный UUIDv4, расширение — каноническое для определённого MIME, поэтому
// клиентское имя не влияет на путь (защита от traversal и подмены типа).
func StoredName(id, ext string) (string, error) {
	if !validStoredExt(ext) {
		return "", ErrUnsafePath
	}
	if !validStoredID(id) {
		return "", ErrUnsafePath
	}
	return id + "." + ext, nil
}

// RelPath строит путь относительно UPLOAD_DIR по правилу Б3:
// YYYY/MM/{uuid}.{ext}. Год и месяц берутся в часовом поясе приложения
// (CAL-2, NFR-7): загрузка в 23:50 по Екатеринбургу попадает в текущие сутки
// этого пояса, а не UTC.
func RelPath(at time.Time, loc *time.Location, id, ext string) (string, error) {
	name, err := StoredName(id, ext)
	if err != nil {
		return "", err
	}
	if loc == nil {
		loc = time.UTC
	}
	local := at.In(loc)
	return path.Join(local.Format("2006"), local.Format("01"), name), nil
}

// validStoredID разрешает только UUID-подобный идентификатор: это исключает
// попадание разделителей пути и специальных элементов из БД/вызова.
func validStoredID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHex(r) {
				return false
			}
		}
	}
	return true
}

// validStoredExt — allowlist расширений, выводимых из MIME (DetectMedia).
func validStoredExt(ext string) bool {
	switch ext {
	case "jpg", "png", "gif", "webp", "mp4", "webm":
		return true
	default:
		return false
	}
}

// isHex проверяет шестнадцатеричный символ.
func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// ResolvePath превращает относительный путь из БД в путь внутри UPLOAD_DIR и
// проверяет, что результат не выходит за пределы корня загрузок (защита от
// path traversal).
//
// Проверка двойная: отклоняются абсолютные пути, элементы «..» и тома Windows,
// а затем результат проверяется по префиксу нормализованного корня — это ловит
// и Windows-специфику (`\`, диск), и пути, пришедшие из БД с другим
// разделителем.
func ResolvePath(baseDir, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", ErrUnsafePath
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if !isInsideRel(clean) {
		return "", ErrUnsafePath
	}

	root := filepath.Clean(baseDir)
	if root == "." {
		// Относительный корень («./data/uploads»): clean уже проверен, выйти
		// за пределы рабочего каталога элементами «..» нельзя.
		return clean, nil
	}
	full := filepath.Join(root, clean)
	if !withinRoot(root, full) {
		return "", ErrUnsafePath
	}
	return full, nil
}

// isInsideRel сообщает, что очищенный относительный путь не выходит наружу.
func isInsideRel(clean string) bool {
	if clean == "" || clean == "." || clean == ".." {
		return false
	}
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return false
	}
	// Проверяем оба разделителя: на Windows filepath.Separator — '\', но путь
	// мог прийти и со слешами (или наоборот, если запущено на другой ОС).
	return !strings.HasPrefix(clean, ".."+string(filepath.Separator)) &&
		!strings.HasPrefix(clean, "../") &&
		!strings.HasPrefix(clean, `..\`)
}

// withinRoot проверяет, что full лежит внутри root (а не совпадает с ним и не
// является соседним каталогом с общим префиксом).
func withinRoot(root, full string) bool {
	if root == "" || full == root {
		return false
	}
	return strings.HasPrefix(full, root+"/") || strings.HasPrefix(full, root+`\`)
}
