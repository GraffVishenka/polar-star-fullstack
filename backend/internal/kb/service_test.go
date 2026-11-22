package kb

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/files"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// user — авторизованный сотрудник для тестов.
func user(id int64) *auth.User { return &auth.User{ID: id, Role: auth.RoleUser} }

// apiError достаёт *apierr.Error из ошибки сервиса.
func apiError(t *testing.T, err error) *apierr.Error {
	t.Helper()
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("ожидалась ошибка API, получено %v", err)
	}
	return apiErr
}

// mustCreate создаёт узел и падает при ошибке.
func mustCreate(t *testing.T, svc *Service, in CreateInput) *Node {
	t.Helper()
	node, _, err := svc.Create(context.Background(), user(7), in)
	if err != nil {
		t.Fatalf("Create(%q): %v", in.Title, err)
	}
	return node
}

// KB-3/KB-4/KB-5: создание корня — санитизация, проверка медиа, отметка linked.
func TestService_CreateRoot(t *testing.T) {
	svc, repo, media := newTestService(t)
	media.scopes[7] = files.ScopeKB

	content := `<p>Текст</p><script>alert(1)</script><img src="/api/files/7" alt="схема">`
	node, crumbs, err := svc.Create(context.Background(), user(7), CreateInput{
		Title:           "  Раздел  ",
		ContentHTML:     content,
		IsHiddenForAnon: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if node.Title != "Раздел" {
		t.Fatalf("title = %q", node.Title)
	}
	if strings.Contains(node.ContentHTML, "script") {
		t.Fatalf("content_html не санитизирован: %s", node.ContentHTML)
	}
	if !strings.Contains(node.ContentHTML, `src="/api/files/7"`) {
		t.Fatalf("медиа потеряно: %s", node.ContentHTML)
	}
	if !node.IsHiddenForAnon {
		t.Fatal("корневой раздел должен сохранять is_hidden_for_anon")
	}
	if node.SortOrder != SortOrderStep {
		t.Fatalf("sort_order = %d, want %d", node.SortOrder, SortOrderStep)
	}
	if node.CreatedBy == nil || *node.CreatedBy != 7 {
		t.Fatalf("created_by = %v", node.CreatedBy)
	}
	if len(crumbs) != 1 || crumbs[0].ID != node.ID {
		t.Fatalf("breadcrumbs = %v", crumbs)
	}
	if len(media.linked) != 1 || media.linked[0] != 7 {
		t.Fatalf("MarkLinked не вызван: %v", media.linked)
	}
	stored, err := repo.Get(context.Background(), node.ID)
	if err != nil {
		t.Fatalf("repo.Get: %v", err)
	}
	if stored.Title != "Раздел" {
		t.Fatalf("узел не сохранён: %+v", stored)
	}
}

// KB-1: у потомков is_hidden_for_anon игнорируется (видимость наследуется от
// корня), sort_order — в конец списка детей (max+10).
func TestService_CreateChild(t *testing.T) {
	svc, _, _ := newTestService(t)
	root := mustCreate(t, svc, CreateInput{Title: "Корень"})
	first := mustCreate(t, svc, CreateInput{Title: "Первый", ParentID: ptr(root.ID)})
	second := mustCreate(t, svc, CreateInput{
		Title:           "Второй",
		ParentID:        ptr(root.ID),
		IsHiddenForAnon: true, // должно игнорироваться
	})
	if second.IsHiddenForAnon {
		t.Fatal("is_hidden_for_anon у потомка должен игнорироваться")
	}
	if first.SortOrder != SortOrderStep || second.SortOrder != 2*SortOrderStep {
		t.Fatalf("sort_order: first=%d second=%d", first.SortOrder, second.SortOrder)
	}
	_, crumbs, err := svc.Node(context.Background(), second.ID, false)
	if err != nil {
		t.Fatalf("Node: %v", err)
	}
	if len(crumbs) != 2 || crumbs[0].ID != root.ID || crumbs[1].ID != second.ID {
		t.Fatalf("breadcrumbs = %v", crumbs)
	}
}

// KB-2: родитель не найден → 404, родитель в корзине → 400.
func TestService_CreateParentErrors(t *testing.T) {
	svc, repo, _ := newTestService(t)

	_, _, err := svc.Create(context.Background(), user(1), CreateInput{Title: "X", ParentID: ptr(int64(404))})
	if status := apiError(t, err).Status; status != http.StatusNotFound {
		t.Fatalf("нет родителя: status = %d, want 404", status)
	}

	deleted := repo.seed(Node{Title: "Удалённый"})
	if _, err := svc.Delete(context.Background(), user(1), deleted.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, _, err = svc.Create(context.Background(), user(1), CreateInput{Title: "X", ParentID: ptr(deleted.ID)})
	if status := apiError(t, err).Status; status != http.StatusBadRequest {
		t.Fatalf("родитель удалён: status = %d, want 400", status)
	}
}

// KB-1: заголовок 1..200, content_html до 100 000.
func TestService_CreateValidation(t *testing.T) {
	svc, _, _ := newTestService(t)

	if status := apiError(t, mustFail(t, svc, CreateInput{Title: "   "})).Status; status != http.StatusUnprocessableEntity {
		t.Fatalf("пустой заголовок: status = %d", status)
	}
	if status := apiError(t, mustFail(t, svc, CreateInput{Title: strings.Repeat("я", MaxTitleRunes+1)})).Status; status != http.StatusUnprocessableEntity {
		t.Fatalf("длинный заголовок: status = %d", status)
	}
	if status := apiError(t, mustFail(t, svc, CreateInput{
		Title:       "Большая статья",
		ContentHTML: strings.Repeat("x", MaxContentRunes+1),
	})).Status; status != http.StatusUnprocessableEntity {
		t.Fatalf("большой content_html: status = %d", status)
	}
}

// mustFail выполняет Create и возвращает ошибку.
func mustFail(t *testing.T, svc *Service, in CreateInput) error {
	t.Helper()
	_, _, err := svc.Create(context.Background(), user(1), in)
	if err == nil {
		t.Fatalf("Create(%q) не вернул ошибку", in.Title)
	}
	return err
}

// KB-5: ссылка на несуществующий файл или файл заявки → 400 INVALID_MEDIA_REF.
func TestService_CreateInvalidMedia(t *testing.T) {
	svc, _, media := newTestService(t)
	media.scopes[9] = files.ScopeTicket // файл заявки в статью вставлять нельзя

	for _, content := range []string{
		`<p><img src="/api/files/404" alt="x"></p>`,
		`<p><img src="/api/files/9" alt="x"></p>`,
		`<video controls><source src="/api/files/404" type="video/mp4"></video>`,
	} {
		_, _, err := svc.Create(context.Background(), user(1), CreateInput{Title: "Статья", ContentHTML: content})
		apiErr := apiError(t, err)
		if apiErr.Status != http.StatusBadRequest || apiErr.Code != apierr.CodeInvalidMediaRef {
			t.Fatalf("content %q: status=%d code=%s, want 400 INVALID_MEDIA_REF", content, apiErr.Status, apiErr.Code)
		}
		if _, ok := apiErr.Details["url"]; !ok {
			t.Fatalf("в details нет url: %v", apiErr.Details)
		}
	}
}

// KB-1/KB-2: вложенность не ограничена — цепочка из 10 узлов создаётся и
// читается с полными хлебными крошками.
func TestService_CreateDepth10(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	var parentID *int64
	var last *Node
	for level := 1; level <= 10; level++ {
		node, _, err := svc.Create(ctx, user(1), CreateInput{
			Title:    "Уровень " + intToString(level),
			ParentID: parentID,
		})
		if err != nil {
			t.Fatalf("уровень %d: %v", level, err)
		}
		parentID = ptr(node.ID)
		last = node
	}

	_, crumbs, err := svc.Node(ctx, last.ID, false)
	if err != nil {
		t.Fatalf("Node: %v", err)
	}
	if len(crumbs) != 10 {
		t.Fatalf("длина breadcrumbs = %d, want 10", len(crumbs))
	}
	if crumbs[0].Title != "Уровень 1" || crumbs[9].Title != "Уровень 10" {
		t.Fatalf("breadcrumbs = %v", crumbs)
	}
}

// KB-2: перемещение в собственное поддерево → 409 CYCLE_DETECTED.
func TestService_UpdateCycle(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	a := mustCreate(t, svc, CreateInput{Title: "A"})
	b := mustCreate(t, svc, CreateInput{Title: "B", ParentID: ptr(a.ID)})
	c := mustCreate(t, svc, CreateInput{Title: "C", ParentID: ptr(b.ID)})

	// A под C — цикл (C потомок A).
	_, _, err := svc.Update(ctx, user(1), a.ID, UpdateInput{ParentID: OptionalInt64{Set: true, Value: ptr(c.ID)}})
	apiErr := apiError(t, err)
	if apiErr.Status != http.StatusConflict || apiErr.Code != apierr.CodeCycleDetected {
		t.Fatalf("status=%d code=%s, want 409 CYCLE_DETECTED", apiErr.Status, apiErr.Code)
	}
	// A сам себе родитель — тоже цикл.
	_, _, err = svc.Update(ctx, user(1), a.ID, UpdateInput{ParentID: OptionalInt64{Set: true, Value: ptr(a.ID)}})
	if apiError(t, err).Status != http.StatusConflict {
		t.Fatalf("self-parent: %v", err)
	}
}

// KB-2/KB-3: смена родителя на удалённый → 400, на несуществующий → 404.
func TestService_UpdateParentErrors(t *testing.T) {
	svc, repo, _ := newTestService(t)
	ctx := context.Background()
	node := mustCreate(t, svc, CreateInput{Title: "Статья"})
	deleted := repo.seed(Node{Title: "Удалённый"})
	if _, err := svc.Delete(ctx, user(1), deleted.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, _, err := svc.Update(ctx, user(1), node.ID, UpdateInput{ParentID: OptionalInt64{Set: true, Value: ptr(deleted.ID)}})
	if status := apiError(t, err).Status; status != http.StatusBadRequest {
		t.Fatalf("удалённый родитель: status = %d, want 400", status)
	}
	_, _, err = svc.Update(ctx, user(1), node.ID, UpdateInput{ParentID: OptionalInt64{Set: true, Value: ptr(int64(404))}})
	if status := apiError(t, err).Status; status != http.StatusNotFound {
		t.Fatalf("нет родителя: status = %d, want 404", status)
	}
}

// PATCH: content_html санитизируется и проверяется; parent_id: null — в корень
// с постановкой в конец списка.
func TestService_UpdateContentAndRoot(t *testing.T) {
	svc, _, media := newTestService(t)
	ctx := context.Background()
	media.scopes[3] = files.ScopeKB

	root := mustCreate(t, svc, CreateInput{Title: "Корень"})
	child := mustCreate(t, svc, CreateInput{Title: "Ребёнок", ParentID: ptr(root.ID)})

	updated, _, err := svc.Update(ctx, user(2), child.ID, UpdateInput{
		ContentHTML: ptr(`<p>Новый <b>текст</b></p><script>x</script><img src="/api/files/3">`),
		ParentID:    OptionalInt64{Set: true, Value: nil},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if strings.Contains(updated.ContentHTML, "script") {
		t.Fatalf("content_html не санитизирован: %s", updated.ContentHTML)
	}
	if updated.ParentID != nil {
		t.Fatalf("parent_id = %v, want null", *updated.ParentID)
	}
	// В корне уже есть узел с sort_order=10 → новый корневой узел получает 20.
	if updated.SortOrder != 2*SortOrderStep {
		t.Fatalf("sort_order = %d, want %d", updated.SortOrder, 2*SortOrderStep)
	}
	if updated.UpdatedBy == nil || *updated.UpdatedBy != 2 {
		t.Fatalf("updated_by = %v", updated.UpdatedBy)
	}
	if len(media.linked) == 0 || media.linked[0] != 3 {
		t.Fatalf("MarkLinked не вызван: %v", media.linked)
	}
}

// KB-6: перемещение переиндексирует sort_order соседей шагом 10.
func TestService_MoveReindex(t *testing.T) {
	svc, repo, _ := newTestService(t)
	ctx := context.Background()
	root := mustCreate(t, svc, CreateInput{Title: "Корень"})
	a := mustCreate(t, svc, CreateInput{Title: "A", ParentID: ptr(root.ID)})
	b := mustCreate(t, svc, CreateInput{Title: "B", ParentID: ptr(root.ID)})
	c := mustCreate(t, svc, CreateInput{Title: "C", ParentID: ptr(root.ID)})
	if a.SortOrder != 10 || b.SortOrder != 20 || c.SortOrder != 30 {
		t.Fatalf("начальный порядок: %d,%d,%d", a.SortOrder, b.SortOrder, c.SortOrder)
	}

	// C перед A: C=10, A=20, B=30.
	moved, _, err := svc.Move(ctx, user(1), c.ID, MoveInput{
		ParentID:     OptionalInt64{Set: true, Value: ptr(root.ID)},
		BeforeNodeID: ptr(a.ID),
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.SortOrder != 10 {
		t.Fatalf("sort_order перемещённого = %d, want 10", moved.SortOrder)
	}
	for id, want := range map[int64]int{a.ID: 20, b.ID: 30, c.ID: 10} {
		stored, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("repo.Get(%d): %v", id, err)
		}
		if stored.SortOrder != want {
			t.Fatalf("узел %d: sort_order = %d, want %d", id, stored.SortOrder, want)
		}
	}

	// Перемещение в другой родитель (в конец) — parent_id и порядок меняются.
	other := mustCreate(t, svc, CreateInput{Title: "Другой корень"})
	moved, _, err = svc.Move(ctx, user(1), c.ID, MoveInput{ParentID: OptionalInt64{Set: true, Value: ptr(other.ID)}})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.ParentID == nil || *moved.ParentID != other.ID {
		t.Fatalf("parent_id = %v, want %d", moved.ParentID, other.ID)
	}
	if moved.SortOrder != SortOrderStep {
		t.Fatalf("sort_order = %d, want %d", moved.SortOrder, SortOrderStep)
	}
	// Прежний родитель не переиндексируется: KB-6 требует шаг 10 у списка
	// нового родителя, а старый порядок A=20, B=30 остаётся корректным.
	for id, want := range map[int64]int{a.ID: 20, b.ID: 30} {
		stored, _ := repo.Get(ctx, id)
		if stored.SortOrder != want {
			t.Fatalf("узел %d: sort_order = %d, want %d", id, stored.SortOrder, want)
		}
	}
}

// KB-6/KB-2: ошибки перемещения — 422 перед чужим узлом, 409 цикл.
func TestService_MoveErrors(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	root := mustCreate(t, svc, CreateInput{Title: "Корень"})
	a := mustCreate(t, svc, CreateInput{Title: "A", ParentID: ptr(root.ID)})
	other := mustCreate(t, svc, CreateInput{Title: "Другой"})
	_ = mustCreate(t, svc, CreateInput{Title: "Чужой ребёнок", ParentID: ptr(other.ID)})

	// before_node_id не сосед нового родителя → 422 (a — ребёнок root).
	_, _, err := svc.Move(ctx, user(1), root.ID, MoveInput{
		ParentID:     OptionalInt64{Set: true, Value: ptr(other.ID)},
		BeforeNodeID: ptr(a.ID),
	})
	if status := apiError(t, err).Status; status != http.StatusUnprocessableEntity {
		t.Fatalf("чужой before: status = %d, want 422", status)
	}
	// parent_id обязателен (отсутствует в теле) → 422.
	if _, _, err := svc.Move(ctx, user(1), root.ID, MoveInput{}); apiError(t, err).Status != http.StatusUnprocessableEntity {
		t.Fatalf("без parent_id: %v", err)
	}
	// Перемещение корня под собственного ребёнка → 409.
	_, _, err = svc.Move(ctx, user(1), root.ID, MoveInput{ParentID: OptionalInt64{Set: true, Value: ptr(a.ID)}})
	if apiError(t, err).Code != apierr.CodeCycleDetected {
		t.Fatalf("цикл: %v", err)
	}
}

// KB-3: удаление в корзину каскадно с поддеревом и числом потомков.
func TestService_Delete(t *testing.T) {
	svc, repo, _ := newTestService(t)
	ctx := context.Background()
	root := mustCreate(t, svc, CreateInput{Title: "Корень"})
	child := mustCreate(t, svc, CreateInput{Title: "Ребёнок", ParentID: ptr(root.ID)})
	grand := mustCreate(t, svc, CreateInput{Title: "Внук", ParentID: ptr(child.ID)})
	_ = grand

	descendants, err := svc.Delete(ctx, user(1), root.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if descendants != 2 {
		t.Fatalf("descendants = %d, want 2", descendants)
	}
	for _, id := range []int64{root.ID, child.ID, grand.ID} {
		if _, err := repo.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("узел %d должен быть в корзине, err = %v", id, err)
		}
	}
	// Повторное удаление — 404 (узел уже в корзине).
	if _, err := svc.Delete(ctx, user(1), root.ID); apiError(t, err).Status != http.StatusNotFound {
		t.Fatalf("повторное удаление: %v", err)
	}
}

// KB-7: видимость меняется только у корневых разделов, ADMIN-права проверяет
// маршрут.
func TestService_SetVisibility(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	root := mustCreate(t, svc, CreateInput{Title: "Корень"})
	child := mustCreate(t, svc, CreateInput{Title: "Ребёнок", ParentID: ptr(root.ID)})

	updated, _, err := svc.SetVisibility(ctx, user(1), root.ID, true)
	if err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}
	if !updated.IsHiddenForAnon {
		t.Fatal("видимость не изменилась")
	}
	_, _, err = svc.SetVisibility(ctx, user(1), child.ID, true)
	if status := apiError(t, err).Status; status != http.StatusUnprocessableEntity {
		t.Fatalf("потомок: status = %d, want 422", status)
	}
	if _, _, err := svc.SetVisibility(ctx, user(1), 404, true); apiError(t, err).Status != http.StatusNotFound {
		t.Fatalf("несуществующий: %v", err)
	}
}

// KB-7/KB-8: публичное дерево скрывает hidden-корни, авторизованное — нет;
// скрытый узел и потомок скрытого корня для анонима — 404.
func TestService_TreeAndNodeVisibility(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	hiddenRoot := mustCreate(t, svc, CreateInput{Title: "Скрытый", IsHiddenForAnon: true})
	hiddenChild := mustCreate(t, svc, CreateInput{Title: "Скрытый потомок", ParentID: ptr(hiddenRoot.ID)})
	publicRoot := mustCreate(t, svc, CreateInput{Title: "Публичный"})
	publicChild := mustCreate(t, svc, CreateInput{Title: "Публичный потомок", ParentID: ptr(publicRoot.ID)})

	publicTree, err := svc.Tree(ctx, true)
	if err != nil {
		t.Fatalf("Tree(public): %v", err)
	}
	if len(publicTree) != 1 || publicTree[0].ID != publicRoot.ID {
		t.Fatalf("публичные корни = %v, want [%d]", ids(publicTree), publicRoot.ID)
	}
	if len(publicTree[0].Children) != 1 || publicTree[0].Children[0].ID != publicChild.ID {
		t.Fatalf("дети публичного = %v", ids(publicTree[0].Children))
	}

	authorizedTree, err := svc.Tree(ctx, false)
	if err != nil {
		t.Fatalf("Tree(authorized): %v", err)
	}
	if len(authorizedTree) != 2 {
		t.Fatalf("корней для авторизованного = %d, want 2", len(authorizedTree))
	}

	// Публичный узел доступен, скрытый и его потомок — 404.
	if _, _, err := svc.Node(ctx, publicChild.ID, true); err != nil {
		t.Fatalf("публичный узел недоступен: %v", err)
	}
	for _, id := range []int64{hiddenRoot.ID, hiddenChild.ID} {
		if _, _, err := svc.Node(ctx, id, true); apiError(t, err).Status != http.StatusNotFound {
			t.Fatalf("узел %d должен быть 404 для анонима: %v", id, err)
		}
	}
	// Авторизованный видит скрытый узел.
	if _, _, err := svc.Node(ctx, hiddenChild.ID, false); err != nil {
		t.Fatalf("скрытый узел для авторизованного: %v", err)
	}

	// Удалённый узел не виден никому.
	if _, err := svc.Delete(ctx, user(1), publicChild.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, _, err := svc.Node(ctx, publicChild.ID, false); apiError(t, err).Status != http.StatusNotFound {
		t.Fatalf("удалённый узел: %v", err)
	}
}

// KB-5/KB-7: публичная доступность медиа — файл в публичном узле да, только в
// скрытом/удалённом — нет; точное совпадение id.
func TestService_PublicKBFile(t *testing.T) {
	svc, _, media := newTestService(t)
	ctx := context.Background()
	media.scopes[11] = files.ScopeKB
	media.scopes[12] = files.ScopeKB

	public := mustCreate(t, svc, CreateInput{
		Title:       "Публичная",
		ContentHTML: `<p><img src="/api/files/11" alt="x"></p>`,
	})
	hidden := mustCreate(t, svc, CreateInput{
		Title:           "Скрытая",
		ContentHTML:     `<p><img src="/api/files/12" alt="x"></p>`,
		IsHiddenForAnon: true,
	})

	if ok, err := svc.PublicKBFile(ctx, 11); err != nil || !ok {
		t.Fatalf("файл публичной статьи: ok=%v err=%v", ok, err)
	}
	if ok, err := svc.PublicKBFile(ctx, 12); err != nil || ok {
		t.Fatalf("файл скрытой статьи: ok=%v err=%v", ok, err)
	}
	if ok, _ := svc.PublicKBFile(ctx, 1); ok {
		t.Fatal("файл без ссылки должен быть недоступен")
	}
	// Точное совпадение: id=1 не должен находиться по ссылке /api/files/11.
	if ok, _ := svc.PublicKBFile(ctx, 1); ok {
		t.Fatal("id=1 найден по подстроке")
	}
	// После скрытия публичного узла медиа перестаёт быть доступным.
	if _, _, err := svc.SetVisibility(ctx, user(1), public.ID, true); err != nil {
		t.Fatalf("SetVisibility: %v", err)
	}
	if ok, _ := svc.PublicKBFile(ctx, 11); ok {
		t.Fatal("медиа скрытого раздела не должно быть доступно")
	}
	// После удаления узла — тоже.
	if _, err := svc.Delete(ctx, user(1), hidden.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := svc.PublicKBFile(ctx, 12); ok {
		t.Fatal("медиа удалённого узла не должно быть доступно")
	}
}
