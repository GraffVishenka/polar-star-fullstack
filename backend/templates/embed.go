// Package templates содержит встроенные шаблоны документов (DOC-1/DOC-4).
package templates

import _ "embed"

// TicketDOCX — шаблон Word-заявки с плейсхолдерами {{...}}.
//
//go:embed ticket.docx
var TicketDOCX []byte
