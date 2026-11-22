package dictionaries

import (
	"strings"
	"unicode/utf8"
)

// ValidateCreate проверяет тело POST: имя обязательно, 1..120 символов.
func ValidateCreate(req CreateRequest) (CreateInput, map[string]string) {
	details := map[string]string{}

	name, ok := normalizeName(req.Name)
	if !ok {
		details["name"] = nameError(req.Name)
	}

	in := CreateInput{Name: name, IsActive: true, SortOrder: 0}
	if req.IsActive != nil {
		in.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	return in, details
}

// ValidateUpdate проверяет PATCH; пустой патч допустим.
func ValidateUpdate(req UpdateRequest) (UpdateInput, map[string]string) {
	details := map[string]string{}
	var in UpdateInput

	if req.Name != nil {
		name, ok := normalizeName(*req.Name)
		if !ok {
			details["name"] = nameError(*req.Name)
		} else {
			in.Name = &name
		}
	}
	if req.IsActive != nil {
		in.IsActive = req.IsActive
	}
	if req.SortOrder != nil {
		in.SortOrder = req.SortOrder
	}
	return in, details
}

// normalizeName обрезает пробелы и проверяет длину в символах: MySQL VARCHAR
// считает символы, а не байты (важно для кириллицы).
func normalizeName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		return "", false
	}
	return name, true
}

func nameError(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "обязательное поле"
	}
	return "не более 120 символов"
}

// fieldDetails приводит ошибки валидации к виду details в ответе API.
func fieldDetails(details map[string]string) map[string]any {
	out := make(map[string]any, len(details))
	for field, message := range details {
		out[field] = message
	}
	return out
}
