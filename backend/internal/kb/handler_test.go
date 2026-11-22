package kb

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
)

// newTestHandler собирает handler с фейковым сервисом и подменённым источником
// пользователя (без MySQL и JWT).
func newTestHandler(t *testing.T) (*Handler, *fakeRepo, *fakeMedia) {
	t.Helper()
	svc, repo, media := newTestService(t)
	handler := NewHandler(svc, discardLogger())
	handler.UserFrom = func(context.Context) (*auth.User, bool) { return user(7), true }
	return handler, repo, media
}

// testRouter повторяет регистрацию маршрутов БЗ из httpserver (префикс /api).
func testRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/api", func(api chi.Router) {
		api.Get("/public/kb/tree", h.PublicTree)
		api.Get("/public/kb/nodes/{id}", h.PublicGet)
		api.Get("/kb/tree", h.Tree)
		api.Get("/kb/nodes/{id}", h.Get)
		api.Post("/kb/nodes", h.Create)
		api.Patch("/kb/nodes/{id}", h.Update)
		api.Post("/kb/nodes/{id}/move", h.Move)
		api.Delete("/kb/nodes/{id}", h.Delete)
		api.Patch("/kb/nodes/{id}/visibility", h.SetVisibility)
	})
	return r
}

// jsonRequest формирует JSON-запрос.
func jsonRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// decodeNode разбирает KbNodeDTO.
func decodeNode(t *testing.T, rec *httptest.ResponseRecorder) KbNodeDTO {
	t.Helper()
	var node KbNodeDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &node); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	return node
}

// POST /api/kb/nodes: 201, санитизация и хлебные крошки в ответе.
func TestHandler_CreateAndGet(t *testing.T) {
	h, _, _ := newTestHandler(t)
	router := testRouter(h)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(t, http.MethodPost, "/api/kb/nodes", map[string]any{
		"title":        "Инструкция",
		"content_html": `<p>Текст</p><script>alert(1)</script>`,
	}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d (%s)", rec.Code, rec.Body.String())
	}
	created := decodeNode(t, rec)
	if created.ID == 0 || created.Title != "Инструкция" {
		t.Fatalf("created = %+v", created)
	}
	if bytes.Contains([]byte(created.ContentHTML), []byte("script")) {
		t.Fatalf("content_html не санитизирован: %s", created.ContentHTML)
	}
	if len(created.Breadcrumbs) != 1 || created.Breadcrumbs[0].ID != created.ID {
		t.Fatalf("breadcrumbs = %v", created.Breadcrumbs)
	}

	// GET узла.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/kb/nodes/"+strconv.FormatInt(created.ID, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}
	if got := decodeNode(t, rec); got.ID != created.ID {
		t.Fatalf("got = %+v", got)
	}

	// Некорректный/несуществующий id → 404.
	for _, path := range []string{"/api/kb/nodes/abc", "/api/kb/nodes/0", "/api/kb/nodes/9999"} {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

// KB-7: публичное дерево не содержит скрытый корень, публичный узел скрытого
// раздела — 404.
func TestHandler_PublicTreeAndNode(t *testing.T) {
	h, repo, _ := newTestHandler(t)
	router := testRouter(h)

	publicRoot := repo.seed(Node{Title: "Публичный", SortOrder: 10})
	publicChild := repo.seed(Node{Title: "Потомок", SortOrder: 10, ParentID: ptr(publicRoot.ID)})
	hiddenRoot := repo.seed(Node{Title: "Скрытый", SortOrder: 20, IsHiddenForAnon: true})
	hiddenChild := repo.seed(Node{Title: "Скрытый потомок", SortOrder: 10, ParentID: ptr(hiddenRoot.ID)})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/kb/tree", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public tree status = %d", rec.Code)
	}
	var publicTree []KbTreeNodeDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &publicTree); err != nil {
		t.Fatalf("ответ не JSON: %v (%s)", err, rec.Body.String())
	}
	if len(publicTree) != 1 || publicTree[0].ID != publicRoot.ID {
		t.Fatalf("публичные корни = %+v", publicTree)
	}
	if len(publicTree[0].Children) != 1 || publicTree[0].Children[0].ID != publicChild.ID {
		t.Fatalf("дети = %+v", publicTree[0].Children)
	}

	// Скрытый узел и его потомок недоступны анониму.
	for _, id := range []int64{hiddenRoot.ID, hiddenChild.ID} {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/kb/nodes/"+strconv.FormatInt(id, 10), nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("скрытый узел %d: %d, want 404", id, rec.Code)
		}
	}
	// Публичный узел отдаётся.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/kb/nodes/"+strconv.FormatInt(publicChild.ID, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("публичный узел: %d (%s)", rec.Code, rec.Body.String())
	}

	// Авторизованное дерево содержит и скрытый раздел.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/kb/tree", nil))
	var authorized []KbTreeNodeDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &authorized); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if len(authorized) != 2 {
		t.Fatalf("корней для авторизованного = %d, want 2", len(authorized))
	}
}

