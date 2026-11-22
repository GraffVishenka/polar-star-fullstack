package files

import (
	"bytes"
	"testing"
)

// Образцы сигнатур разрешённых форматов. Содержимое после сигнатуры
// произвольное: DetectMedia смотрит только первые байты.
var (
	jpegHead = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x00}, 16)...)
	pngHead  = append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00}, 16)...)
	gif87    = append([]byte("GIF87a"), bytes.Repeat([]byte{0x00}, 16)...)
	gif89    = append([]byte("GIF89a"), bytes.Repeat([]byte{0x00}, 16)...)
	// WebP: "RIFF" + размер + "WEBP".
	webpHead = append([]byte("RIFF\x24\x00\x00\x00WEBP"), bytes.Repeat([]byte{0x00}, 16)...)
	// MP4: размер бокса 0x20 + "ftyp" + brand "isom".
	mp4Head = append([]byte{0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, bytes.Repeat([]byte{0x00}, 16)...)
	// WebM: EBML-заголовок + DocType "webm".
	webmHead = append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01, 0x42, 0x82, 0x84, 'w', 'e', 'b', 'm'},
		bytes.Repeat([]byte{0x00}, 16)...)
	// Matroska использует тот же EBML-заголовок, но DocType "matroska".
	matroskaHead = append([]byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81, 0x01, 0x42, 0x82, 0x88, 'm', 'a', 't', 'r', 'o', 's', 'k', 'a'},
		bytes.Repeat([]byte{0x00}, 16)...)
)

// Б3: MIME определяется по сигнатуре (magic bytes), а не по расширению.
func TestDetectMedia_AllowedFormats(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want Media
	}{
		{"jpeg", jpegHead, Media{MIME: "image/jpeg", Ext: "jpg", Kind: KindImage}},
		{"png", pngHead, Media{MIME: "image/png", Ext: "png", Kind: KindImage}},
		{"gif87a", gif87, Media{MIME: "image/gif", Ext: "gif", Kind: KindImage}},
		{"gif89a", gif89, Media{MIME: "image/gif", Ext: "gif", Kind: KindImage}},
		{"webp", webpHead, Media{MIME: "image/webp", Ext: "webp", Kind: KindImage}},
		{"mp4", mp4Head, Media{MIME: "video/mp4", Ext: "mp4", Kind: KindVideo}},
		{"webm", webmHead, Media{MIME: "video/webm", Ext: "webm", Kind: KindVideo}},
		// Короткий, но валидный JPEG (только сигнатура).
		{"short jpeg", []byte{0xFF, 0xD8, 0xFF}, Media{MIME: "image/jpeg", Ext: "jpg", Kind: KindImage}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DetectMedia(c.head)
			if err != nil {
				t.Fatalf("DetectMedia: %v", err)
			}
			if got != c.want {
				t.Fatalf("DetectMedia = %+v, want %+v", got, c.want)
			}
		})
	}
}

// Б3: `.exe` под видом jpeg (и прочие форматы вне allowlist) → 415.
func TestDetectMedia_RejectsSpoofedAndUnknown(t *testing.T) {
	cases := []struct {
		name string
		head []byte
	}{
		{"exe (MZ) под видом jpeg", []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xFF\xFF")},
		{"elf", []byte{0x7F, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}},
		{"windows PE в середине, MZ начало", append([]byte("MZ"), bytes.Repeat([]byte{0x00}, 64)...)},
		{"html", []byte("<html><script>alert(1)</script></html>")},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		{"pdf", []byte("%PDF-1.7\n")},
		{"zip", []byte{'P', 'K', 0x03, 0x04, 0x14, 0x00}},
		// EBML, но Matroska: video/webm не должен приниматься за mkv.
		{"matroska", matroskaHead},
		// RIFF, но не WEBP (например, WAV): не фото.
		{"wav", []byte("RIFF\x24\x00\x00\x00WAVEfmt ")},
		{"пустой файл", nil},
		{"случайные байты", []byte{0x01, 0x02, 0x03, 0x04, 0x05}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := DetectMedia(c.head); err == nil {
				t.Fatalf("DetectMedia принял недопустимый формат (head=% x)", c.head)
			}
		})
	}
}

// Расширение и Content-Type клиента не участвуют в определении: решение
// принимается только по байтам.
func TestDetectMedia_IgnoresDeclaredType(t *testing.T) {
	// Валидный JPEG, который клиент мог назвать как угодно: результат — jpeg.
	media, err := DetectMedia(jpegHead)
	if err != nil {
		t.Fatalf("DetectMedia: %v", err)
	}
	if media.MIME != "image/jpeg" || media.Ext != "jpg" {
		t.Fatalf("media = %+v, want image/jpeg", media)
	}
}

// Б3: лимиты — фото 10 МБ, видео 200 МБ.
func TestMedia_Limit(t *testing.T) {
	photo := Media{MIME: "image/jpeg", Ext: "jpg", Kind: KindImage}
	video := Media{MIME: "video/mp4", Ext: "mp4", Kind: KindVideo}
	if got := photo.Limit(); got != 10<<20 {
		t.Fatalf("лимит фото = %d, want %d", got, int64(10<<20))
	}
	if got := video.Limit(); got != 200<<20 {
		t.Fatalf("лимит видео = %d, want %d", got, int64(200<<20))
	}
	// Значения констант фиксированы требованиями Б3.
	if MaxImageBytes != 10<<20 || MaxVideoBytes != 200<<20 {
		t.Fatalf("константы лимитов изменены: image=%d video=%d", MaxImageBytes, MaxVideoBytes)
	}
}

func TestSizeLimitError_Message(t *testing.T) {
	err := &SizeLimitError{Limit: MaxImageBytes, Size: MaxImageBytes + 1, Kind: KindImage}
	msg := err.Error()
	if msg == "" || !bytes.Contains([]byte(msg), []byte("10")) {
		t.Fatalf("неинформативное сообщение: %q", msg)
	}
}
