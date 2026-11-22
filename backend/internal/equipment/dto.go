package equipment

import "time"

// DTO — представление единицы оборудования в API (openapi Equipment).
type DTO struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	EquipmentTypeID    int64      `json:"equipment_type_id"`
	EquipmentTypeName  string     `json:"equipment_type_name"`
	SerialNumber       string     `json:"serial_number"`
	RegistrationNumber string     `json:"registration_number"`
	WarehouseID        int64      `json:"warehouse_id"`
	WarehouseName      string     `json:"warehouse_name"`
	BuildingID         int64      `json:"building_id"`
	BuildingName       string     `json:"building_name"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at"`
}

// NewDTO конвертирует доменную модель в DTO.
func NewDTO(e *Equipment) DTO {
	if e == nil {
		return DTO{}
	}
	return DTO{
		ID:                 e.ID,
		Name:               e.Name,
		EquipmentTypeID:    e.EquipmentTypeID,
		EquipmentTypeName:  e.EquipmentTypeName,
		SerialNumber:       e.SerialNumber,
		RegistrationNumber: e.RegistrationNumber,
		WarehouseID:        e.WarehouseID,
		WarehouseName:      e.WarehouseName,
		BuildingID:         e.BuildingID,
		BuildingName:       e.BuildingName,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
		DeletedAt:          e.DeletedAt,
	}
}

// ListDTO — страница оборудования (openapi EquipmentListResponse).
type ListDTO struct {
	Items []DTO `json:"items"`
	Total int   `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// NewListDTO формирует страницу для ответа.
func NewListDTO(result *ListResult) ListDTO {
	items := make([]DTO, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, NewDTO(&result.Items[i]))
	}
	return ListDTO{Items: items, Total: result.Total, Page: result.Page, Size: result.Size}
}

// MovementDTO — запись журнала перемещений (openapi EquipmentMovement).
type MovementDTO struct {
	ID                int64     `json:"id"`
	EquipmentID       int64     `json:"equipment_id"`
	FromWarehouseID   int64     `json:"from_warehouse_id"`
	FromWarehouseName string    `json:"from_warehouse_name"`
	FromBuildingName  string    `json:"from_building_name"`
	ToWarehouseID     int64     `json:"to_warehouse_id"`
	ToWarehouseName   string    `json:"to_warehouse_name"`
	ToBuildingName    string    `json:"to_building_name"`
	MovedByUserID     int64     `json:"moved_by_user_id"`
	MovedByName       string    `json:"moved_by_name"`
	MovedAt           time.Time `json:"moved_at"`
	Comment           *string   `json:"comment"`
}

// NewMovementDTO конвертирует доменную модель в DTO.
func NewMovementDTO(m *Movement) MovementDTO {
	if m == nil {
		return MovementDTO{}
	}
	return MovementDTO{
		ID:                m.ID,
		EquipmentID:       m.EquipmentID,
		FromWarehouseID:   m.FromWarehouseID,
		FromWarehouseName: m.FromWarehouseName,
		FromBuildingName:  m.FromBuildingName,
		ToWarehouseID:     m.ToWarehouseID,
		ToWarehouseName:   m.ToWarehouseName,
		ToBuildingName:    m.ToBuildingName,
		MovedByUserID:     m.MovedByUserID,
		MovedByName:       m.MovedByName,
		MovedAt:           m.MovedAt,
		Comment:           m.Comment,
	}
}

// MovementListDTO — страница журнала (openapi EquipmentMovementListResponse).
type MovementListDTO struct {
	Items []MovementDTO `json:"items"`
	Total int           `json:"total"`
	Page  int           `json:"page"`
	Size  int           `json:"size"`
}

// NewMovementListDTO формирует страницу журнала для ответа.
func NewMovementListDTO(page *MovementPage) MovementListDTO {
	items := make([]MovementDTO, 0, len(page.Items))
	for i := range page.Items {
		items = append(items, NewMovementDTO(&page.Items[i]))
	}
	return MovementListDTO{Items: items, Total: page.Total, Page: page.Page, Size: page.Size}
}

// MoveResponseDTO — ответ POST /api/equipment/{id}/move
// (openapi EquipmentMoveResponse): единица после перемещения + запись журнала.
type MoveResponseDTO struct {
	Equipment DTO         `json:"equipment"`
	Movement  MovementDTO `json:"movement"`
}

// NewMoveResponseDTO собирает ответ на перемещение.
func NewMoveResponseDTO(result *MoveResult) MoveResponseDTO {
	return MoveResponseDTO{
		Equipment: NewDTO(result.Equipment),
		Movement:  NewMovementDTO(result.Movement),
	}
}

// CreateRequest — тело POST /api/equipment (EQ-2).
type CreateRequest struct {
	Name               string `json:"name"`
	EquipmentTypeID    int64  `json:"equipment_type_id"`
	SerialNumber       string `json:"serial_number"`
	RegistrationNumber string `json:"registration_number"`
	WarehouseID        int64  `json:"warehouse_id"`
}

// UpdateRequest — тело PATCH /api/equipment/{id}; все поля необязательны.
type UpdateRequest struct {
	Name               *string `json:"name"`
	EquipmentTypeID    *int64  `json:"equipment_type_id"`
	SerialNumber       *string `json:"serial_number"`
	RegistrationNumber *string `json:"registration_number"`
	WarehouseID        *int64  `json:"warehouse_id"`
}

// MoveRequest — тело POST /api/equipment/{id}/move (EQ-3).
type MoveRequest struct {
	ToWarehouseID int64   `json:"to_warehouse_id"`
	Comment       *string `json:"comment"`
}

// BulkMoveRequest — тело POST /api/equipment/bulk-move (EQ-4).
type BulkMoveRequest struct {
	EquipmentIDs  []int64 `json:"equipment_ids"`
	ToWarehouseID int64   `json:"to_warehouse_id"`
	Comment       *string `json:"comment"`
}

// BulkMoveResponseDTO — ответ массового перемещения (контракт
// EquipmentBulkMoveResponse). `skipped_count` — аддитивное поле: единицы,
// которые уже находились в целевом складе и не создали запись журнала.
type BulkMoveResponseDTO struct {
	MovedCount   int           `json:"moved_count"`
	SkippedCount int           `json:"skipped_count"`
	Movements    []MovementDTO `json:"movements"`
}

// NewBulkMoveResponseDTO собирает ответ на массовое перемещение (movements
// всегда массив).
func NewBulkMoveResponseDTO(result *BulkMoveResult) BulkMoveResponseDTO {
	movements := make([]MovementDTO, 0, len(result.Movements))
	for _, movement := range result.Movements {
		movements = append(movements, NewMovementDTO(movement))
	}
	return BulkMoveResponseDTO{
		MovedCount:   result.MovedCount,
		SkippedCount: result.SkippedCount,
		Movements:    movements,
	}
}
