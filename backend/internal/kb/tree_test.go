package kb

import "testing"

// ptr возвращает указатель на значение (nullable parent_id).
func ptr[T any](value T) *T { return &value }

// buildChain создаёт цепочку узлов заданной глубины: узел 1 — корень.
func buildChain(depth int, hiddenRoot bool) []Node {
	nodes := make([]Node, 0, depth)
	for level := 1; level <= depth; level++ {
		var parentID *int64
		if level > 1 {
			parentID = ptr(int64(level - 1))
		}
		nodes = append(nodes, Node{
			ID:              int64(level),
			Title:           string(rune('A'+level-1)) + "-узел",
			ParentID:        parentID,
			SortOrder:       level * 10,
			IsHiddenForAnon: level == 1 && hiddenRoot,
		})
	}
	return nodes
}

// KB-8: дерево строится с вложенностью любой глубины; глубина 10 проверяется
// явно (KB-2: UI отображает глубину ≥ 5).
func TestBuildTree_Depth(t *testing.T) {
	nodes := buildChain(10, false)
	tree := buildTree(nodes, false)
	if len(tree) != 1 {
		t.Fatalf("корней = %d, want 1", len(tree))
	}
	depth := 1
	for current := tree[0]; len(current.Children) > 0; current = current.Children[0] {
		depth++
	}
	if depth != 10 {
		t.Fatalf("глубина дерева = %d, want 10", depth)
	}
	// Каждый ребёнок имеет правильного родителя.
	for current := tree[0]; len(current.Children) > 0; current = current.Children[0] {
		child := current.Children[0]
		if child.ParentID == nil || *child.ParentID != current.ID {
			t.Fatalf("parent_id ребёнка %d = %v, want %d", child.ID, child.ParentID, current.ID)
		}
	}
}

// KB-8: сортировка по sort_order, затем title на каждом уровне.
func TestBuildTree_SortsByOrderThenTitle(t *testing.T) {
	nodes := []Node{
		{ID: 2, Title: "Яблоко", SortOrder: 20},
		{ID: 1, Title: "Банан", SortOrder: 10},
		{ID: 3, Title: "Абрикос", SortOrder: 20},
		{ID: 4, Title: "Ребёнок Б", SortOrder: 20, ParentID: ptr(int64(3))},
		{ID: 5, Title: "Ребёнок А", SortOrder: 10, ParentID: ptr(int64(3))},
	}
	tree := buildTree(nodes, false)
	if len(tree) != 3 {
		t.Fatalf("корней = %d, want 3", len(tree))
	}
	wantOrder := []int64{1, 3, 2}
	for i, want := range wantOrder {
		if tree[i].ID != want {
			t.Fatalf("порядок корней = %v, want %v", ids(tree), wantOrder)
		}
	}
	// Дети узла 3: сначала sort_order=10, затем 20.
	children := tree[1].Children
	if len(children) != 2 || children[0].ID != 5 || children[1].ID != 4 {
		t.Fatalf("порядок детей = %v", ids(children))
	}
}

// KB-7: анониму скрытые корневые разделы и всё их поддерево не отдаются.
func TestBuildTree_PublicExcludesHiddenSubtree(t *testing.T) {
	nodes := []Node{
		{ID: 1, Title: "Публичный", SortOrder: 10},
		{ID: 2, Title: "Скрытый", SortOrder: 20, IsHiddenForAnon: true},
		{ID: 3, Title: "Ребёнок публичного", SortOrder: 10, ParentID: ptr(int64(1))},
		{ID: 4, Title: "Ребёнок скрытого", SortOrder: 10, ParentID: ptr(int64(2))},
		{ID: 5, Title: "Внук скрытого", SortOrder: 10, ParentID: ptr(int64(4))},
	}
	public := buildTree(nodes, true)
	if len(public) != 1 || public[0].ID != 1 {
		t.Fatalf("публичные корни = %v, want [1]", ids(public))
	}
	if len(public[0].Children) != 1 || public[0].Children[0].ID != 3 {
		t.Fatalf("дети публичного = %v, want [3]", ids(public[0].Children))
	}

	// Авторизованный видит оба корня и всё поддерево скрытого.
	authorized := buildTree(nodes, false)
	if len(authorized) != 2 {
		t.Fatalf("корней для авторизованного = %d, want 2", len(authorized))
	}
	if len(authorized[1].Children) != 1 || authorized[1].Children[0].ID != 4 {
		t.Fatalf("поддерево скрытого не построено: %v", ids(authorized[1].Children))
	}
}

// Узел, чей родитель отсутствует (в корзине), у авторизованного поднимается в
// корень, а анониму не показывается вовсе (иначе скрытая ветка «всплыла» бы).
func TestBuildTree_Orphans(t *testing.T) {
	nodes := []Node{
		{ID: 1, Title: "Живой корень", SortOrder: 10},
		{ID: 2, Title: "Сирота", SortOrder: 20, ParentID: ptr(int64(99))},
	}
	if got := buildTree(nodes, false); len(got) != 2 {
		t.Fatalf("авторизованный: корней = %d, want 2", len(got))
	}
	if got := buildTree(nodes, true); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("аноним: корни = %v, want [1]", ids(got))
	}
}

func TestBuildTree_Empty(t *testing.T) {
	if got := buildTree(nil, false); len(got) != 0 {
		t.Fatalf("got = %d узлов", len(got))
	}
	if got := buildTree(nil, true); len(got) != 0 {
		t.Fatalf("got = %d узлов", len(got))
	}
}

// ids — идентификаторы узлов для сообщений об ошибках.
func ids(items []*TreeNode) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}
