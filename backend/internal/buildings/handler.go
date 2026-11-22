package buildings

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой домена «Здания и склады». Тонкий: разбор запроса,
// валидация формата и вызовы сервиса. Права (ADMIN на запись) проверяет
// middleware маршрута (PLAN §3.1).
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

// ListBuildings — GET /api/buildings. По умолчанию отдаёт и неактивные здания
// (EQ-1: они остаются в истории и фильтрах); include_inactive=false — только
// активные.
func (h *Handler) ListBuildings(w http.ResponseWriter, r *http.Request) {
	includeInactive := true
	if raw := strings.TrimSpace(r.URL.Query().Get("include_inactive")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			h.fail(w, r, apierr.Validation("Некорректный параметр include_inactive",
				map[string]any{"include_inactive": "ожидается true или false"}))
			return
		}
		includeInactive = value
	}

	items, err := h.svc.Buildings(r.Context(), includeInactive)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewBuildingDTOs(items))
}

// CreateBuilding — POST /api/buildings (ADMIN).
func (h *Handler) CreateBuilding(w http.ResponseWriter, r *http.Request) {
	var req CreateBuildingRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateCreateBuilding(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля здания", fieldDetails(details)))
		return
	}

	building, err := h.svc.CreateBuilding(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, NewBuildingDTO(building))
}

// UpdateBuilding — PATCH /api/buildings/{id} (ADMIN): переименование, адрес,
// деактивация is_active=false (EQ-6).
func (h *Handler) UpdateBuilding(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, buildingNotFoundMessage)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req UpdateBuildingRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	patch, details := ValidateUpdateBuilding(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля здания", fieldDetails(details)))
		return
	}

	building, err := h.svc.UpdateBuilding(r.Context(), id, patch)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewBuildingDTO(building))
}

// DeleteBuilding — DELETE /api/buildings/{id} (ADMIN). EQ-6: только пустое
// здание, иначе 409 BUILDING_IN_USE.
func (h *Handler) DeleteBuilding(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, buildingNotFoundMessage)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.DeleteBuilding(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListWarehouses — GET /api/warehouses?building_id= (чтение всем авторизованным).
func (h *Handler) ListWarehouses(w http.ResponseWriter, r *http.Request) {
	var buildingID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("building_id")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			h.fail(w, r, apierr.Validation("Некорректный параметр building_id",
				map[string]any{"building_id": "ожидается положительное целое"}))
			return
		}
		buildingID = &value
	}

	items, err := h.svc.Warehouses(r.Context(), buildingID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewWarehouseDTOs(items))
}

// CreateWarehouse — POST /api/warehouses (ADMIN).
func (h *Handler) CreateWarehouse(w http.ResponseWriter, r *http.Request) {
	var req CreateWarehouseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateCreateWarehouse(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля склада", fieldDetails(details)))
		return
	}

	warehouse, err := h.svc.CreateWarehouse(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, NewWarehouseDTO(warehouse))
}

// UpdateWarehouse — PATCH /api/warehouses/{id} (ADMIN).
func (h *Handler) UpdateWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, warehouseNotFoundMessage)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var req UpdateWarehouseRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateUpdateWarehouse(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля склада", fieldDetails(details)))
		return
	}

	warehouse, err := h.svc.UpdateWarehouse(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewWarehouseDTO(warehouse))
}

// DeleteWarehouse — DELETE /api/warehouses/{id} (ADMIN). EQ-6: склад с
// оборудованием → 409 WAREHOUSE_NOT_EMPTY.
func (h *Handler) DeleteWarehouse(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, warehouseNotFoundMessage)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.DeleteWarehouse(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pathID читает {id} из пути. Неразбираемый/неположительный id — 404: такого
// ресурса не существует.
func pathID(r *http.Request, notFoundMessage string) (int64, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apierr.NotFound(notFoundMessage)
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
		h.logger.Error("buildings: запись ответа", "error", err)
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