// PATCH /move: переиндексация и ответ с новым sort_order.
func TestHandler_Move(t *testing.T) {
	h, _, _ := newTestHandler(t)
	router := testRouter(h)

	create := func(title string, parent *int64) KbNodeDTO {
		rec := httptest.NewRecorder()
		body := map[string]any{"title": title}
		if parent != nil {
			body["parent_id"] = *parent
		}
		router.ServeHTTP(rec, jsonRequest(t, http.MethodPost, "/api/kb/nodes", body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %q: %d (%s)", title, rec.Code, rec.Body.String())
		}
		return decodeNode(t, rec)
	}
	root := create("Корень", nil)
	a := create("A", &root.ID)
	b := create("B", &root.ID)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(t, http.MethodPost,
		"/api/kb/nodes/"+strconv.FormatInt(b.ID, 10)+"/move",
		map[string]any{"parent_id": root.ID, "before_node_id": a.ID}))
	if rec.Code != http.StatusOK {
		t.Fatalf("move status = %d (%s)", rec.Code, rec.Body.String())
	}
	moved := decodeNode(t, rec)
	if moved.SortOrder != 10 {
		t.Fatalf("sort_order = %d, want 10", moved.SortOrder)
	}

	// Цикл: корень под собственного ребёнка → 409 CYCLE_DETECTED.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(t, http.MethodPost,
		"/api/kb/nodes/"+strconv.FormatInt(root.ID, 10)+"/move",
		map[string]any{"parent_id": a.ID}))
	if rec.Code != http.StatusConflict {
		t.Fatalf("цикл: %d (%s), want 409", rec.Code, rec.Body.String())
	}
	var errBody struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &errBody)
	if errBody.Code != "CYCLE_DETECTED" {
		t.Fatalf("code = %q", errBody.Code)
	}
}

// DELETE: ответ с числом потомков; повторное удаление — 404.
func TestHandler_Delete(t *testing.T) {
	h, _, _ := newTestHandler(t)
	router := testRouter(h)

	root := mustCreate(t, h.svc, CreateInput{Title: "Корень"})
	_ = mustCreate(t, h.svc, CreateInput{Title: "Ребёнок 1", ParentID: ptr(root.ID)})
	child2 := mustCreate(t, h.svc, CreateInput{Title: "Ребёнок 2", ParentID: ptr(root.ID)})
	_ = mustCreate(t, h.svc, CreateInput{Title: "Внук", ParentID: ptr(child2.ID)})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/kb/nodes/"+strconv.FormatInt(root.ID, 10), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d (%s)", rec.Code, rec.Body.String())
	}
	var result KbDeleteDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	if result.DeletedID != root.ID || result.DeletedDescendants != 3 {
		t.Fatalf("result = %+v, want id=%d descendants=3", result, root.ID)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/kb/nodes/"+strconv.FormatInt(root.ID, 10), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("повторное удаление: %d, want 404", rec.Code)
	}
}

// PATCH /visibility: только корень и обязательное поле.
func TestHandler_SetVisibility(t *testing.T) {
	h, _, _ := newTestHandler(t)
	router := testRouter(h)

	root := mustCreate(t, h.svc, CreateInput{Title: "Корень"})
	child := mustCreate(t, h.svc, CreateInput{Title: "Ребёнок", ParentID: ptr(root.ID)})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(t, http.MethodPatch,
		"/api/kb/nodes/"+strconv.FormatInt(root.ID, 10)+"/visibility",
		map[string]any{"is_hidden_for_anon": true}))
	if rec.Code != http.StatusOK {
		t.Fatalf("visibility status = %d (%s)", rec.Code, rec.Body.String())
	}
	if !decodeNode(t, rec).IsHiddenForAnon {
		t.Fatal("видимость не изменилась")
	}

	// Потомок → 422.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(t, http.MethodPatch,
		"/api/kb/nodes/"+strconv.FormatInt(child.ID, 10)+"/visibility",
		map[string]any{"is_hidden_for_anon": true}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("потомок: %d, want 422", rec.Code)
	}

	// Поле не передано → 422.
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(t, http.MethodPatch,
		"/api/kb/nodes/"+strconv.FormatInt(root.ID, 10)+"/visibility", map[string]any{}))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("без поля: %d, want 422", rec.Code)
	}
}

// Некорректный JSON и неизвестные поля не ломают обработку.
func TestHandler_BadJSON(t *testing.T) {
	h, _, _ := newTestHandler(t)
	router := testRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/api/kb/nodes", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
