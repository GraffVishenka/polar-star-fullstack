// Package equipment реализует домен «Оборудование» (Б4, EQ-2, EQ-3, EQ-5, EQ-7):
// CRUD единиц, серверные фильтры/поиск/пагинация, перемещение между складами в
// транзакции и неизменяемый журнал перемещений.
//
// Чтение/создание/правка/перемещение доступны всем авторизованным (ограничение
// по зданию действует только на заявки — PLAN §3.1), удаление в корзину — только
// ADMIN (EQ-6). Массовое перемещение (EQ-4) реализуется блоком Б12.
package equipment

import "time"

// Equipment — единица оборудования (EQ-2). Всегда привязана к складу.
type Equipment struct {
	ID                 int64
	Name               string
	EquipmentTypeID    int64
	EquipmentTypeName  string
	SerialNumber       string
	RegistrationNumber string
	WarehouseID        int64
	WarehouseName      string
	BuildingID         int64
	BuildingName       string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

// Movement — запись журнала перемещений (EQ-5). Журнал неизменяем: API не
// предоставляет PATCH/DELETE, имена отдаются join'ом.
type Movement struct {
	ID                int64
	EquipmentID       int64
	FromWarehouseID   int64
	FromWarehouseName string
	FromBuildingName  string
	ToWarehouseID     int64
	ToWarehouseName   string
	ToBuildingName    string
	MovedByUserID     int64
	MovedByName       string
	MovedAt           time.Time
	Comment           *string
}

// WarehouseInfo — склад с зданием: минимум для правил размещения (EQ-3).
type WarehouseInfo struct {
	ID             int64
	Name           string
	BuildingID     int64
	BuildingName   string
	BuildingActive bool
}

// Actor — действующий пользователь для правил доступа домена.
// Роль берётся из БД при аутентификации (AUTH-6), не из токена.
type Actor struct {
	UserID  int64
	IsAdmin bool
}

// ListFilter — фильтры списка оборудования (EQ-7).
type ListFilter struct {
	BuildingID      *int64
	WarehouseID     *int64
	EquipmentTypeID *int64
	Search          string
	Page            int
	Size            int
}

// ListResult — страница оборудования.
type ListResult struct {
	Items []Equipment
	Total int
	Page  int
	Size  int
}

// MovementPage — страница журнала перемещений.
type MovementPage struct {
	Items []Movement
	Total int
	Page  int
	Size  int
}

// CreateInput — провалидированное тело POST /api/equipment.
type CreateInput struct {
	Name               string
	EquipmentTypeID    int64
	SerialNumber       string
	RegistrationNumber string
	WarehouseID        int64
}

// UpdateInput — частичное обновление единицы; nil-поля не меняются.
type UpdateInput struct {
	Name               *string
	EquipmentTypeID    *int64
	SerialNumber       *string
	RegistrationNumber *string
	WarehouseID        *int64
}

// Empty сообщает, что в PATCH не было ни одного поля.
func (in UpdateInput) Empty() bool {
	return in.Name == nil && in.EquipmentTypeID == nil && in.SerialNumber == nil &&
		in.RegistrationNumber == nil && in.WarehouseID == nil
}

// MoveInput — провалидированное тело POST /api/equipment/{id}/move.
type MoveInput struct {
	ToWarehouseID int64
	Comment       *string
}

// MoveResult — ответ на перемещение: единица и новая запись журнала.
type MoveResult struct {
	Equipment *Equipment
	Movement  *Movement
}
