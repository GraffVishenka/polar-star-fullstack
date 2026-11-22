package equipment

import (
	"strings"
	"testing"
)

// EQ-2: обязательные поля, длины и положительные id.
func TestValidateCreate(t *testing.T) {
	valid := CreateRequest{
		Name:               "  Ноутбук  ",
		EquipmentTypeID:    2,
		SerialNumber:       " SN-001 ",
		RegistrationNumber: " REG-001 ",
		WarehouseID:        4,
	}
	in, details := ValidateCreate(valid)
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	if in.Name != "Ноутбук" || in.SerialNumber != "SN-001" || in.RegistrationNumber != "REG-001" {
		t.Fatalf("поля не нормализованы: %+v", in)
	}
	if in.EquipmentTypeID != 2 || in.WarehouseID != 4 {
		t.Fatalf("id не проброшены: %+v", in)
	}

	invalid := CreateRequest{
		Name:               strings.Repeat("я", 201),
		SerialNumber:       strings.Repeat("s", 101),
		RegistrationNumber: strings.Repeat("r", 101),
		EquipmentTypeID:    0,
		WarehouseID:        -1,
	}
	_, details = ValidateCreate(invalid)
	for _, field := range []string{"name", "serial_number", "registration_number", "equipment_type_id", "warehouse_id"} {
		if _, ok := details[field]; !ok {
			t.Fatalf("нет ошибки по полю %q: %v", field, details)
		}
	}

	// Ровно 200/100/100 символов проходят (кириллица считается в символах).
	_, details = ValidateCreate(CreateRequest{
		Name:               strings.Repeat("я", 200),
		SerialNumber:       strings.Repeat("s", 100),
		RegistrationNumber: strings.Repeat("r", 100),
		EquipmentTypeID:    1,
		WarehouseID:        1,
	})
	if len(details) != 0 {
		t.Fatalf("граничные длины должны проходить: %v", details)
	}
}

// PATCH: nil-поле не меняется, пустой патч допустим.
func TestValidateUpdate(t *testing.T) {
	in, details := ValidateUpdate(UpdateRequest{})
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	if !in.Empty() {
		t.Fatal("пустой патч должен считаться пустым")
	}

	name := " Новое имя "
	in, details = ValidateUpdate(UpdateRequest{Name: &name})
	if len(details) != 0 || in.Name == nil || *in.Name != "Новое имя" {
		t.Fatalf("name: in=%+v details=%v", in, details)
	}

	zero := int64(0)
	_, details = ValidateUpdate(UpdateRequest{EquipmentTypeID: &zero, WarehouseID: &zero})
	if _, ok := details["equipment_type_id"]; !ok {
		t.Fatalf("нет ошибки equipment_type_id: %v", details)
	}
	if _, ok := details["warehouse_id"]; !ok {
		t.Fatalf("нет ошибки warehouse_id: %v", details)
	}

	empty := "   "
	_, details = ValidateUpdate(UpdateRequest{SerialNumber: &empty})
	if _, ok := details["serial_number"]; !ok {
		t.Fatalf("пустой serial_number должен отклоняться: %v", details)
	}
}

// EQ-3: to_warehouse_id обязателен, comment ≤ 1000 и нормализуется.
func TestValidateMove(t *testing.T) {
	in, details := ValidateMove(MoveRequest{ToWarehouseID: 5})
	if len(details) != 0 || in.ToWarehouseID != 5 || in.Comment != nil {
		t.Fatalf("in=%+v details=%v", in, details)
	}

	_, details = ValidateMove(MoveRequest{ToWarehouseID: 0})
	if _, ok := details["to_warehouse_id"]; !ok {
		t.Fatalf("нет ошибки to_warehouse_id: %v", details)
	}

	blank := "   "
	in, details = ValidateMove(MoveRequest{ToWarehouseID: 5, Comment: &blank})
	if len(details) != 0 || in.Comment != nil {
		t.Fatalf("пустой комментарий должен стать nil: in=%+v details=%v", in, details)
	}

	comment := "  перенос в другой корпус  "
	in, details = ValidateMove(MoveRequest{ToWarehouseID: 5, Comment: &comment})
	if len(details) != 0 || in.Comment == nil || *in.Comment != "перенос в другой корпус" {
		t.Fatalf("comment: in=%+v details=%v", in, details)
	}

	tooLong := strings.Repeat("к", 1001)
	_, details = ValidateMove(MoveRequest{ToWarehouseID: 5, Comment: &tooLong})
	if _, ok := details["comment"]; !ok {
		t.Fatalf("комментарий 1001 символ должен отклоняться: %v", details)
	}
}

