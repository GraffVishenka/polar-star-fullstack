package files

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// IDGenerator создаёт идентификатор файла на диске (по умолчанию UUIDv4).
// Вынесен в тип, чтобы unit-тесты работали детерминированно.
type IDGenerator func() (string, error)

// NewUUID — UUIDv4 (RFC 4122) из crypto/rand. Своя реализация вместо
// зависимости: нужен один вызов, а криптостойкость обеспечивает crypto/rand.
func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("files: генерация UUID: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // версия 4
	b[8] = (b[8] & 0x3f) | 0x80 // вариант RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// Storage — локальное файловое хранилище (PLAN §4: медиа на диске, выдача
// через API). Все пути внутри хранилища относительны UPLOAD_DIR; абсолютные
// пути не покидают сервер.
type Storage struct {
	dir   string
	loc   *time.Location
	clock platform.Clock
	newID IDGenerator
}

// NewStorage создаёт хранилище с корнем dir. loc — часовой пояс приложения для
// раскладки YYYY/MM (CAL-2).
func NewStorage(dir string, loc *time.Location, clock platform.Clock) *Storage {
	if clock == nil {
		clock = platform.RealClock{}
	}
	return &Storage{dir: dir, loc: loc, clock: clock, newID: NewUUID}
}

// WithIDGenerator подменяет генератор идентификаторов (unit-тесты).
func (s *Storage) WithIDGenerator(gen IDGenerator) *Storage {
	if gen != nil {
		s.newID = gen
	}
	return s
}

// Dir возвращает корень загрузок (UPLOAD_DIR).
func (s *Storage) Dir() string { return s.dir }

// Staged — файл, уже принятый на диск во временное имя. До Commit он не виден
// в БД и не отдаётся клиентам; при ошибке удаляется через Discard.
type Staged struct {
	// TempPath — абсолютный путь временного файла внутри UPLOAD_DIR: перенос
	// в итоговый каталог делается rename'ом в пределах одной ФС.
	TempPath string
	Media    Media
	Size     int64
	SHA256   string
}

// Discard удаляет временный файл (идемпотентно).
func (s *Staged) Discard() {
	if s == nil || s.TempPath == "" {
		return
	}
	_ = os.Remove(s.TempPath)
	s.TempPath = ""
}

// Stage принимает поток и записывает его на диск временным файлом, одновременно
// определяя MIME по первым 512 байтам (не по расширению), считая sha256 и
// контролируя лимит Б3: фото ≤ 10 МБ, видео ≤ 200 МБ.
//
// maxUpload — дополнительный общий предел из конфигурации (MAX_UPLOAD_BYTES,
// по умолчанию 200 МБ): если он ниже лимита типа, применяется он.
func (s *Storage) Stage(src io.Reader, maxUpload int64) (*Staged, error) {
	if src == nil {
		return nil, errors.New("files: пустой поток загрузки")
	}
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return nil, fmt.Errorf("files: создание каталога загрузок %q: %w", s.dir, err)
	}

	// Первые SniffLen байт нужны для сигнатуры; короткий файл — не ошибка,
	// просто сигнатура проверяется по тому, что пришло.
	head := make([]byte, SniffLen)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("files: чтение начала файла: %w", err)
	}
	head = head[:n]

	media, err := DetectMedia(head)
	if err != nil {
		return nil, err
	}
	limit := media.Limit()
	if maxUpload > 0 && maxUpload < limit {
		limit = maxUpload
	}
	if int64(n) > limit {
		return nil, &SizeLimitError{Limit: limit, Size: int64(n), Kind: media.Kind}
	}

	tmp, err := os.CreateTemp(s.dir, stagingPrefix+"*")
	if err != nil {
		return nil, fmt.Errorf("files: создание временного файла: %w", err)
	}
	staged := &Staged{TempPath: tmp.Name(), Media: media}

	hasher := sha256.New()
	dst := io.MultiWriter(tmp, hasher)

	// Пишем уже прочитанный заголовок, затем остаток потока. LimitReader с
	// запасом в один байт позволяет отличить «ровно лимит» от превышения.
	if _, werr := dst.Write(head); werr != nil {
		_ = tmp.Close()
		staged.Discard()
		return nil, fmt.Errorf("files: запись файла: %w", werr)
	}
	rest, copyErr := io.Copy(dst, io.LimitReader(src, limit-int64(n)+1))
	closeErr := tmp.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		staged.Discard()
		return nil, fmt.Errorf("files: запись файла: %w", copyErr)
	}

	staged.Size = int64(n) + rest
	if staged.Size > limit {
		staged.Discard()
		return nil, &SizeLimitError{Limit: limit, Size: staged.Size, Kind: media.Kind}
	}
	staged.SHA256 = hex.EncodeToString(hasher.Sum(nil))
	return staged, nil
}

// Commit переносит принятый файл в итоговый путь YYYY/MM/{uuid}.{ext}
// (правило Б3) и возвращает путь относительно UPLOAD_DIR. Расширение берётся
// из определённого MIME, а не из клиентского имени.
func (s *Storage) Commit(staged *Staged, at time.Time) (string, error) {
	if staged == nil || staged.TempPath == "" {
		return "", ErrNoUpload
	}
	id, err := s.newID()
	if err != nil {
		return "", err
	}
	rel, err := RelPath(at, s.loc, id, staged.Media.Ext)
	if err != nil {
		return "", err
	}
	full, err := ResolvePath(s.dir, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", fmt.Errorf("files: создание каталога %q: %w", filepath.Dir(full), err)
	}
	if err := os.Rename(staged.TempPath, full); err != nil {
		return "", fmt.Errorf("files: перенос файла в хранилище: %w", err)
	}
	staged.TempPath = ""
	return rel, nil
}

// Open открывает файл хранилища по относительному пути. Возвращает файл и его
// FileInfo (нужен http.ServeContent для Range и Last-Modified).
func (s *Storage) Open(rel string) (*os.File, os.FileInfo, error) {
	full, err := ResolvePath(s.dir, rel)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, nil, ErrUnsafePath
	}
	return f, info, nil
}

// Remove удаляет файл хранилища. Отсутствие файла не считается ошибкой:
// метаданные всё равно нужно вычистить (Б3, очистка orphan).
func (s *Storage) Remove(rel string) error {
	full, err := ResolvePath(s.dir, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// CleanStaleStaging удаляет временные файлы незавершённых загрузок (обрыв
// соединения, падение процесса) старше olderThan. Возвращает число удалённых.
func (s *Storage) CleanStaleStaging(olderThan time.Time) (int, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) < len(stagingPrefix) || entry.Name()[:len(stagingPrefix)] != stagingPrefix {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !info.ModTime().Before(olderThan) {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
			continue
		}
		removed++
	}
	return removed, errors.Join(errs...)
}
