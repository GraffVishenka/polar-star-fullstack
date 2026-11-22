package kb

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой базы знаний. Тонкий: разбор запроса, проверка формата,
// вызовы сервиса. Права на удаление и видимость (ADMIN) проверяет middleware
// маршрута (PLAN §3.1).
type Handler struct {
	svc    *Service
	logger *slog.Logger
	// UserFrom — источник аутентифицированного пользователя. В API это
	// auth.UserFrom; unit-тесты подменяют его, чтобы не поднимать JWT/MySQL.
	UserFrom func(context.Context) (*auth.User, bool)
}

// NewHandler создаёт HTTP-handler'ы домена.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger, UserFrom: auth.UserFrom}
}

// createRequest — тело POST /api/kb/nodes.
type createRequest struct {
	Title           string `json:"title"`
	ContentHTML     string `json:"content_html"`
	ParentID        *int64 `json:"parent_id"`
	SortOrder       *int   `json:"sort_order"`
	IsHiddenForAnon bool   `json:"is_hidden_for_anon"`
}

// updateRequest — тело PATCH /api/kb/nodes/{id}: отсутствующее поле не меняется,
// `parent_id: null` перемещает узел в корень.
type updateRequest struct {
	Title       *string       `json:"title"`
	ContentHTML *string       `json:"content_html"`
	ParentID    OptionalInt64 `json:"parent_id"`
	SortOrder   *int          `json:"sort_order"`
}

// moveRequest — тело POST /api/kb/nodes/{id}/move (KB-6).
type moveRequest struct {
	ParentID     OptionalInt64 `json:"parent_id"`
	BeforeNodeID *int64        `json:"before_node_id"`
}

// visibilityRequest — тело PATCH /api/kb/nodes/{id}/visibility (KB-7).
type visibilityRequest struct {
	IsHiddenForAnon *bool `json:"is_hidden_for_anon"`
}

// Tree — GET /api/kb/tree: полное дерево для USER/ADMIN, включая скрытые
// разделы (KB-8).
func (h *Handler) Tree(w http.ResponseWriter, r *http.Request) {
	h.tree(w, r, false)
}

// PublicTree — GET /api/public/kb/tree: только публичные корневые разделы и их
// потомки (KB-7).
func (h *Handler) PublicTree(w http.ResponseWriter, r *http.Request) {
	h.tree(w, r, true)
}

// tree — общая реализация дерева для авторизованного и публичного периметра.
func (h *Handler) tree(w http.ResponseWriter, r *http.Request, public bool) {
	tree, err := h.svc.Tree(r.Context(), public)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewKbTreeNodeDTOs(tree))
}

// Get — GET /api/kb/nodes/{id} (USER/ADMIN).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	h.getNode(w, r, false)
}

// PublicGet — GET /api/public/kb/nodes/{id}: скрытый или удалённый узел
// неотличим от несуществующего (404, KB-7).
func (h *Handler) PublicGet(w http.ResponseWriter, r *http.Request) {
	h.getNode(w, r, true)
}

// getNode — общая выдача узла.
func (h *Handler) getNode(w http.ResponseWriter, r *http.Request, public bool) {
	id, err := pathID(r, "Статья не найдена")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	node, crumbs, err := h.svc.Node(r.Context(), id, public)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewKbNodeDTO(node, crumbs))
}

// Create — POST /api/kb/nodes (USER/ADMIN): KB-1..KB-3.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req createRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	node, crumbs, err := h.svc.Create(r.Context(), actor, CreateInput{
		Title:           req.Title,
		ContentHTML:     req.ContentHTML,
		ParentID:        req.ParentID,
		SortOrder:       req.SortOrder,
		IsHiddenForAnon: req.IsHiddenForAnon,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, NewKbNodeDTO(node, crumbs))
}

// Update — PATCH /api/kb/nodes/{id} (USER/ADMIN): KB-1..KB-3, KB-6.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	id, err := pathID(r, "Статья не найдена")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req updateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	node, crumbs, err := h.svc.Update(r.Context(), actor, id, UpdateInput{
		Title:       req.Title,
		ContentHTML: req.ContentHTML,
		ParentID:    req.ParentID,
		SortOrder:   req.SortOrder,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewKbNodeDTO(node, crumbs))
}

// Move — POST /api/kb/nodes/{id}/move: drag-n-drop с переиндексацией (KB-6).
func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	id, err := pathID(r, "Статья не найдена")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req moveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	node, crumbs, err := h.svc.Move(r.Context(), actor, id, MoveInput{
		ParentID:     req.ParentID,
		BeforeNodeID: req.BeforeNodeID,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewKbNodeDTO(node, crumbs))
}

// Delete — DELETE /api/kb/nodes/{id} (ADMIN): мягкое удаление поддерева в
// корзину с числом потомков (KB-3).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	id, err := pathID(r, "Статья не найдена")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	descendants, err := h.svc.Delete(r.Context(), actor, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, KbDeleteDTO{DeletedID: id, DeletedDescendants: descendants})
}

// SetVisibility — PATCH /api/kb/nodes/{id}/visibility (ADMIN, только корень,
// KB-7).
func (h *Handler) SetVisibility(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	id, err := pathID(r, "Статья не найдена")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req visibilityRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if req.IsHiddenForAnon == nil {
		h.fail(w, r, apierr.Validation("Укажите видимость раздела",
			map[string]any{"is_hidden_for_anon": "обязательное поле"}))
		return
	}
	node, crumbs, err := h.svc.SetVisibility(r.Context(), actor, id, *req.IsHiddenForAnon)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewKbNodeDTO(node, crumbs))
}

// PublicFile — удалён из kb: публичная выдача медиа статьи обслуживается
// `GET /api/public/files/{id}` без токена (files.Service.OpenPublicKB +
// KBMediaAuthorizer, реализация — kb.Service.PublicKBFile).

// actor достаёт авторизованного пользователя (маршруты БЗ всегда за
// RequireAuth; проверка нужна для defensive-ответа 401).
func (h *Handler) actor(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user, ok := h.UserFrom(r.Context())
	if !ok {
		h.fail(w, r, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация"))
		return nil, false
	}
	return user, true
}

// pathID читает {id} из пути; неразбираемый/неположительный id — 404.
func pathID(r *http.Request, notFoundMessage string) (int64, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apierr.NotFound(notFoundMessage)
	}
	return id, nil
}

// writeJSON отдаёт JSON-ответ (формат API, PLAN §3.13).
func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("kb: запись ответа", "error", err)
	}
}

// fail отдаёт ошибку в едином формате.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) && apiErr.Status >= http.StatusInternalServerError {
		h.logger.Error("kb: ошибка запроса",
			"route", r.URL.Path, "status", apiErr.Status, "error", apiErr.Error())
	}
	apierr.Respond(w, r, err)
}

// decodeJSON читает тело с лимитом 1 МБ и отвергает мусор после объекта.
// content_html до 100 000 символов UTF-8 занимает до ~400 КБ, лимит покрывает.
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
