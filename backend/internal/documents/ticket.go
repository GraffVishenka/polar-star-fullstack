package documents

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
	"github.com/graffvishenka/polar-star-service/backend/internal/tickets"
	"github.com/graffvishenka/polar-star-service/backend/templates"
)

const commentPageSize = 100

// TicketDOCX — GET /api/tickets/{id}/export.docx (DOC-1): USER — только своё
// здание, ADMIN — любое; права проверяет tickets.Service.Get.
func (h *Handler) TicketDOCX(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		h.fail(w, r, apierr.NotFound("Заявка не найдена"))
		return
	}
	ticket, err := h.tickets.Get(r.Context(), actor, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	comments, err := h.allCommentsForActor(r.Context(), actor, ticket.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeTicketDOCX(w, r, ticket, comments)
}

// PublicTicketDOCX — GET /api/public/tickets/{token}/export.docx (DOC-1):
// доступ только к заявке своего бессрочного токена.
func (h *Handler) PublicTicketDOCX(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		h.fail(w, r, apierr.NotFound("Заявка не найдена"))
		return
	}
	ticket, _, err := h.tickets.PublicTicket(r.Context(), token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	comments, err := h.allPublicComments(r.Context(), token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeTicketDOCX(w, r, ticket, comments)
}

func (h *Handler) allCommentsForActor(ctx context.Context, actor *auth.User, ticketID int64) ([]*tickets.Comment, error) {
	return h.collectComments(func(page int) (*tickets.CommentPage, error) {
		return h.tickets.Comments(ctx, actor, ticketID, page, commentPageSize)
	})
}

func (h *Handler) allPublicComments(ctx context.Context, token string) ([]*tickets.Comment, error) {
	return h.collectComments(func(page int) (*tickets.CommentPage, error) {
		return h.tickets.PublicComments(ctx, token, page, commentPageSize)
	})
}

func (h *Handler) collectComments(fetch func(page int) (*tickets.CommentPage, error)) ([]*tickets.Comment, error) {
	var all []*tickets.Comment
	for page := 1; ; page++ {
		result, err := fetch(page)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Items...)
		if len(all) >= result.Total || len(result.Items) == 0 {
			return all, nil
		}
	}
}

func (h *Handler) writeTicketDOCX(w http.ResponseWriter, r *http.Request, ticket *tickets.Ticket, comments []*tickets.Comment) {
	body, err := RenderTicketDOCX(templates.TicketDOCX, h.ticketDocument(ticket, comments))
	if err != nil {
		h.fail(w, r, apierr.Internal(err))
		return
	}
	writeDownload(w, docxContentType, fmt.Sprintf("ticket_%d.docx", ticket.ID), body)
}

func (h *Handler) ticketDocument(ticket *tickets.Ticket, comments []*tickets.Comment) TicketDocument {
	loc := h.location()
	generatedAt := h.clock.Now().In(loc).Format("02.01.2006 15:04")
	document := TicketDocument{
		AppName:       h.appName(),
		TicketID:      ticket.ID,
		Building:      fallback(ticket.BuildingName),
		CreatedAt:     ticket.CreatedAt.In(loc).Format("02.01.2006 15:04"),
		ApplicantName: fallback(ticket.ApplicantName),
		Cabinet:       fallback(ticket.Cabinet),
		ProblemType:   fallback(ticket.ProblemTypeName),
		Deadline:      "—",
		Status:        fallback(h.tickets.StatusLabel(ticket.Status)),
		Description:   fallback(ticket.Description),
		GeneratedAt:   generatedAt,
	}
	if ticket.Deadline != nil {
		document.Deadline = ticket.Deadline.In(loc).Format("02.01.2006")
	}
	if ticket.ExecutorName != nil {
		document.Executor = *ticket.ExecutorName
	} else {
		document.Executor = "—"
	}
	for _, comment := range comments {
		attachments := make([]string, 0, len(comment.Files))
		for _, file := range comment.Files {
			attachments = append(attachments, file.Filename)
		}
		document.Comments = append(document.Comments, CommentDocument{
			CreatedAt:   comment.CreatedAt.In(loc).Format("02.01.2006 15:04"),
			Author:      fallback(comment.AuthorName),
			Text:        fallback(comment.Body),
			Attachments: attachments,
		})
	}
	return document
}

func fallback(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	return value
}
