package files

import "time"

// FileInfo — представление файла в API (компонент FileInfo в openapi.yaml).
// Поле URL заполняет HTTP-слой: авторизованным — `/api/files/{id}`, анониму —
// `/api/public/files/{id}?token=...` (TICKET-13).
type FileInfo struct {
	ID        int64     `json:"id"`
	Filename  string    `json:"filename"`
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	Scope     Scope     `json:"scope"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// NewFileInfo конвертирует доменную модель в DTO. URL проставляет вызывающий:
// в домене нет знаний о публичном маршруте и токене.
func NewFileInfo(f *File) FileInfo {
	if f == nil {
		return FileInfo{}
	}
	return FileInfo{
		ID:        f.ID,
		Filename:  f.OriginalName,
		MimeType:  f.MIME,
		Size:      f.SizeBytes,
		Scope:     f.Scope,
		CreatedAt: f.CreatedAt,
	}
}
