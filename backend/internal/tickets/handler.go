package tickets

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой заявок: авторизованные маршруты и публичный периметр.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler создаёт handler'ы заявок.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

type createRequest struct {
	BuildingID    *int64 `json:"building_id"`
	ApplicantName string `json:"applicant_name"`
	Cabinet       string `json:"cabinet"`
	ProblemTypeID int64  `json:"problem_type_id"`
	Description   string `json:"description"`
	ExecutorID    *int64 `json:"executor_id"`
}

type publicCreateRequest struct {
	BuildingID    int64  `json:"building_id"`
	ApplicantName string `json:"applicant_name"`
	Cabinet       string `json:"cabinet"`
	ProblemTypeID int64  `json:"problem_type_id"`
	Description   string `json:"description"`
}

type updateRequest struct {
	ApplicantName *string       `json:"applicant_name"`
	Cabinet       *string       `json:"cabinet"`
	ProblemTypeID *int64        `json:"problem_type_id"`
	Description   *string       `json:"description"`
	Status        *Status       `json:"status"`
	ExecutorID    OptionalInt64 `json:"executor_id"`
	Deadline      OptionalDate  `json:"deadline"`
	BuildingID    OptionalInt64 `json:"building_id"`
}

type commentRequest struct {
	Text    string  `json:"text"`
	FileIDs []int64 `json:"file_ids"`
}

// --- Авторизованные маршруты ----------------------------------------------

// List — GET /api/tickets (TICKET-7, TICKET-15).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	filter, err := h.parseListFilter(r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result, err := h.service.List(r.Context(), actor, filter)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]TicketListItemDTO, 0, len(result.Items))
	for _, ticket := range result.Items {
		items = append(items, h.service.TicketListItem(ticket))
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": result.Total,
		"page":  result.Page,
		"size":  result.Size,
	})
}

// Create — POST /api/tickets (TICKET-2).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	var req createRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	ticket, err := h.service.Create(r.Context(), actor, CreateInput{
		BuildingID:    req.BuildingID,
		ApplicantName: req.ApplicantName,
		Cabinet:       req.Cabinet,
		ProblemTypeID: req.ProblemTypeID,
		Description:   req.Description,
		ExecutorID:    req.ExecutorID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.service.TicketDTO(ticket))
}

// Get — GET /api/tickets/{id} (TICKET-7).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ticket, err := h.service.Get(r.Context(), actor, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.service.TicketDTO(ticket))
}

// Update — PATCH /api/tickets/{id} (TICKET-8..TICKET-11).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req updateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	ticket, reset, err := h.service.Update(r.Context(), actor, id, UpdateInput{
		ApplicantName: req.ApplicantName,
		Cabinet:       req.Cabinet,
		ProblemTypeID: req.ProblemTypeID,
		Description:   req.Description,
		Status:        req.Status,
		ExecutorID:    req.ExecutorID,
		Deadline:      req.Deadline,
		BuildingID:    req.BuildingID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, TicketUpdateResponseDTO{
		Ticket:        h.service.TicketDTO(ticket),
		ExecutorReset: reset,
	})
}

// Delete — DELETE /api/tickets/{id} (TICKET-12, ADMIN).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.service.Delete(r.Context(), actor, id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListComments — GET /api/tickets/{id}/comments (TICKET-14).
func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	page, size, err := parsePagination(r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result, err := h.service.Comments(r.Context(), actor, id, page, size)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.commentListDTO(result))
}

// CreateComment — POST /api/tickets/{id}/comments (TICKET-14).
func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return
	}
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req commentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	comment, err := h.service.AddComment(r.Context(), actor, id, req.Text, req.FileIDs)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.service.CommentDTO(comment))
}

// --- Публичный периметр ----------------------------------------------------

// PublicBuildings — GET /api/public/buildings (TICKET-6).
func (h *Handler) PublicBuildings(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.PublicBuildings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]PublicBuildingDTO, 0, len(items))
	for _, item := range items {
		out = append(out, PublicBuildingDTO{
			ID:   item.ID,
			Name: item.Name,
			Counts: PublicBuildingCountsDTO{
				New:        item.New,
				InProgress: item.InProgress,
				Deferred:   item.Deferred,
			},
		})
	}
	h.writeJSON(w, http.StatusOK, out)
}

// PublicList — GET /api/public/tickets (TICKET-5, PUB-2/PUB-4).
func (h *Handler) PublicList(w http.ResponseWriter, r *http.Request) {
	buildingID, err := optionalInt64Query(r.URL.Query(), "building_id")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	board, err := h.service.PublicBoard(r.Context(), buildingID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]PublicTicketCardDTO, 0, len(board.Items))
	for _, ticket := range board.Items {
		items = append(items, h.service.PublicCard(ticket))
	}
	h.writeJSON(w, http.StatusOK, PublicTicketsResponseDTO{
		BuildingID:   board.BuildingID,
		BuildingName: board.BuildingName,
		Items:        items,
	})
}

