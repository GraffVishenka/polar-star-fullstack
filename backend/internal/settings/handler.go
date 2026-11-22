package settings

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Handler — HTTP-слой настроек и праздников. Тонкий: разбор запроса, валидация
// формата, вызовы сервиса. Права (ADMIN) проверяет middleware маршрута.
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

// AdminGet — GET /api/admin/settings: полные настройки (ADMIN).
func (h *Handler) AdminGet(w http.ResponseWriter, r *http.Request) {
	current, err := h.svc.Settings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewSettingsDTO(current))
}

// Update — PUT /api/admin/settings: полная замена настроек (ADMIN) с
// валидацией всех полей (CAL-1..CAL-3, TICKET-1).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var req SettingsUpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	in, details := ValidateSettings(req)
	if len(details) > 0 {
		h.fail(w, r, apierr.Validation("Проверьте поля настроек", fieldDetails(details)))
		return
	}

	updated, err := h.svc.UpdateSettings(r.Context(), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewSettingsDTO(updated))
}

// Public — GET /api/public/settings: безопасное подмножество для публичного
// дашборда (имя приложения, названия и цвета статусов). Отдаётся из снапшота,
// поэтому не зависит от БД в момент запроса.
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	current := h.svc.Provider().Settings()
	h.writeJSON(w, http.StatusOK, NewPublicSettingsDTO(current))
}

// ListHolidays — GET /api/admin/holidays?year= (ADMIN).
func (h *Handler) ListHolidays(w http.ResponseWriter, r *http.Request) {
	year, ok := validateHolidayYear(r.URL.Query().Get("year"))
	if !ok {
		h.fail(w, r, apierr.Validation("Некорректный параметр year",
			map[string]any{"year": "ожидается год от 2000 до 2100"}))
		return
	}

	items, err := h.svc.Holidays(r.Context(), year)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewHolidayDTOs(items))
}

// CreateHoliday — POST /api/admin/holidays (ADMIN). Дата уникальна (CAL-1).
func (h *Handler) CreateHoliday(w http.ResponseWriter, r *http.Request) {
	date, ok := h.decodeHolidayDate(w, r)
	if !ok {
		return
	}
	holiday, err := h.svc.CreateHoliday(r.Context(), date)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, NewHolidayDTO(holiday))
}

// UpdateHoliday — PATCH /api/admin/holidays/{id}: смена даты (ADMIN).
func (h *Handler) UpdateHoliday(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	date, ok := h.decodeHolidayDate(w, r)
	if !ok {
		return
	}
	holiday, err := h.svc.UpdateHoliday(r.Context(), id, date)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, NewHolidayDTO(holiday))
}

// DeleteHoliday — DELETE /api/admin/holidays/{id} (ADMIN).
func (h *Handler) DeleteHoliday(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.svc.DeleteHoliday(r.Context(), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeHolidayDate разбирает тело {date} и валидирует дату в поясе приложения.
func (h *Handler) decodeHolidayDate(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	var req HolidayRequest
	if err := decodeJSON(w, r, &req); err != nil {
		h.fail(w, r, err)
		return time.Time{}, false
	}
	date, message := ValidateHolidayDate(req.Date, h.svc.Provider().Location())
	if message != "" {
		h.fail(w, r, apierr.Validation("Проверьте поля праздника",
			map[string]any{"date": message}))
		return time.Time{}, false
	}
	return date, true
}

// pathID читает {id} из пути; неразбираемый id — 404.
func pathID(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(chi.URLParam(r, "id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apierr.NotFound("Праздник не найден")
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
		h.logger.Error("settings: запись ответа", "error", err)
	}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierr.Respond(w, r, err)
}

// decodeJSON читает тело запроса с лимитом 1 МБ и отвергает мусор после
// объекта.
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
