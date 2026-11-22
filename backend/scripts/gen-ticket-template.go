//go:build ignore

// Генератор шаблона backend/templates/ticket.docx (DOC-1).
//
// Запуск из backend:  go run ./scripts/gen-ticket-template.go
// Файл коммитится; runtime использует go:embed, а не генерацию на старте.
package main

import (
	"fmt"
	"os"

	"github.com/graffvishenka/polar-star-service/backend/internal/documents"
)

func main() {
	body, err := documents.BuildTicketTemplateDOCX()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate ticket template: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile("templates/ticket.docx", body, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write ticket template: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("templates/ticket.docx generated")
}
