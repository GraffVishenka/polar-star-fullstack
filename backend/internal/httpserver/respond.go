package httpserver

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// writeJSON отдаёт JSON-ответ. Для ответов с персональными данными handler'ы
// могут заранее выставить Cache-Control.
func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		s.deps.Logger.Error("httpserver: запись JSON-ответа", "error", err)
	}
}

// fail отдаёт ошибку в едином формате.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	apierr.Respond(w, r, err)
}

// notImplemented — заглушка замороженного маршрута контракта (Б1). Возвращает
// 501 NOT_IMPLEMENTED в едином формате ошибки, чтобы фронт мог отличить
// «ещё не сделано» от реальной ошибки.
func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	apierr.Respond(w, r, apierr.NotImplemented())
}

// decodeJSONBody читает JSON с лимитом 1 МБ (для будущих доменов).
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return apierr.BadRequest(apierr.CodeInvalidJSON, "Тело запроса пусто")
		}
		return apierr.BadRequest(apierr.CodeInvalidJSON, "Некорректный JSON в теле запроса")
	}
	return nil
}
