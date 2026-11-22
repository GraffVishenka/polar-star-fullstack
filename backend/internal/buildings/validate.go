package buildings

import (
	"strings"
	"unicode/utf8"
)

// Ограничения полей (EQ-1, openapi Building/Warehouse).
const (
	maxNameLength    = 200
	maxAddressLength = 500
)

// ValidateCreateBuilding нормализует и проверяет тело POST /api/buildings.
// Возвращает провалидированный вход и карту ошибок по полям (пустая — ок).
func ValidateCreateBuilding(req CreateBuildingRequest) (BuildingInput, map[string]string) {
	details := map[string]string{}

	name, nameOK := normalizeName(req.Name)
	if !nameOK {
		details["name"] = nameError(req.Name)
	}
	address, addressErr := normalizeAddressValue(req.Address)
	if addressErr != "" {
		details["address"] = addressErr
	}

	return BuildingInput{Name: name, Address: address}, details
}

// ValidateUpdateBuilding проверяет PATCH /api/buildings/{id}. Пустой патч
// допустим: он возвращает текущее состояние здания без изменений.
func ValidateUpdateBuilding(req UpdateBuildingRequest) (BuildingPatch, map[string]string) {
	details := map[string]string{}
	var patch BuildingPatch

	if req.Name != nil {
		name, ok := normalizeName(*req.Name)
		if !ok {
			details["name"] = nameError(*req.Name)
		} else {
			patch.Name = &name
		}
	}
	if req.Address.Set {
		address, addressErr := normalizeAddressValue(req.Address.Value)
		if addressErr != "" {
			details["address"] = addressErr
		} else {
			// AddressSet=true даже при nil: явный null очищает адрес.
			patch.Address = address
			patch.AddressSet = true
		}
	}
	if req.IsActive != nil {
		patch.IsActive = req.IsActive
	}

	return patch, details
}

// ValidateCreateWarehouse проверяет POST /api/warehouses (EQ-1).
func ValidateCreateWarehouse(req CreateWarehouseRequest) (WarehouseInput, map[string]string) {
	details := map[string]string{}
	if req.BuildingID <= 0 {
		details["building_id"] = "должно быть положительным"
	}
	name, ok := normalizeName(req.Name)
	if !ok {
		details["name"] = nameError(req.Name)
	}
	return WarehouseInput{BuildingID: req.BuildingID, Name: name}, details
}

// ValidateUpdateWarehouse проверяет PATCH /api/warehouses/{id}.
func ValidateUpdateWarehouse(req UpdateWarehouseRequest) (WarehouseInput, map[string]string) {
	details := map[string]string{}
	name, ok := normalizeName(req.Name)
	if !ok {
		details["name"] = nameError(req.Name)
	}
	return WarehouseInput{Name: name}, details
}

// fieldDetails приводит ошибки валидации к виду details в ответе API.
func fieldDetails(details map[string]string) map[string]any {
	out := make(map[string]any, len(details))
	for field, message := range details {
		out[field] = message
	}
	return out
}

// normalizeName обрезает пробелы и проверяет длину в символах: MySQL VARCHAR
// считает символы, поэтому кириллица не должна урезаться вдвое.
func normalizeName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return "", false
	}
	return name, true
}

// nameError объясняет, почему название не прошло валидацию.
func nameError(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "обязательное поле"
	}
	return "не более 200 символов"
}

// normalizeAddressValue нормализует необязательный адрес: пустая строка
// превращается в NULL (в контракте address nullable).
func normalizeAddressValue(raw *string) (*string, string) {
	if raw == nil {
		return nil, ""
	}
	address := strings.TrimSpace(*raw)
	if address == "" {
		return nil, ""
	}
	if utf8.RuneCountInString(address) > maxAddressLength {
		return nil, "не более 500 символов"
	}
	return &address, ""
}
