package documents

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/graffvishenka/polar-star-service/backend/templates"
)

func TestTicketTemplateIsValidDOCX(t *testing.T) {
	reader, err := zip.NewReader(bytes.NewReader(templates.TicketDOCX), int64(len(templates.TicketDOCX)))
	if err != nil {
		t.Fatalf("шаблон не zip: %v", err)
	}
	names := map[string]bool{}
	for _, file := range reader.File {
		names[file.Name] = true
	}
	for _, required := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml",
		"word/styles.xml", "word/_rels/document.xml.rels"} {
		if !names[required] {
			t.Fatalf("в шаблоне нет %s", required)
		}
	}
	body := readZipFile(t, reader, "word/document.xml")
	for _, placeholder := range []string{"{{app_name}}", "{{ticket_id}}", "{{comments}}", "{{generated_at}}"} {
		if !strings.Contains(body, placeholder) {
			t.Fatalf("в шаблоне нет %s", placeholder)
		}
	}
}

func TestRenderTicketDOCX_ReplacesAllValues(t *testing.T) {
	body, err := RenderTicketDOCX(templates.TicketDOCX, TicketDocument{
		AppName:       "Сервис заявок",
		TicketID:      42,
		Building:      "Здание А",
		CreatedAt:     "22.11.2026 10:00",
		ApplicantName: "Иванов И.И.",
		Cabinet:       "101",
		ProblemType:   "Электрика",
		Deadline:      "24.11.2026",
		Status:        "В работе",
		Executor:      "Петров П.П.",
		Description:   "Не работает лампа & <проверка>",
		GeneratedAt:   "22.11.2026 23:30",
		Comments: []CommentDocument{{
			CreatedAt:   "22.11.2026 11:00",
			Author:      "Петров П.П.",
			Text:        "Выехал на место",
			Attachments: []string{"photo.jpg", "video.mp4"},
		}},
	})
	if err != nil {
		t.Fatalf("RenderTicketDOCX: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("результат не zip: %v", err)
	}
	xml := readZipFile(t, reader, "word/document.xml")
	for _, value := range []string{"Сервис заявок", "42", "Здание А", "Иванов И.И.",
		"Электрика", "В работе", "Петров П.П.", "Не работает лампа &amp; &lt;проверка&gt;",
		"Выехал на место", "photo.jpg", "video.mp4", "22.11.2026 23:30"} {
		if !strings.Contains(xml, value) {
			t.Fatalf("в документе нет %q:\n%s", value, xml)
		}
	}
	if strings.Contains(xml, "{{") {
		t.Fatalf("в документе остались плейсхолдеры")
	}
}

func TestRenderTicketDOCX_NoComments(t *testing.T) {
	body, err := RenderTicketDOCX(templates.TicketDOCX, TicketDocument{TicketID: 1})
	if err != nil {
		t.Fatal(err)
	}
	reader, _ := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	xml := readZipFile(t, reader, "word/document.xml")
	if !strings.Contains(xml, "Нет комментариев") {
		t.Fatalf("нет заглушки комментариев: %s", xml)
	}
}

func readZipFile(t *testing.T, reader *zip.Reader, name string) string {
	t.Helper()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = body.Close() }()
		data, err := io.ReadAll(body)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Fatalf("в zip нет %s", name)
	return ""
}
