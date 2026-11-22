// Package buildings реализует домен «Здания и склады» (Б4, EQ-1, EQ-6):
// справочник зданий с деактивацией вместо удаления, склады/кабинеты здания и
// правила физического удаления по связанным данным.
//
// Чтение зданий и складов доступно всем авторизованным (включая неактивные —
// они нужны в истории и фильтрах), запись/деактивация — только ADMIN
// (PLAN §3.1, §3.6).
package buildings

import "time"

// Building — здание (таблица buildings, миграция 0001_core).
type Building struct {
	ID        int64
	Name      string
	Address   *string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Warehouse — склад/кабинет здания (EQ-1). Название уникально в рамках здания.
type Warehouse struct {
	ID           int64
	BuildingID   int64
	BuildingName string
	Name         string
	CreatedAt    time.Time
}

// BuildingUsage — счётчики связанных сущностей для правила EQ-6.
type BuildingUsage struct {
	Warehouses int
	Equipment  int
	Users      int
}

// BuildingPatch — частичное обновление здания (PATCH /api/buildings/{id}).
// AddressSet отделяет «поле не передано» от «передано null» (очистка адреса).
type BuildingPatch struct {
	Name       *string
	Address    *string
	AddressSet bool
	IsActive   *bool
}

// Empty сообщает, что в PATCH не было ни одного поля: запрос идемпотентен и
// возвращает текущее состояние здания.
func (p BuildingPatch) Empty() bool {
	return p.Name == nil && !p.AddressSet && p.IsActive == nil
}

// BuildingInput — провалидированное тело POST /api/buildings.
type BuildingInput struct {
	Name    string
	Address *string
}

// WarehouseInput — провалидированное тело создания/переименования склада.
type WarehouseInput struct {
	BuildingID int64
	Name       string
}
