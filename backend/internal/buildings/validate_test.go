package buildings

import (
	"strings"
	"testing"
)

// EQ-1: название здания обязательно и ограничено 200 символами.
func TestValidateCreateBuilding(t *testing.T) {
	tests := []struct {
		name        string
		req         CreateBuildingRequest
		wantName    string
		wantAddress *string
		wantFields  []string
	}{
		{
			name:     "корректное здание",
			req:      CreateBuildingRequest{Name: "  Главный корпус ", Address: strPtr(" ул. Ленина, 1 ")},
			wantName: "Главный корпус", wantAddress: strPtr("ул. Ленина, 1"),
		},
		{
			name:       "пустое название",
			req:        CreateBuildingRequest{Name: "   "},
			wantFields: []string{"name"},
		},
		{
			name:       "название из 201 символа",
			req:        CreateBuildingRequest{Name: strings.Repeat("я", 201)},
			wantFields: []string{"name"},
		},
		{
			name:     "название ровно 200 символов принимается",
			req:      CreateBuildingRequest{Name: strings.Repeat("я", 200)},
			wantName: strings.Repeat("я", 200),
		},
		{
			name:       "адрес из 501 символа",
			req:        CreateBuildingRequest{Name: "Дом", Address: strPtr(strings.Repeat("а", 501))},
			wantName:   "Дом",
			wantFields: []string{"address"},
		},
		{
			name:     "пустой адрес нормализуется в null",
			req:      CreateBuildingRequest{Name: "Дом", Address: strPtr("  ")},
			wantName: "Дом",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in, details := ValidateCreateBuilding(tc.req)
			if len(details) != len(tc.wantFields) {
				t.Fatalf("details = %v, want поля %v", details, tc.wantFields)
			}
			for _, field := range tc.wantFields {
				if _, ok := details[field]; !ok {
					t.Fatalf("нет ошибки по полю %q: %v", field, details)
				}
			}
			if details == nil {
				t.Fatal("details не должна быть nil")
			}
			if len(tc.wantFields) == 0 {
				if in.Name != tc.wantName {
					t.Fatalf("name = %q, want %q", in.Name, tc.wantName)
				}
				if !equalPtr(in.Address, tc.wantAddress) {
					t.Fatalf("address = %v, want %v", deref(in.Address), deref(tc.wantAddress))
				}
			}
		})
	}
}

// PATCH здания: отсутствующий address не меняется, null и "" очищают.
func TestValidateUpdateBuilding(t *testing.T) {
	empty, details := ValidateUpdateBuilding(UpdateBuildingRequest{})
	if len(details) != 0 {
		t.Fatalf("пустой патч не должен ломать валидацию: %v", details)
	}
	if !empty.Empty() {
		t.Fatal("пустой патч должен считаться пустым")
	}

	patch, details := ValidateUpdateBuilding(UpdateBuildingRequest{Name: strPtr(" Новое имя ")})
	if len(details) != 0 || patch.Name == nil || *patch.Name != "Новое имя" {
		t.Fatalf("name: patch=%+v details=%v", patch, details)
	}
	if patch.AddressSet {
		t.Fatal("отсутствующий address не должен помечаться как переданный")
	}

	var null OptionalString
	if err := null.UnmarshalJSON([]byte("null")); err != nil {
		t.Fatalf("UnmarshalJSON(null): %v", err)
	}
	patch, details = ValidateUpdateBuilding(UpdateBuildingRequest{Address: null})
	if len(details) != 0 || !patch.AddressSet || patch.Address != nil {
		t.Fatalf("address=null должен очищать адрес: patch=%+v details=%v", patch, details)
	}

	var blank OptionalString
	if err := blank.UnmarshalJSON([]byte(`"   "`)); err != nil {
		t.Fatalf("UnmarshalJSON(blank): %v", err)
	}
	patch, details = ValidateUpdateBuilding(UpdateBuildingRequest{Address: blank})
	if len(details) != 0 || !patch.AddressSet || patch.Address != nil {
		t.Fatalf(`address="   " должен нормализоваться в null: patch=%+v details=%v`, patch, details)
	}

	tooLong := OptionalString{Set: true, Value: strPtr(strings.Repeat("а", 501))}
	patch, details = ValidateUpdateBuilding(UpdateBuildingRequest{Address: tooLong})
	if _, ok := details["address"]; !ok {
		t.Fatalf("ожидалась ошибка address: details=%v patch=%+v", details, patch)
	}

	inactive := false
	patch, details = ValidateUpdateBuilding(UpdateBuildingRequest{IsActive: &inactive})
	if len(details) != 0 || patch.IsActive == nil || *patch.IsActive {
		t.Fatalf("is_active=false: patch=%+v details=%v", patch, details)
	}
}

// EQ-1: имя склада обязательно, building_id положителен.
func TestValidateWarehouseRequests(t *testing.T) {
	in, details := ValidateCreateWarehouse(CreateWarehouseRequest{BuildingID: 3, Name: " Склад 1 "})
	if len(details) != 0 {
		t.Fatalf("details = %v", details)
	}
	if in.BuildingID != 3 || in.Name != "Склад 1" {
		t.Fatalf("in = %+v", in)
	}

	_, details = ValidateCreateWarehouse(CreateWarehouseRequest{BuildingID: 0, Name: ""})
	if _, ok := details["building_id"]; !ok {
		t.Fatalf("нет ошибки building_id: %v", details)
	}
	if _, ok := details["name"]; !ok {
		t.Fatalf("нет ошибки name: %v", details)
	}

	_, details = ValidateUpdateWarehouse(UpdateWarehouseRequest{Name: strings.Repeat("x", 201)})
	if _, ok := details["name"]; !ok {
		t.Fatalf("нет ошибки name: %v", details)
	}
}

func strPtr(value string) *string { return &value }

func deref(value *string) any {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func equalPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
