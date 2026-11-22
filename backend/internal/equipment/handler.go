package equipment

import (
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

// Handler — HTTP-слой домена «Оборудование». Тонкий: разбор запроса, валидация
// формата, вызовы сервиса. Роли (ADMIN на удаление) проверяет middleware.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler создаёт HTTP-handler'ы домена.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// List — GET /api/equipment: фильтры по зданию/складу/типу, поиск по названию,
// серийному и регистрационному номеру, серверная пагинация (EQ-7).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	details := map[string]string{}

	buildingID, message := ParseOptionalID(query.Get("building_id"))
	if message != "" {
		details["building_id"] = message
	}
	warehouseID, message := ParseOptionalID(query.Get("warehouse_id"))
	if message != "" {
		details["warehouse_id"] = message
	}
	equipmentTypeID, message := ParseOptionalID(query.Get("equipment_type_id"))
	if message != "" {
		details["equipment_type_id"] = message
	}

	// `search` — параметр контракта; `q` принимается как короткий алиас.
	searchRaw := query.Get("search")
	if strings.TrimSpace(searchRaw) == "" {
		searchRaw = query.Get("q")
	}
	search, message := NormalizeSearch(searchRaw)
	if message != "" {
		details["search"] = message
	}

	page, size, pageDetails := ParsePagination(query.Get("page"), query.Get("size"))
	for field, text := range pageDetails {
		details[field] = text
	}
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте параметры списка", fieldDetails(details)))
		return
	}

	result, err := h.svc.List(r.Context(), ListFilter{
		BuildingID:      buildingID,
		WarehouseID:     warehouseID,
		EquipmentTypeID: equipmentTypeID,
		Search:          search,
		Page:            page,
		Size:            size,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewListDTO(result))
}

// Get — GET /api/equipment/{id}: карточка единицы (чтение всем авторизованным).
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	item, err := h.svc.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewDTO(item))
}

// Create — POST /api/equipment (все авторизованные, EQ-2).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateCreate(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля оборудования", fieldDetails(details)))
		return
	}

	item, err := h.svc.Create(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, NewDTO(item))
}

// Update — PATCH /api/equipment/{id} (все авторизованные).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	actor, err := actorFrom(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req UpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateUpdate(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля оборудования", fieldDetails(details)))
		return
	}

	item, err := h.svc.Update(r.Context(), id, in, actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewDTO(item))
}

// Delete — DELETE /api/equipment/{id} (ADMIN): мягкое удаление в корзину.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Move — POST /api/equipment/{id}/move (EQ-3): транзакция, журнал, правила
// SAME_WAREHOUSE / BUILDING_INACTIVE.
func (h *Handler) Move(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	actor, err := actorFrom(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req MoveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateMove(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля перемещения", fieldDetails(details)))
		return
	}

	result, err := h.svc.Move(r.Context(), id, in, actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewMoveResponseDTO(result))
}

// BulkMove — POST /api/equipment/bulk-move (EQ-4): массовое перемещение 1..200
// единиц в один склад одной транзакцией, журнал по каждой перемещённой единице.
// Единицы, уже находящиеся в целевом складе, пропускаются (`skipped_count`);
// если таких все — 409 SAME_WAREHOUSE. Несуществующий id → 404 без частичных
// изменений.
func (h *Handler) BulkMove(w http.ResponseWriter, r *http.Request) {
	actor, err := actorFrom(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req BulkMoveRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateBulkMove(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля массового перемещения", fieldDetails(details)))
		return
	}

	result, err := h.svc.BulkMove(r.Context(), in, actor)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewBulkMoveResponseDTO(result))
}

// Movements — GET /api/equipment/{id}/movements: журнал по убыванию moved_at с
// именами складов/зданий/пользователя (EQ-5).
func (h *Handler) Movements(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	query := r.URL.Query()
	page, size, details := ParsePagination(query.Get("page"), query.Get("size"))
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте параметры пагинации", fieldDetails(details)))
		return
	}

	result, err := h.svc.Movements(r.Context(), id, page, size)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewMovementListDTO(result))
}

// actorFrom достаёт действующего пользователя из контекста (проставляет
// requireAuth). Роль берётся из БД, поэтому доверять ей можно.
func actorFrom(r *http.Request) (Actor, error) {
	user, ok := auth.UserFrom(r.Context())
	if !ok {
		return Actor{}, apierr.Unauthorized(apierr.CodeUnauthorized, "Требуется авторизация")
	}
	return Actor{UserID: user.ID, IsAdmin: user.IsAdmin()}, nil
}

// pathID читает {id} из пути. Неразбираемый/неположительный id — 404.
func pathID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apierr.NotFound("Оборудование не найдено")
	}
	return id, nil
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		h.logger.Error("equipment: запись ответа", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierr.Respond(w, r, err)
}

// decodeJSON читает тело запроса с лимитом 1 МБ и отвергает мусор после
// объекта, чтобы не принимались составные тела.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apierr.BadRequest(apierr.CodeInvalidJSON, "Тело запроса пусто")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apierr.New(http.StatusRequestEntityTooLarge, apierr.CodePayloadTooLarge, "Слишком большое тело запроса")
		}
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	return nil
}
