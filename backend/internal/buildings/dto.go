package buildings

import (
	"encoding/json"
	"time"
)

// BuildingDTO — представление здания в API (openapi Building).
type BuildingDTO struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Address   *string   `json:"address"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// NewBuildingDTO конвертирует доменную модель в DTO.
func NewBuildingDTO(b *Building) BuildingDTO {
	if b == nil {
		return BuildingDTO{}
	}
	return BuildingDTO{
		ID:        b.ID,
		Name:      b.Name,
		Address:   b.Address,
		IsActive:  b.IsActive,
		CreatedAt: b.CreatedAt,
	}
}

// NewBuildingDTOs конвертирует список (всегда не nil — в JSON будет []).
func NewBuildingDTOs(items []Building) []BuildingDTO {
	out := make([]BuildingDTO, 0, len(items))
	for i := range items {
		out = append(out, NewBuildingDTO(&items[i]))
	}
	return out
}

// WarehouseDTO — представление склада в API (openapi Warehouse).
type WarehouseDTO struct {
	ID           int64     `json:"id"`
	BuildingID   int64     `json:"building_id"`
	BuildingName string    `json:"building_name"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
}

// NewWarehouseDTO конвертирует доменную модель в DTO.
func NewWarehouseDTO(w *Warehouse) WarehouseDTO {
	if w == nil {
		return WarehouseDTO{}
	}
	return WarehouseDTO{
		ID:           w.ID,
		BuildingID:   w.BuildingID,
		BuildingName: w.BuildingName,
		Name:         w.Name,
		CreatedAt:    w.CreatedAt,
	}
}

// NewWarehouseDTOs конвертирует список (всегда не nil).
func NewWarehouseDTOs(items []Warehouse) []WarehouseDTO {
	out := make([]WarehouseDTO, 0, len(items))
	for i := range items {
		out = append(out, NewWarehouseDTO(&items[i]))
	}
	return out
}

// CreateBuildingRequest — тело POST /api/buildings.
type CreateBuildingRequest struct {
	Name    string  `json:"name"`
	Address *string `json:"address"`
}

// UpdateBuildingRequest — тело PATCH /api/buildings/{id}. Все поля необязательны.
type UpdateBuildingRequest struct {
	Name     *string        `json:"name"`
	Address  OptionalString `json:"address"`
	IsActive *bool          `json:"is_active"`
}

// CreateWarehouseRequest — тело POST /api/warehouses.
type CreateWarehouseRequest struct {
	BuildingID int64  `json:"building_id"`
	Name       string `json:"name"`
}

// UpdateWarehouseRequest — тело PATCH /api/warehouses/{id}.
type UpdateWarehouseRequest struct {
	Name string `json:"name"`
}

// OptionalString — nullable-строка с признаком «поле присутствует в JSON».
// Нужна, чтобы отличить отсутствующий `address` (не менять) от `address: null`
// (очистить адрес).
type OptionalString struct {
	Set   bool
	Value *string
}

// UnmarshalJSON реализует json.Unmarshaler.
func (o *OptionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}
