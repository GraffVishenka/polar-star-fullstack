package equipment

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Ограничения полей (EQ-2, openapi Equipment/Move).
const (
	maxNameLength         = 200
	maxSerialLength       = 100
	maxRegistrationLength = 100
	maxSearchLength       = 200
	maxCommentLength      = 1000

	// DefaultPageSize и MaxPageSize — значения PageQuery/SizeQuery контракта.
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ValidateCreate проверяет тело POST /api/equipment.
func ValidateCreate(req CreateRequest) (CreateInput, map[string]string) {
	details := map[string]string{}

	name, ok := normalizeText(req.Name, maxNameLength)
	if !ok {
		details["name"] = requiredTextError(req.Name, maxNameLength)
	}
	serialNumber, ok := normalizeText(req.SerialNumber, maxSerialLength)
	if !ok {
		details["serial_number"] = requiredTextError(req.SerialNumber, maxSerialLength)
	}
	registrationNumber, ok := normalizeText(req.RegistrationNumber, maxRegistrationLength)
	if !ok {
		details["registration_number"] = requiredTextError(req.RegistrationNumber, maxRegistrationLength)
	}
	if req.EquipmentTypeID <= 0 {
		details["equipment_type_id"] = "должно быть положительным"
	}
	if req.WarehouseID <= 0 {
		details["warehouse_id"] = "должно быть положительным"
	}

	return CreateInput{
		Name:               name,
		EquipmentTypeID:    req.EquipmentTypeID,
		SerialNumber:       serialNumber,
		RegistrationNumber: registrationNumber,
		WarehouseID:        req.WarehouseID,
	}, details
}

// ValidateUpdate проверяет PATCH /api/equipment/{id}; пустой патч допустим.
func ValidateUpdate(req UpdateRequest) (UpdateInput, map[string]string) {
	details := map[string]string{}
	var in UpdateInput

	if req.Name != nil {
		name, ok := normalizeText(*req.Name, maxNameLength)
		if !ok {
			details["name"] = requiredTextError(*req.Name, maxNameLength)
		} else {
			in.Name = &name
		}
	}
	if req.SerialNumber != nil {
		serialNumber, ok := normalizeText(*req.SerialNumber, maxSerialLength)
		if !ok {
			details["serial_number"] = requiredTextError(*req.SerialNumber, maxSerialLength)
		} else {
			in.SerialNumber = &serialNumber
		}
	}
	if req.RegistrationNumber != nil {
		registrationNumber, ok := normalizeText(*req.RegistrationNumber, maxRegistrationLength)
		if !ok {
			details["registration_number"] = requiredTextError(*req.RegistrationNumber, maxRegistrationLength)
		} else {
			in.RegistrationNumber = &registrationNumber
		}
	}
	if req.EquipmentTypeID != nil {
		if *req.EquipmentTypeID <= 0 {
			details["equipment_type_id"] = "должно быть положительным"
		} else {
			in.EquipmentTypeID = req.EquipmentTypeID
		}
	}
	if req.WarehouseID != nil {
		if *req.WarehouseID <= 0 {
			details["warehouse_id"] = "должно быть положительным"
		} else {
			in.WarehouseID = req.WarehouseID
		}
	}

	return in, details
}

// ValidateMove проверяет тело POST /api/equipment/{id}/move (EQ-3).
func ValidateMove(req MoveRequest) (MoveInput, map[string]string) {
	details := map[string]string{}
	if req.ToWarehouseID <= 0 {
		details["to_warehouse_id"] = "должно быть положительным"
	}

	var comment *string
	if req.Comment != nil {
		value := strings.TrimSpace(*req.Comment)
		if value == "" {
			comment = nil
		} else if utf8.RuneCountInString(value) > maxCommentLength {
			details["comment"] = "не более 1000 символов"
		} else {
			comment = &value
		}
	}

	return MoveInput{ToWarehouseID: req.ToWarehouseID, Comment: comment}, details
}

// ParsePagination разбирает page/size контракта: page ≥ 1 (по умолчанию 1),
// size 1..100 (по умолчанию 20).
func ParsePagination(pageRaw, sizeRaw string) (int, int, map[string]string) {
	page, size := 1, DefaultPageSize
	details := map[string]string{}

	if raw := strings.TrimSpace(pageRaw); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			details["page"] = "ожидается целое число от 1"
		} else {
			page = value
		}
	}
	if raw := strings.TrimSpace(sizeRaw); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > MaxPageSize {
			details["size"] = "ожидается целое число от 1 до 100"
		} else {
			size = value
		}
	}

	return page, size, details
}

// ParseOptionalID разбирает необязательный положительный id из query.
// Пустая строка — фильтр не задан (nil, без ошибки).
func ParseOptionalID(raw string) (*int64, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, ""
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || value <= 0 {
		return nil, "ожидается положительное целое"
	}
	return &value, ""
}

// NormalizeSearch обрезает и проверяет строку поиска (EQ-7, maxLength 200).
func NormalizeSearch(raw string) (string, string) {
	search := strings.TrimSpace(raw)
	if utf8.RuneCountInString(search) > maxSearchLength {
		return "", "не более 200 символов"
	}
	return search, ""
}

// LikePattern превращает поисковую строку в LIKE-шаблон с экранированием
// спецсимволов: `%` и `_` из ввода не должны превращаться в wildcard.
// MySQL по умолчанию использует обратный слеш как escape-символ LIKE.
func LikePattern(search string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
	return "%" + escaped + "%"
}

// fieldDetails приводит ошибки валидации к виду details в ответе API.
func fieldDetails(details map[string]string) map[string]any {
	out := make(map[string]any, len(details))
	for field, message := range details {
		out[field] = message
	}
	return out
}

// normalizeText обрезает пробелы и проверяет непустую длину в символах:
// MySQL VARCHAR считает символы, а не байты (важно для кириллицы).
func normalizeText(raw string, maxLen int) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || utf8.RuneCountInString(value) > maxLen {
		return "", false
	}
	return value, true
}

// requiredTextError объясняет, почему обязательное текстовое поле не прошло.
func requiredTextError(raw string, maxLen int) string {
	if strings.TrimSpace(raw) == "" {
		return "обязательное поле"
	}
	return "не более " + strconv.Itoa(maxLen) + " символов"
}
