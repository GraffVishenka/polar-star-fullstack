// Package dictionaries реализует справочники «Виды проблем» и «Типы
// оборудования» (Б7, PLAN §3.13, TICKET-2/EQ-2): CRUD для ADMIN, чтение для
// USER/ADMIN.
//
// Удаление запрещено: справочники используются в исторических заявках и
// оборудовании, поэтому DELETE переводит запись в is_active=false (деактивация).
// Деактивированный элемент исчезает из активных списков, но остаётся в старых
// записях и доступен ADMIN с include_inactive=true.
package dictionaries

import "time"

// Kind — вид справочника. Имена таблиц берутся из whitelist (см. table), в SQL
// никогда не попадает пользовательский ввод.
type Kind string

const (
	// KindProblemTypes — справочник видов проблем (tickets.problem_type_id).
	KindProblemTypes Kind = "problem_types"
	// KindEquipmentTypes — справочник типов оборудования (equipment.equipment_type_id).
	KindEquipmentTypes Kind = "equipment_types"
)

// maxNameLength — openapi ProblemType/EquipmentType: name ≤ 120.
const maxNameLength = 120

// Item — элемент справочника.
type Item struct {
	ID        int64
	Name      string
	IsActive  bool
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateInput — провалидированное тело POST.
type CreateInput struct {
	Name      string
	IsActive  bool
	SortOrder int
}

// UpdateInput — частичное обновление (PATCH); nil-поля не меняются.
type UpdateInput struct {
	Name      *string
	IsActive  *bool
	SortOrder *int
}

// Empty сообщает, что в PATCH не было ни одного поля.
func (in UpdateInput) Empty() bool {
	return in.Name == nil && in.IsActive == nil && in.SortOrder == nil
}

// table возвращает имя таблицы для вида справочника. Второе значение — false
// для неизвестного вида: такой запрос должен превратиться в 500/404, но не в
// SQL-инъекцию.
func (k Kind) table() (string, bool) {
	switch k {
	case KindProblemTypes:
		return "problem_types", true
	case KindEquipmentTypes:
		return "equipment_types", true
	default:
		return "", false
	}
}
