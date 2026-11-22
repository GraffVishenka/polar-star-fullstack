package equipment

// CheckMove — правило EQ-3 для перемещения единицы:
//   - в тот же склад → ErrSameWarehouse (409);
//   - USER не может переместить оборудование в деактивированное здание →
//     ErrBuildingInactive (422), ADMIN может.
//
// Функция чистая: репозиторий вызывает её внутри транзакции уже после
// SELECT ... FOR UPDATE, а тесты проверяют правило без MySQL.
func CheckMove(fromWarehouseID int64, target WarehouseInfo, actor Actor) error {
	if fromWarehouseID == target.ID {
		return ErrSameWarehouse
	}
	return CheckWarehouseTarget(target, actor)
}

// CheckWarehouseTarget — общее правило размещения оборудования. Применяется
// при перемещении (EQ-3) и при смене склада через PATCH, чтобы USER не мог
// обойти запрет перемещения в неактивное здание другим эндпоинтом.
// Создание единицы — первичное размещение, а не перемещение: правило EQ-3 на
// него не распространяется (неактивные здания всё равно не предлагаются в
// формах — EQ-6).
func CheckWarehouseTarget(target WarehouseInfo, actor Actor) error {
	if target.BuildingActive || actor.IsAdmin {
		return nil
	}
	return ErrBuildingInactive
}
