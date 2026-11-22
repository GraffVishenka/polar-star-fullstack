// Package trash реализует корзину и восстановление (Б10, TRASH-1..TRASH-5):
// список удалённых заявок, узлов базы знаний и оборудования, восстановление из
// корзины, необратимое физическое удаление и ежедневную автоочистку элементов
// старше 90 дней (по часовому поясу приложения).
//
// Домен не дублирует soft delete: удаление в корзину по-прежнему выполняют
// сервисы Б4/Б5/Б6 (они теперь фиксируют автора в `deleted_by`), а `trash`
// работает с уже помеченными строками.
package trash

import (
	"errors"
	"time"
)

// Type — тип элемента корзины (openapi TrashType).
type Type string

const (
	// TypeTicket — заявка (tickets.deleted_at).
	TypeTicket Type = "ticket"
	// TypeKB — узел базы знаний (kb_nodes.deleted_at, поддерево).
	TypeKB Type = "kb"
	// TypeEquipment — единица оборудования (equipment.deleted_at).
	TypeEquipment Type = "equipment"
)

// Types — все поддерживаемые типы; порядок используется автоочисткой.
var Types = []Type{TypeTicket, TypeKB, TypeEquipment}

// Valid сообщает, что тип известен контракту.
func (t Type) Valid() bool {
	switch t {
	case TypeTicket, TypeKB, TypeEquipment:
		return true
	default:
		return false
	}
}

// DefaultRetention — TRASH-4: элементы, удалённые более 90 дней назад,
// удаляются физически.
const DefaultRetention = 90 * 24 * time.Hour

// Значения по умолчанию для пагинации и автоочистки.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	defaultBatch    = 100
)

// Item — элемент списка корзины (openapi TrashItem). Title — краткое описание
// по типу: заявка (#id, заявитель, кабинет, здание), узел БЗ (заголовок,
// родитель), оборудование (название, рег. номер, склад).
type Item struct {
	Type            Type
	ID              int64
	Title           string
	DeletedAt       time.Time
	DeletedByUserID *int64
	DeletedByName   *string
	BuildingID      *int64
	BuildingName    *string
}

// Page — страница корзины.
type Page struct {
	Items []Item
	Total int
	Page  int
	Size  int
}

// RestoreResult — результат восстановления (openapi TrashRestoreResponse).
type RestoreResult struct {
	Type     Type
	ID       int64
	Restored bool
	Warnings []string
}

// TicketRestore — данные заявки, нужные для правил восстановления (TRASH-2).
type TicketRestore struct {
	BuildingID     int64
	BuildingName   string
	BuildingActive bool
}

// KBRestore — данные узла БЗ для правил восстановления: жив ли родитель и
// сколько потомков осталось в корзине (TRASH-5: они не восстанавливаются).
type KBRestore struct {
	ParentID        *int64
	ParentAlive     bool
	DeletedChildren int
}

// EquipmentRestore — данные оборудования для правил восстановления.
type EquipmentRestore struct {
	WarehouseID    int64
	WarehouseName  string
	BuildingName   string
	BuildingActive bool
}

// Ошибки домена. Сервис переводит их в типизированные ответы API.
var (
	// ErrNotFound — элемент не найден или не находится в корзине.
	ErrNotFound = errors.New("trash: элемент корзины не найден")
	// ErrParentDeleted — родительский узел БЗ удалён: сначала восстановите его.
	ErrParentDeleted = errors.New("trash: родительский узел удалён")
	// ErrWarehouseDeleted — склад оборудования удалён.
	ErrWarehouseDeleted = errors.New("trash: склад оборудования удалён")
	// ErrDuplicateRegistration — регистрационный номер занят другим
	// неудалённым оборудованием (EQ-2).
	ErrDuplicateRegistration = errors.New("trash: регистрационный номер уже используется")
	// ErrUnknownType — неизвестный тип элемента корзины.
	ErrUnknownType = errors.New("trash: неизвестный тип элемента корзины")
)

// FileRemover удаляет файл с диска. Реализуется files.Storage.Remove, чтобы
// домен корзины не зависел от пакета files.
type FileRemover interface {
	Remove(rel string) error
}
