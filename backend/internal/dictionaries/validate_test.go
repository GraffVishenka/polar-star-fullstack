package dictionaries

import (
	"strings"
	"testing"
)

// Валидация справочника: имя 1..120, дефолты is_active/sort_order.
func TestValidateCreate(t *testing.T) {
	in, details := ValidateCreate(CreateRequest{Name: "  Не работает оборудование  "})
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	if in.Name != "Не работает оборудование" {
		t.Fatalf("name = %q", in.Name)
	}
	if !in.IsActive {
		t.Fatal("is_active по умолчанию должен быть true")
	}
	if in.SortOrder != 0 {
		t.Fatalf("sort_order = %d", in.SortOrder)
	}

	inactive := false
	order := 7
	in, details = ValidateCreate(CreateRequest{Name: "Прочее", IsActive: &inactive, SortOrder: &order})
	if len(details) != 0 || in.IsActive || in.SortOrder != 7 {
		t.Fatalf("in = %+v details = %v", in, details)
	}

	_, details = ValidateCreate(CreateRequest{Name: "   "})
	if _, ok := details["name"]; !ok {
		t.Fatalf("пустое имя должно отклоняться: %v", details)
	}
	_, details = ValidateCreate(CreateRequest{Name: strings.Repeat("я", 121)})
	if _, ok := details["name"]; !ok {
		t.Fatalf("имя 121 символ должно отклоняться: %v", details)
	}
	if _, details := ValidateCreate(CreateRequest{Name: strings.Repeat("я", 120)}); len(details) != 0 {
		t.Fatalf("имя 120 символов должно проходить: %v", details)
	}
}

// PATCH: пустой патч допустим, поля нормализуются.
func TestValidateUpdate(t *testing.T) {
	in, details := ValidateUpdate(UpdateRequest{})
	if len(details) != 0 || !in.Empty() {
		t.Fatalf("in = %+v details = %v", in, details)
	}

	name := "  Новое имя "
	inactive := false
	order := 3
	in, details = ValidateUpdate(UpdateRequest{Name: &name, IsActive: &inactive, SortOrder: &order})
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	if in.Name == nil || *in.Name != "Новое имя" || in.IsActive == nil || *in.IsActive || in.SortOrder == nil || *in.SortOrder != 3 {
		t.Fatalf("in = %+v", in)
	}

	empty := "   "
	_, details = ValidateUpdate(UpdateRequest{Name: &empty})
	if _, ok := details["name"]; !ok {
		t.Fatalf("пустое имя должно отклоняться: %v", details)
	}
}

// Вид справочника ограничен whitelist имён таблиц.
func TestKindTable(t *testing.T) {
	if table, ok := KindProblemTypes.table(); !ok || table != "problem_types" {
		t.Fatalf("problem_types: %q %v", table, ok)
	}
	if table, ok := KindEquipmentTypes.table(); !ok || table != "equipment_types" {
		t.Fatalf("equipment_types: %q %v", table, ok)
	}
	if table, ok := Kind("users").table(); ok || table != "" {
		t.Fatalf("неизвестный вид не должен давать таблицу: %q %v", table, ok)
	}
}
