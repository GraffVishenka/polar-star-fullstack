package documents

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

const (
	docxContentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// fail отдаёт ошибку в едином формате API (DOC-3).
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) && apiErr.Status >= http.StatusInternalServerError {
		h.logger.Error("documents: ошибка выгрузки",
			"route", r.URL.Path, "status", apiErr.Status, "error", apiErr.Error())
	}
	apierr.Respond(w, r, err)
}

// writeDownload выставляет обязательные заголовки выгрузки (DOC-3).
func writeDownload(w http.ResponseWriter, contentType, filename string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, apierr.NotFound("Документ не найден")
	}
	return id, nil
}
