package trash

import "time"

// ItemDTO — элемент корзины в API (openapi TrashItem).
type ItemDTO struct {
	Type            Type      `json:"type"`
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	DeletedAt       time.Time `json:"deleted_at"`
	DeletedByUserID *int64    `json:"deleted_by_user_id"`
	DeletedByName   *string   `json:"deleted_by_name"`
	BuildingID      *int64    `json:"building_id"`
	BuildingName    *string   `json:"building_name"`
}

// ListDTO — страница корзины (openapi TrashListResponse).
type ListDTO struct {
	Items []ItemDTO `json:"items"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Size  int       `json:"size"`
}

// RestoreDTO — ответ восстановления (openapi TrashRestoreResponse).
type RestoreDTO struct {
	Type     Type     `json:"type"`
	ID       int64    `json:"id"`
	Restored bool     `json:"restored"`
	Warnings []string `json:"warnings"`
}

// NewListDTO собирает страницу ответа; items всегда массив (не null).
func NewListDTO(page *Page) ListDTO {
	items := make([]ItemDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, NewItemDTO(item))
	}
	return ListDTO{Items: items, Total: page.Total, Page: page.Page, Size: page.Size}
}

// NewItemDTO конвертирует элемент корзины.
func NewItemDTO(item Item) ItemDTO {
	return ItemDTO{
		Type:            item.Type,
		ID:              item.ID,
		Title:           item.Title,
		DeletedAt:       item.DeletedAt,
		DeletedByUserID: item.DeletedByUserID,
		DeletedByName:   item.DeletedByName,
		BuildingID:      item.BuildingID,
		BuildingName:    item.BuildingName,
	}
}

// NewRestoreDTO конвертирует результат восстановления; warnings всегда массив.
func NewRestoreDTO(result *RestoreResult) RestoreDTO {
	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	return RestoreDTO{Type: result.Type, ID: result.ID, Restored: result.Restored, Warnings: warnings}
}
