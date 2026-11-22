package kb

import "sort"

// sibling — узел-сосед при перемещении (KB-6). Сортировка выполняется в Go, а
// не в SQL, чтобы порядок блокировок строк был по id (защита от deadlock при
// параллельных перемещениях).
type sibling struct {
	ID        int64
	SortOrder int
	Title     string
}

// orderWithMove возвращает новый порядок детей целевого родителя после
// перемещения movingID перед beforeNodeID (nil — в конец).
//
// siblings — текущие живые дети родителя; movingID может как присутствовать в
// списке (перемещение внутри того же родителя), так и отсутствовать.
//
// Ошибка ErrBeforeNotFound: beforeNodeID не является ребёнком целевого родителя.
func orderWithMove(siblings []sibling, movingID int64, beforeNodeID *int64) ([]int64, error) {
	orderedSiblings := make([]sibling, 0, len(siblings))
	for _, item := range siblings {
		if item.ID == movingID {
			continue
		}
		orderedSiblings = append(orderedSiblings, item)
	}
	sort.SliceStable(orderedSiblings, func(i, j int) bool {
		a, b := orderedSiblings[i], orderedSiblings[j]
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})

	ordered := make([]int64, 0, len(orderedSiblings)+1)
	if beforeNodeID == nil {
		for _, item := range orderedSiblings {
			ordered = append(ordered, item.ID)
		}
		return append(ordered, movingID), nil
	}

	inserted := false
	for _, item := range orderedSiblings {
		if item.ID == *beforeNodeID && !inserted {
			ordered = append(ordered, movingID)
			inserted = true
		}
		ordered = append(ordered, item.ID)
	}
	if !inserted {
		// before_node_id не найден среди соседей нового родителя.
		return nil, ErrBeforeNotFound
	}
	return ordered, nil
}

// sortOrderAt возвращает sort_order для позиции index (с нуля) в списке
// соседей: 10, 20, 30, … (KB-6: переиндексация с шагом 10).
func sortOrderAt(index int) int {
	return (index + 1) * SortOrderStep
}
