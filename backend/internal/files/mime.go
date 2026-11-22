package files

import (
	"bytes"
)

// Лимиты Б3: фото ≤ 10 МБ, видео ≤ 200 МБ.
const (
	// MaxImageBytes — предел для фото (jpeg/png/webp/gif).
	MaxImageBytes int64 = 10 << 20
	// MaxVideoBytes — предел для видео (mp4/webm).
	MaxVideoBytes int64 = 200 << 20
	// SniffLen — сколько первых байт достаточно для определения сигнатуры.
	// Значение совпадает с рекомендацией http.DetectContentType (512 байт),
	// но MIME мы определяем сами по allowlist'у, а не доверяем эвристике.
	SniffLen = 512
)

// Kind — класс медиа, определяющий лимит размера.
type Kind string

const (
	// KindImage — фото.
	KindImage Kind = "image"
	// KindVideo — видео.
	KindVideo Kind = "video"
)

// Media — результат определения формата по сигнатуре.
type Media struct {
	// MIME — канонический MIME-тип (image/jpeg, image/png, image/gif,
	// image/webp, video/mp4, video/webm).
	MIME string
	// Ext — каноническое расширение для имени файла на диске (без точки).
	// Клиентское имя не используется: расширение выводится из сигнатуры.
	Ext  string
	Kind Kind
}

// Limit возвращает предельный размер файла для этого типа медиа (Б3).
func (m Media) Limit() int64 {
	if m.Kind == KindVideo {
		return MaxVideoBytes
	}
	return MaxImageBytes
}

// Сигнатуры (magic bytes) разрешённых форматов.
var (
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	ebmlMagic = []byte{0x1A, 0x45, 0xDF, 0xA3} // EBML, контейнер WebM/Matroska
	gif87a    = []byte("GIF87a")
	gif89a    = []byte("GIF89a")
	riffMagic = []byte("RIFF")
	webpMagic = []byte("WEBP")
	ftypMagic = []byte("ftyp") // ISO BMFF (MP4): размер(4) + 'ftyp'
	webmDoc   = []byte("webm") // DocType EBML
)

// DetectMedia определяет формат файла по первым байтам (magic bytes), а не по
// расширению или клиентскому Content-Type (Б3: `.exe` под видом jpeg → 415).
//
// Allowlist намеренно узкий: jpeg, png, gif, webp, mp4, webm. Всё остальное
// (включая SVG, HTML и исполняемые файлы) отклоняется как ErrUnsupportedMedia.
func DetectMedia(head []byte) (Media, error) {
	switch {
	case isJPEG(head):
		return Media{MIME: "image/jpeg", Ext: "jpg", Kind: KindImage}, nil
	case bytes.HasPrefix(head, pngMagic):
		return Media{MIME: "image/png", Ext: "png", Kind: KindImage}, nil
	case bytes.HasPrefix(head, gif87a), bytes.HasPrefix(head, gif89a):
		return Media{MIME: "image/gif", Ext: "gif", Kind: KindImage}, nil
	case isWebP(head):
		return Media{MIME: "image/webp", Ext: "webp", Kind: KindImage}, nil
	case isMP4(head):
		return Media{MIME: "video/mp4", Ext: "mp4", Kind: KindVideo}, nil
	case isWebM(head):
		return Media{MIME: "video/webm", Ext: "webm", Kind: KindVideo}, nil
	}
	return Media{}, ErrUnsupportedMedia
}

// isJPEG — SOI-маркер JPEG: FF D8 FF.
func isJPEG(head []byte) bool {
	return len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF
}

// isWebP — RIFF-контейнер с четырёхсимвольным типом WEBP:
// "RIFF" + размер(4) + "WEBP".
func isWebP(head []byte) bool {
	return len(head) >= 12 && bytes.HasPrefix(head, riffMagic) && bytes.Equal(head[8:12], webpMagic)
}

// isMP4 — ISO BMFF: 4-байтовый размер бокса, затем 'ftyp'. Проверяем и
// осмысленность размера бокса, чтобы не принять случайные данные за видео.
func isMP4(head []byte) bool {
	if len(head) < 12 || !bytes.Equal(head[4:8], ftypMagic) {
		return false
	}
	boxSize := uint32(head[0])<<24 | uint32(head[1])<<16 | uint32(head[2])<<8 | uint32(head[3])
	return boxSize >= 16
}

// isWebM — EBML-заголовок с DocType "webm". Matroska (mkv) имеет тот же
// EBML-заголовок, но другой DocType, поэтому он не проходит allowlist.
func isWebM(head []byte) bool {
	return bytes.HasPrefix(head, ebmlMagic) && bytes.Contains(head, webmDoc)
}
