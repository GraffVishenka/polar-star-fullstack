package documents

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"strings"
)

// TicketDocument — данные для Word-заявки (DOC-1). Все даты уже
// отформатированы в часовом поясе приложения.
type TicketDocument struct {
	AppName       string
	TicketID      int64
	Building      string
	CreatedAt     string
	ApplicantName string
	Cabinet       string
	ProblemType   string
	Deadline      string
	Status        string
	Executor      string
	Description   string
	Comments      []CommentDocument
	GeneratedAt   string
}

// CommentDocument — комментарий с вложениями в Word-заявке.
type CommentDocument struct {
	CreatedAt   string
	Author      string
	Text        string
	Attachments []string
}

// RenderTicketDOCX подставляет данные в шаблон ticket.docx. Используется
// простая замена плейсхолдеров; комментарии вставляются отдельными абзацами,
// чтобы документ читался в Word/LibreOffice.
func RenderTicketDOCX(template []byte, document TicketDocument) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(template), int64(len(template)))
	if err != nil {
		return nil, fmt.Errorf("documents: шаблон docx: %w", err)
	}

	values := map[string]string{
		"app_name":       document.AppName,
		"ticket_id":      fmt.Sprintf("%d", document.TicketID),
		"building":       document.Building,
		"created_at":     document.CreatedAt,
		"applicant_name": document.ApplicantName,
		"cabinet":        document.Cabinet,
		"problem_type":   document.ProblemType,
		"deadline":       document.Deadline,
		"status":         document.Status,
		"executor":       document.Executor,
		"description":    document.Description,
		"generated_at":   document.GeneratedAt,
	}

	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	foundDocument := false
	for _, entry := range reader.File {
		body, err := readZipEntry(entry)
		if err != nil {
			return nil, err
		}
		if entry.Name == "word/document.xml" {
			foundDocument = true
			rendered, renderErr := renderDocumentXML(string(body), values, document.Comments)
			if renderErr != nil {
				return nil, renderErr
			}
			body = []byte(rendered)
		}
		header := entry.FileHeader
		target, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, fmt.Errorf("documents: запись %s: %w", entry.Name, err)
		}
		if _, err := target.Write(body); err != nil {
			return nil, fmt.Errorf("documents: запись %s: %w", entry.Name, err)
		}
	}
	if !foundDocument {
		return nil, fmt.Errorf("documents: в шаблоне нет word/document.xml")
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("documents: сборка docx: %w", err)
	}
	return output.Bytes(), nil
}

func readZipEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("documents: чтение %s: %w", entry.Name, err)
	}
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("documents: чтение %s: %w", entry.Name, err)
	}
	return body, nil
}

func renderDocumentXML(xmlDocument string, values map[string]string, comments []CommentDocument) (string, error) {
	for placeholder, value := range values {
		token := "{{" + placeholder + "}}"
		xmlDocument = strings.ReplaceAll(xmlDocument, token, escapeValue(value))
	}

	const commentParagraph = `<w:p><w:r><w:t xml:space="preserve">{{comments}}</w:t></w:r></w:p>`
	commentBlock := renderCommentParagraphs(comments)
	if strings.Contains(xmlDocument, commentParagraph) {
		xmlDocument = strings.Replace(xmlDocument, commentParagraph, commentBlock, 1)
	} else {
		xmlDocument = strings.ReplaceAll(xmlDocument, "{{comments}}", escapeValue("Нет комментариев"))
	}

	if strings.Contains(xmlDocument, "{{") {
		return "", fmt.Errorf("documents: в шаблоне остались неизвестные плейсхолдеры")
	}
	return xmlDocument, nil
}

func renderCommentParagraphs(comments []CommentDocument) string {
	if len(comments) == 0 {
		return wordParagraph("Нет комментариев")
	}
	var builder strings.Builder
	for _, comment := range comments {
		text := fmt.Sprintf("%s — %s: %s", comment.CreatedAt, comment.Author, comment.Text)
		if len(comment.Attachments) > 0 {
			text += " (вложения: " + strings.Join(comment.Attachments, ", ") + ")"
		}
		builder.WriteString(wordParagraph(text))
	}
	return builder.String()
}

func wordParagraph(text string) string {
	return `<w:p><w:r><w:t xml:space="preserve">` + escapeValue(text) + `</w:t></w:r></w:p>`
}

// escapeValue экранирует XML и нейтрализует случайные "{{" в пользовательских
// данных, чтобы проверка «в результате нет плейсхолдеров» была строгой.
func escapeValue(value string) string {
	value = strings.ReplaceAll(value, "{{", "{ {")
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return html.EscapeString(value)
}