// PublicCreate — POST /api/public/tickets (TICKET-4, TICKET-13).
func (h *Handler) PublicCreate(w http.ResponseWriter, r *http.Request) {
	var req publicCreateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	ticket, token, err := h.service.CreatePublic(r.Context(), PublicCreateInput{
		BuildingID:    req.BuildingID,
		ApplicantName: req.ApplicantName,
		Cabinet:       req.Cabinet,
		ProblemTypeID: req.ProblemTypeID,
		Description:   req.Description,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, PublicTicketCreateResponseDTO{
		Ticket:      h.service.PublicDetail(ticket, nil),
		AccessToken: token,
		AccessURL:   "/t/" + token,
	})
}

// PublicGet — GET /api/public/tickets/{token} (TICKET-13, PUB-6).
func (h *Handler) PublicGet(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	ticket, comments, err := h.service.PublicTicket(r.Context(), token)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.service.PublicDetail(ticket, comments))
}

// PublicComments — GET /api/public/tickets/{token}/comments.
func (h *Handler) PublicComments(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	page, size, err := parsePagination(r.URL.Query())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	result, err := h.service.PublicComments(r.Context(), token, page, size)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, h.commentListDTO(result))
}

// PublicAddComment — POST /api/public/tickets/{token}/comments (TICKET-14).
func (h *Handler) PublicAddComment(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	var req commentRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	comment, err := h.service.AddPublicComment(r.Context(), token, req.Text, req.FileIDs)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, h.service.CommentDTO(comment))
}

// --- Вспомогательные -------------------------------------------------------

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("tickets: запись ответа", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierr.Respond(w, r, err)
}

func (h *Handler) commentListDTO(page *CommentPage) CommentListDTO {
	items := make([]CommentDTO, 0, len(page.Items))
	for _, comment := range page.Items {
		items = append(items, h.service.CommentDTO(comment))
	}
	return CommentListDTO{Items: items, Total: page.Total, Page: page.Page, Size: page.Size}
}

// parseListFilter разбирает query-параметры TICKET-15.
func (h *Handler) parseListFilter(q url.Values) (ListFilter, error) {
	var filter ListFilter

	for _, raw := range q["status"] {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			status := Status(value)
			if !status.Valid() {
				return filter, validation("status", "неизвестный статус")
			}
			filter.Statuses = append(filter.Statuses, status)
		}
	}

	var err error
	// Ф2: `executor_id=none` — сентинел «без исполнителя» (executor_id IS NULL).
	if raw := q.Get("executor_id"); strings.EqualFold(strings.TrimSpace(raw), "none") {
		filter.ExecutorNone = true
	} else if filter.ExecutorID, err = optionalInt64Query(q, "executor_id"); err != nil {
		return filter, err
	}
	if filter.ProblemTypeID, err = optionalInt64Query(q, "problem_type_id"); err != nil {
		return filter, err
	}
	if filter.BuildingID, err = optionalInt64Query(q, "building_id"); err != nil {
		return filter, err
	}

	if raw := q.Get("date_from"); raw != "" {
		value, parseErr := h.service.ParseDate(raw)
		if parseErr != nil {
			return filter, validation("date_from", "ожидается YYYY-MM-DD")
		}
		filter.DateFrom = &value
	}
	if raw := q.Get("date_to"); raw != "" {
		value, parseErr := h.service.ParseDate(raw)
		if parseErr != nil {
			return filter, validation("date_to", "ожидается YYYY-MM-DD")
		}
		filter.DateTo = &value
	}

	filter.Search = q.Get("search")
	if filter.Search == "" {
		filter.Search = q.Get("q")
	}
	filter.Sort = q.Get("sort")
	filter.Order = q.Get("order")
	filter.Page, filter.Size, err = parsePagination(q)
	return filter, err
}

// ParseDate разбирает дату в часовом поясе приложения (используется фильтрами).
func (s *Service) ParseDate(value string) (time.Time, error) {
	return time.ParseInLocation(platform.DateLayout, strings.TrimSpace(value), s.calendar().Location())
}

func pathID(r *http.Request) (int64, error) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, validation("id", "некорректный идентификатор")
	}
	return id, nil
}

func parsePagination(q url.Values) (page, size int, err error) {
	page, size = 1, 20
	if raw := q.Get("page"); raw != "" {
		value, convErr := strconv.Atoi(raw)
		if convErr != nil || value < 1 {
			return 0, 0, validation("page", "целое число ≥ 1")
		}
		page = value
	}
	if raw := q.Get("size"); raw != "" {
		value, convErr := strconv.Atoi(raw)
		if convErr != nil || value < 1 || value > 100 {
			return 0, 0, validation("size", "целое число от 1 до 100")
		}
		size = value
	}
	return page, size, nil
}

func optionalInt64Query(q url.Values, key string) (*int64, error) {
	raw := q.Get(key)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, validation(key, "целое число > 0")
	}
	return &value, nil
}

// UnmarshalJSON различает отсутствие поля и явный null для PATCH executor_id.
func (o *OptionalInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

// UnmarshalJSON различает отсутствие поля и явный null для PATCH deadline.
func (o *OptionalDate) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apierr.BadRequest(apierr.CodeInvalidJSON, "Тело запроса пусто")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge,
				"Слишком большое тело запроса")
		}
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	return nil
}