// EQ-4: 1..200 уникальных положительных id, целевой склад и комментарий.
func TestValidateBulkMove(t *testing.T) {
	in, details := ValidateBulkMove(BulkMoveRequest{
		EquipmentIDs:  []int64{5, 3, 5, 1},
		ToWarehouseID: 7,
	})
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	if len(in.EquipmentIDs) != 3 || in.ToWarehouseID != 7 {
		t.Fatalf("дубликаты должны удаляться: in=%+v", in)
	}
	// Порядок не нормализуется в валидаторе: сортировку делает репозиторий для
	// порядка блокировок.
	if in.EquipmentIDs[0] != 5 || in.EquipmentIDs[2] != 1 {
		t.Fatalf("ids = %v", in.EquipmentIDs)
	}

	// Пустой список → 422.
	_, details = ValidateBulkMove(BulkMoveRequest{ToWarehouseID: 7})
	if _, ok := details["equipment_ids"]; !ok {
		t.Fatalf("пустой список должен отклоняться: %v", details)
	}
	// Только нулевые/отрицательные id → 422.
	_, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: []int64{0, -1}, ToWarehouseID: 7})
	if _, ok := details["equipment_ids"]; !ok {
		t.Fatalf("неположительные id должны отклоняться: %v", details)
	}
	// Один битый id в наборе → 422 (операция не выполняется).
	_, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: []int64{1, 0, 2}, ToWarehouseID: 7})
	if _, ok := details["equipment_ids"]; !ok {
		t.Fatalf("битый id в наборе должен отклоняться: %v", details)
	}
	// Целевой склад обязателен.
	_, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: []int64{1}, ToWarehouseID: 0})
	if _, ok := details["to_warehouse_id"]; !ok {
		t.Fatalf("to_warehouse_id обязателен: %v", details)
	}

	// Ровно 200 единиц — можно, 201 — нельзя.
	maxIDs := make([]int64, MaxBulkMoveItems)
	for i := range maxIDs {
		maxIDs[i] = int64(i + 1)
	}
	_, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: maxIDs, ToWarehouseID: 7})
	if len(details) != 0 {
		t.Fatalf("200 единиц должны проходить: %v", details)
	}
	tooMany := append(append([]int64{}, maxIDs...), 201)
	_, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: tooMany, ToWarehouseID: 7})
	if _, ok := details["equipment_ids"]; !ok {
		t.Fatalf("201 единица должна отклоняться: %v", details)
	}

	// Комментарий: пустой → nil, длинный → ошибка, обычный нормализуется.
	blank := "   "
	in, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: []int64{1}, ToWarehouseID: 7, Comment: &blank})
	if len(details) != 0 || in.Comment != nil {
		t.Fatalf("пустой комментарий: in=%+v details=%v", in, details)
	}
	comment := "  массовый перенос  "
	in, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: []int64{1}, ToWarehouseID: 7, Comment: &comment})
	if len(details) != 0 || in.Comment == nil || *in.Comment != "массовый перенос" {
		t.Fatalf("comment: in=%+v details=%v", in, details)
	}
	tooLong := strings.Repeat("к", 1001)
	_, details = ValidateBulkMove(BulkMoveRequest{EquipmentIDs: []int64{1}, ToWarehouseID: 7, Comment: &tooLong})
	if _, ok := details["comment"]; !ok {
		t.Fatalf("длинный комментарий должен отклоняться: %v", details)
	}
}

// PageQuery/SizeQuery: page ≥ 1, size 1..100, значения по умолчанию 1/20.
func TestParsePagination(t *testing.T) {
	page, size, details := ParsePagination("", "")
	if len(details) != 0 || page != 1 || size != DefaultPageSize {
		t.Fatalf("defaults: page=%d size=%d details=%v", page, size, details)
	}

	page, size, details = ParsePagination("3", "100")
	if len(details) != 0 || page != 3 || size != 100 {
		t.Fatalf("valid: page=%d size=%d details=%v", page, size, details)
	}

	for _, tc := range []struct{ page, size, field string }{
		{"0", "", "page"},
		{"-1", "", "page"},
		{"abc", "", "page"},
		{"", "0", "size"},
		{"", "101", "size"},
		{"", "abc", "size"},
	} {
		_, _, details = ParsePagination(tc.page, tc.size)
		if _, ok := details[tc.field]; !ok {
			t.Fatalf("page=%q size=%q: нет ошибки %q в %v", tc.page, tc.size, tc.field, details)
		}
	}
}

// EQ-7: фильтры — необязательные положительные id.
func TestParseOptionalID(t *testing.T) {
	value, message := ParseOptionalID("")
	if value != nil || message != "" {
		t.Fatalf("пустая строка: %v %q", value, message)
	}
	value, message = ParseOptionalID(" 42 ")
	if message != "" || value == nil || *value != 42 {
		t.Fatalf("42: %v %q", value, message)
	}
	for _, raw := range []string{"0", "-3", "abc", "1.5"} {
		if _, message := ParseOptionalID(raw); message == "" {
			t.Fatalf("значение %q должно отклоняться", raw)
		}
	}
}

// EQ-7: поиск ограничен 200 символами.
func TestNormalizeSearch(t *testing.T) {
	search, message := NormalizeSearch("  ноутбук  ")
	if message != "" || search != "ноутбук" {
		t.Fatalf("search=%q message=%q", search, message)
	}
	if _, message := NormalizeSearch(strings.Repeat("я", 201)); message == "" {
		t.Fatal("слишком длинный поиск должен отклоняться")
	}
}

// Спецсимволы LIKE не должны превращаться в wildcard (поиск по вводу).
func TestLikePattern(t *testing.T) {
	tests := map[string]string{
		"ноутбук": `%ноутбук%`,
		"50%":     `%50\%%`,
		"a_b":     `%a\_b%`,
		`c:\dir`:  `%c:\\dir%`,
	}
	for input, want := range tests {
		if got := LikePattern(input); got != want {
			t.Fatalf("LikePattern(%q) = %q, want %q", input, got, want)
		}
	}
}
