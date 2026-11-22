// Package kb реализует базу знаний (Б6, PLAN §3.5 KB-1..KB-8): дерево статей
// с неограниченной вложенностью и ручной сортировкой, скрытые корневые разделы,
// санитизацию HTML и проверку ссылок на медиа.
//
// Слои: Handler → Service → Repository. Правила (циклы, видимость, порядок,
// санитизация, проверка медиа) живут в Service; работа с БД и транзакционная
// переиндексация sort_order — в Repository.
package kb

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Лимиты KB-1.
const (
	// MaxTitleRunes — длина заголовка 1..200.
	MaxTitleRunes = 200
	// MaxContentRunes — длина content_html 0..100 000.
	MaxContentRunes = 100_000
	// SortOrderStep — шаг переиндексации при перемещении (KB-6).
	SortOrderStep = 10
)

// Ошибки домена; handler переводит их в коды контракта.
var (
	// ErrNotFound — узел отсутствует или удалён в корзину.
	ErrNotFound = errors.New("kb: узел не найден")
	// ErrParentNotFound — родитель для создания/перемещения не найден.
	ErrParentNotFound = errors.New("kb: родительский узел не найден")
	// ErrParentDeleted — родитель удалён в корзину (KB-2: 400).
	ErrParentDeleted = errors.New("kb: родительский узел удалён")
	// ErrCycle — перемещение в собственное поддерево (KB-2: 409 CYCLE_DETECTED).
	ErrCycle = errors.New("kb: перемещение в собственное поддерево")
	// ErrBeforeNotFound — before_node_id не является соседом нового родителя.
	ErrBeforeNotFound = errors.New("kb: узел before_node_id не найден среди соседей")
	// ErrVisibilityRootOnly — видимость задаётся только корневым разделам (KB-7).
	ErrVisibilityRootOnly = errors.New("kb: видимость задаётся только для корневого раздела")
	// ErrMediaNotFound — ссылка на медиа недопустима (KB-5: 400 INVALID_MEDIA_REF).
	ErrMediaNotFound = errors.New("kb: файл медиа не найден или недоступен")
)

// Node — узел дерева базы знаний (таблица kb_nodes).
type Node struct {
	ID              int64
	Title           string
	ContentHTML     string
	ParentID        *int64
	SortOrder       int
	IsHiddenForAnon bool
	CreatedBy       *int64
	UpdatedBy       *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	// DeletedAt непустой у узлов в корзине; такие узлы не отдаются API.
	DeletedAt *time.Time
}

// IsDeleted сообщает, что узел в корзине.
func (n *Node) IsDeleted() bool { return n != nil && n.DeletedAt != nil }

// Breadcrumb — элемент пути от корня до узла (контракт KbNode.breadcrumbs).
type Breadcrumb struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// TreeNode — узел для ответа дерева с детьми.
type TreeNode struct {
	Node
	Children []*TreeNode
}

// MediaCandidate — узел, в content_html которого встречается ссылка на файл
// (для проверки публичной доступности медиа БЗ).
type MediaCandidate struct {
	ID          int64
	ContentHTML string
}

// CreateInput — провалидированное тело POST /api/kb/nodes.
type CreateInput struct {
	Title           string
	ContentHTML     string
	ParentID        *int64
	SortOrder       *int
	IsHiddenForAnon bool
}

// UpdateInput — тело PATCH /api/kb/nodes/{id}. Указатели означают «поле
// передано»; ParentID.Set отличает отсутствие поля от явного `null`
// (перемещение в корень).
type UpdateInput struct {
	Title       *string
	ContentHTML *string
	ParentID    OptionalInt64
	SortOrder   *int
}

// MoveInput — тело POST /api/kb/nodes/{id}/move (KB-6).
type MoveInput struct {
	// ParentID обязателен; Value=nil — перемещение в корень.
	ParentID     OptionalInt64
	BeforeNodeID *int64
}

// CreateParams — данные для INSERT.
type CreateParams struct {
	Title           string
	ContentHTML     string
	ParentID        *int64
	SortOrder       int
	IsHiddenForAnon bool
	ActorID         int64
}

// UpdateParams — итоговые значения для UPDATE после бизнес-валидации.
type UpdateParams struct {
	ID          int64
	Title       *string
	ContentHTML *string
	ParentID    *int64
	ParentIDSet bool
	SortOrder   *int
	ActorID     int64
}

// OptionalInt64 — nullable-число с признаком «поле присутствует в JSON».
// Нужно, чтобы отличить отсутствующий parent_id (не менять) от `parent_id: null`
// (переместить в корень).
type OptionalInt64 struct {
	Set   bool
	Value *int64
}

// UnmarshalJSON реализует json.Unmarshaler.
func (o *OptionalInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

// Repository — хранилище дерева БЗ (MySQL в проде, in-memory в тестах).
type Repository interface {
	// List возвращает все узлы вне корзины (для построения дерева).
	List(ctx context.Context) ([]Node, error)
	// Get возвращает живой узел; удалённый или отсутствующий — ErrNotFound.
	Get(ctx context.Context, id int64) (*Node, error)
	// ParentStatus сообщает, существует ли узел и не в корзине ли он. Нужен,
	// чтобы отличить «родитель не найден» (404) от «родитель удалён» (400, KB-2).
	ParentStatus(ctx context.Context, id int64) (exists bool, deleted bool, err error)
	// Create вставляет узел и возвращает его с проставленным id.
	Create(ctx context.Context, params CreateParams) (*Node, error)
	// Update применяет частичные изменения.
	Update(ctx context.Context, params UpdateParams) (*Node, error)
	// Move меняет родителя и переиндексирует sort_order соседей с шагом 10 в
	// одной транзакции (KB-6). beforeNodeID=nil — в конец списка.
	Move(ctx context.Context, nodeID int64, parentID *int64, beforeNodeID *int64, actorID int64) error
	// SoftDelete отправляет узел и всё поддерево в корзину, возвращает число
	// удалённых потомков (без самого узла).
	SoftDelete(ctx context.Context, id int64, at time.Time) (descendants int, err error)
	// SetVisibility переключает видимость корневого раздела (KB-7).
	SetVisibility(ctx context.Context, id int64, hidden bool, actorID int64) (*Node, error)
	// Ancestors возвращает путь от корня до узла включительно.
	Ancestors(ctx context.Context, id int64) ([]Breadcrumb, error)
	// IsAncestorOrSelf сообщает, является ли candidate предком узла nodeID
	// (или самим узлом). Используется для запрета циклов.
	IsAncestorOrSelf(ctx context.Context, candidate, nodeID int64) (bool, error)
	// MaxSortOrder — максимальный sort_order среди живых детей parentID
	// (0, если детей нет).
	MaxSortOrder(ctx context.Context, parentID *int64) (int, error)
	// MediaCandidates возвращает живые узлы, content_html которых содержит
	// подстроку like (предварительный отбор для точной проверки ссылки).
	MediaCandidates(ctx context.Context, like string) ([]MediaCandidate, error)
	// NodePublic сообщает, что узел и все его предки не удалены, а корень не
	// скрыт от анонимов (KB-7).
	NodePublic(ctx context.Context, id int64) (bool, error)
}
