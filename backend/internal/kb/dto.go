package kb

import "time"

// KbNodeDTO — статья базы знаний (контракт KbNode в openapi.yaml).
type KbNodeDTO struct {
	ID              int64        `json:"id"`
	Title           string       `json:"title"`
	ContentHTML     string       `json:"content_html"`
	ParentID        *int64       `json:"parent_id"`
	SortOrder       int          `json:"sort_order"`
	IsHiddenForAnon bool         `json:"is_hidden_for_anon"`
	Breadcrumbs     []Breadcrumb `json:"breadcrumbs"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

// NewKbNodeDTO конвертирует доменную модель и путь до узла в DTO. breadcrumbs
// всегда массив (не null), чтобы клиент не проверял nil.
func NewKbNodeDTO(node *Node, crumbs []Breadcrumb) KbNodeDTO {
	if node == nil {
		return KbNodeDTO{}
	}
	if crumbs == nil {
		crumbs = []Breadcrumb{}
	}
	return KbNodeDTO{
		ID:              node.ID,
		Title:           node.Title,
		ContentHTML:     node.ContentHTML,
		ParentID:        node.ParentID,
		SortOrder:       node.SortOrder,
		IsHiddenForAnon: node.IsHiddenForAnon,
		Breadcrumbs:     crumbs,
		CreatedAt:       node.CreatedAt,
		UpdatedAt:       node.UpdatedAt,
	}
}

// KbTreeNodeDTO — узел дерева без содержимого статьи (контракт KbTreeNode).
type KbTreeNodeDTO struct {
	ID              int64           `json:"id"`
	Title           string          `json:"title"`
	ParentID        *int64          `json:"parent_id"`
	SortOrder       int             `json:"sort_order"`
	IsHiddenForAnon bool            `json:"is_hidden_for_anon"`
	Children        []KbTreeNodeDTO `json:"children"`
}

// NewKbTreeNodeDTOs конвертирует дерево (children всегда массив).
func NewKbTreeNodeDTOs(items []*TreeNode) []KbTreeNodeDTO {
	out := make([]KbTreeNodeDTO, 0, len(items))
	for _, item := range items {
		out = append(out, newKbTreeNodeDTO(item))
	}
	return out
}

// newKbTreeNodeDTO рекурсивно конвертирует узел.
func newKbTreeNodeDTO(item *TreeNode) KbTreeNodeDTO {
	if item == nil {
		return KbTreeNodeDTO{}
	}
	children := make([]KbTreeNodeDTO, 0, len(item.Children))
	for _, child := range item.Children {
		children = append(children, newKbTreeNodeDTO(child))
	}
	return KbTreeNodeDTO{
		ID:              item.ID,
		Title:           item.Title,
		ParentID:        item.ParentID,
		SortOrder:       item.SortOrder,
		IsHiddenForAnon: item.IsHiddenForAnon,
		Children:        children,
	}
}

// KbDeleteDTO — ответ удаления в корзину (контракт KbDeleteResponse): KB-3,
// deleted_descendants — число каскадно удалённых потомков без самого узла.
type KbDeleteDTO struct {
	DeletedID          int64 `json:"deleted_id"`
	DeletedDescendants int   `json:"deleted_descendants"`
}
