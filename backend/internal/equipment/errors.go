package equipment

import "errors"

// Доменные ошибки. Сервис переводит их в типизированные ответы API, репозиторий
// возвращает как есть — так правила тестируются без MySQL.
var (
	// ErrNotFound — единица оборудования не найдена (в том числе удалённая).
	ErrNotFound = errors.New("equipment: оборудование не найдено")
	// ErrWarehouseNotFound — склад не найден (в том числе удалённый).
	ErrWarehouseNotFound = errors.New("equipment: склад не найден")
	// ErrDuplicateRegistration — регистрационный номер занят среди неудалённых
	// (EQ-2) → 409 DUPLICATE_REGISTRATION_NUMBER.
	ErrDuplicateRegistration = errors.New("equipment: регистрационный номер уже используется")
	// ErrSameWarehouse — перемещение в тот же склад (EQ-3) → 409 SAME_WAREHOUSE.
	ErrSameWarehouse = errors.New("equipment: оборудование уже в этом складе")
	// ErrBuildingInactive — USER пытается разместить оборудование в
	// деактивированном здании (EQ-3) → 422 BUILDING_INACTIVE.
	ErrBuildingInactive = errors.New("equipment: здание склада деактивировано")
	// ErrInvalidSelection — некорректный набор единиц для массового перемещения
	// (пусто или больше 200, EQ-4) → 422 VALIDATION_ERROR. Сервис проверяет
	// набор заранее, репозиторий — защитно, внутри транзакции.
	ErrInvalidSelection = errors.New("equipment: некорректный набор единиц оборудования")
)
