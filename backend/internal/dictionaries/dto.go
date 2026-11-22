package dictionaries

import "time"

// DTO — элемент справочника в API (openapi ProblemType/EquipmentType).
type DTO struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewDTO конвертирует доменную модель в DTO.
func NewDTO(item *Item) DTO {
	if item == nil {
		return DTO{}
	}
	return DTO{
		ID:        item.ID,
		Name:      item.Name,
		IsActive:  item.IsActive,
		SortOrder: item.SortOrder,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}
}

// NewDTOs конвертирует список (всегда не nil — в JSON будет []).
func NewDTOs(items []Item) []DTO {
	out := make([]DTO, 0, len(items))
	for i := range items {
		out = append(out, NewDTO(&items[i]))
	}
	return out
}

// CreateRequest — тело POST /api/admin/{problem-types|equipment-types}.
// is_active по умолчанию true, sort_order по умолчанию 0.
type CreateRequest struct {
	Name      string `json:"name"`
	IsActive  *bool  `json:"is_active"`
	SortOrder *int   `json:"sort_order"`
}

// UpdateRequest — тело PATCH; все поля необязательны.
type UpdateRequest struct {
	Name      *string `json:"name"`
	IsActive  *bool   `json:"is_active"`
	SortOrder *int    `json:"sort_order"`
}

// PublicProblemTypeDTO — публичный элемент справочника видов проблем для формы
// анонимной заявки: только id, name, sort_order. is_active и служебные поля не
// раскрываются.
type PublicProblemTypeDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
}

// NewPublicProblemTypeDTOs конвертирует список (всегда не nil).
func NewPublicProblemTypeDTOs(items []Item) []PublicProblemTypeDTO {
	out := make([]PublicProblemTypeDTO, 0, len(items))
	for _, item := range items {
		out = append(out, PublicProblemTypeDTO{ID: item.ID, Name: item.Name, SortOrder: item.SortOrder})
	}
	return out
}
