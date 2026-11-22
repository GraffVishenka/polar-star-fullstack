package kb

import (
	"errors"
	"testing"
)

// KB-6: вставка перед указанным узлом.
func TestOrderWithMove_Before(t *testing.T) {
	siblings := []sibling{
		{ID: 1, SortOrder: 10, Title: "A"},
		{ID: 2, SortOrder: 20, Title: "B"},
		{ID: 3, SortOrder: 30, Title: "C"},
	}
	before := int64(2)
	got, err := orderWithMove(siblings, 9, &before)
	if err != nil {
		t.Fatalf("orderWithMove: %v", err)
	}
	want := []int64{1, 9, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("got = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got = %v, want %v", got, want)
		}
	}
}

// KB-6: before_node_id не передан — узел встаёт в конец.
func TestOrderWithMove_AppendToEnd(t *testing.T) {
	siblings := []sibling{
		{ID: 2, SortOrder: 20, Title: "B"},
		{ID: 1, SortOrder: 10, Title: "A"},
	}
	got, err := orderWithMove(siblings, 7, nil)
	if err != nil {
		t.Fatalf("orderWithMove: %v", err)
	}
	want := []int64{1, 2, 7}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got = %v, want %v", got, want)
		}
	}
}

// Перемещение внутри того же родителя: перемещаемый узел не дублируется.
func TestOrderWithMove_SameParent(t *testing.T) {
	siblings := []sibling{
		{ID: 1, SortOrder: 10, Title: "A"},
		{ID: 2, SortOrder: 20, Title: "B"},
		{ID: 3, SortOrder: 30, Title: "C"},
	}
	before := int64(1)
	got, err := orderWithMove(siblings, 3, &before)
	if err != nil {
		t.Fatalf("orderWithMove: %v", err)
	}
	want := []int64{3, 1, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got = %v, want %v", got, want)
		}
	}
}

// before_node_id не является соседом нового родителя → ошибка валидации.
func TestOrderWithMove_MissingBefore(t *testing.T) {
	siblings := []sibling{{ID: 1, SortOrder: 10, Title: "A"}}
	before := int64(999)
	if _, err := orderWithMove(siblings, 5, &before); !errors.Is(err, ErrBeforeNotFound) {
		t.Fatalf("ожидалась ErrBeforeNotFound, получено %v", err)
	}
}

// Сортировка соседей: sort_order, затем title, затем id.
func TestOrderWithMove_SortsSiblings(t *testing.T) {
	siblings := []sibling{
		{ID: 5, SortOrder: 20, Title: "Б"},
		{ID: 4, SortOrder: 20, Title: "А"},
		{ID: 6, SortOrder: 10, Title: "В"},
	}
	got, err := orderWithMove(siblings, 9, nil)
	if err != nil {
		t.Fatalf("orderWithMove: %v", err)
	}
	want := []int64{6, 4, 5, 9}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got = %v, want %v", got, want)
		}
	}
}

// KB-6: шаг переиндексации — 10.
func TestSortOrderAt(t *testing.T) {
	cases := map[int]int{0: 10, 1: 20, 2: 30, 9: 100}
	for index, want := range cases {
		if got := sortOrderAt(index); got != want {
			t.Fatalf("sortOrderAt(%d) = %d, want %d", index, got, want)
		}
	}
	if SortOrderStep != 10 {
		t.Fatalf("SortOrderStep = %d, want 10", SortOrderStep)
	}
}
