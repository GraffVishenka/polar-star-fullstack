package documents

import (
	"archive/zip"
	"bytes"
	"fmt"
)

// Ticket document template. The .docx is built from primitives below and the
// generated file lives in backend/templates/ticket.docx (embedded into the
// binary); placeholders are replaced at request time without a template engine.
const ticketDocumentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Title"/></w:pPr><w:r><w:t xml:space="preserve">{{app_name}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Заявка № {{ticket_id}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Здание: {{building}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Создана: {{created_at}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Заявитель: {{applicant_name}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Кабинет: {{cabinet}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Вид проблемы: {{problem_type}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Дедлайн: {{deadline}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Статус: {{status}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Исполнитель: {{executor}}</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Описание</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">{{description}}</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Комментарии</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">{{comments}}</w:t></w:r></w:p>
    <w:p><w:r><w:t xml:space="preserve">Сформировано: {{generated_at}}</w:t></w:r></w:p>
    <w:sectPr>
      <w:pgSz w:w="11906" w:h="16838"/>
      <w:pgMar w:top="1134" w:right="850" w:bottom="1134" w:left="1701" w:header="708" w:footer="708" w:gutter="0"/>
    </w:sectPr>
  </w:body>
</w:document>`

const ticketStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:docDefaults>
    <w:rPrDefault><w:rPr><w:rFonts w:ascii="Calibri" w:hAnsi="Calibri" w:cs="Calibri"/><w:sz w:val="22"/></w:rPr></w:rPrDefault>
    <w:pPrDefault><w:pPr><w:spacing w:after="120" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault>
  </w:docDefaults>
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
  <w:style w:type="paragraph" w:styleId="Title">
    <w:name w:val="Title"/>
    <w:basedOn w:val="Normal"/>
    <w:rPr><w:b/><w:sz w:val="32"/></w:rPr>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Heading1">
    <w:name w:val="heading 1"/>
    <w:basedOn w:val="Normal"/>
    <w:rPr><w:b/><w:sz w:val="26"/></w:rPr>
  </w:style>
</w:styles>`

const ticketContentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`

const ticketRootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`

const ticketDocumentRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

const ticketCoreXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
 xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/"
 xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>Заявка</dc:title>
  <dc:creator>polar-star-service</dc:creator>
  <cp:lastModifiedBy>polar-star-service</cp:lastModifiedBy>
</cp:coreProperties>`

const ticketAppXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Application>polar-star-service</Application>
</Properties>`

// BuildTicketTemplateDOCX собирает шаблон Word-заявки без внешних библиотек.
// Файл коммитится в backend/templates/ticket.docx; runtime читает его через
// go:embed. Плейсхолдеры перечислены в docs/templates.md.
func BuildTicketTemplateDOCX() ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entries := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", ticketContentTypesXML},
		{"_rels/.rels", ticketRootRelsXML},
		{"docProps/core.xml", ticketCoreXML},
		{"docProps/app.xml", ticketAppXML},
		{"word/document.xml", ticketDocumentXML},
		{"word/styles.xml", ticketStylesXML},
		{"word/_rels/document.xml.rels", ticketDocumentRelsXML},
	}
	for _, entry := range entries {
		writer, err := archive.Create(entry.name)
		if err != nil {
			return nil, fmt.Errorf("documents: шаблон %s: %w", entry.name, err)
		}
		if _, err := writer.Write([]byte(entry.body)); err != nil {
			return nil, fmt.Errorf("documents: шаблон %s: %w", entry.name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("documents: шаблон: %w", err)
	}
	return buffer.Bytes(), nil
}
