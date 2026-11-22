package httpserver

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// docsPage — HTML Swagger UI. Скрипты и стили подключаются с CDN (допустимо по
// заданию Б1); контракт отдаётся этим же сервером по /openapi.yaml, поэтому
// страница работает и за reverse proxy.
const docsPage = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>API · Заявки · База знаний · Оборудование</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        persistAuthorization: true,
        displayRequestDuration: true,
        tryItOutEnabled: true
      });
    };
  </script>
</body>
</html>
`

// docs отдаёт Swagger UI.
func (s *Server) docs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(docsPage))
}

// openapiSpec отдаёт openapi.yaml. Файл ищется по OPENAPI_PATH относительно
// рабочего каталога, затем каталога бинарника и его родителя (Docker кладёт
// контракт в /app/openapi/openapi.yaml).
func (s *Server) openapiSpec(w http.ResponseWriter, r *http.Request) {
	path, err := s.resolveOpenAPIPath()
	if err != nil {
		s.deps.Logger.Error("docs: не найден openapi.yaml", "error", err)
		apierr.Respond(w, r, apierr.Internal(err))
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		s.deps.Logger.Error("docs: чтение openapi.yaml", "error", err)
		apierr.Respond(w, r, apierr.Internal(err))
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) resolveOpenAPIPath() (string, error) {
	configured := s.deps.Config.OpenAPIPath
	candidates := []string{configured}
	if !filepath.IsAbs(configured) {
		// Запуск из backend/ (go run) или из корня репозитория (make up):
		// пробуем подняться на уровень/два вверх по дереву.
		candidates = append(candidates,
			filepath.Join("..", configured),
			filepath.Join("..", "..", configured),
		)
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates,
				filepath.Join(exeDir, configured),
				filepath.Join(exeDir, "..", configured),
				filepath.Join(exeDir, "..", "..", configured),
			)
		}
	}
	var lastErr error
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err != nil {
			lastErr = err
			continue
		}
		return candidate, nil
	}
	return "", lastErr
}
