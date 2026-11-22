package kb

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/graffvishenka/polar-star-service/backend/internal/files"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
)

// baseTime — фиксированный момент времени для детерминированных тестов.
var baseTime = time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)

// fakeRepo — реализация Repository в памяти: сервис и handler'ы БЗ
// тестируются без MySQL (в окружении разработки БД нет, docs/ENVIRONMENT.md).
// Поведение (каскадное удаление, переиндексация, предки) зеркалит SQL-версию.
type fakeRepo struct {
	mu            sync.Mutex
	nextID        int64
	nodes         map[int64]*Node
	lastDeletedBy int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{nodes: make(map[int64]*Node)}
}

// seed добавляет узел напрямую (для подготовки дерева в тестах).
func (r *fakeRepo) seed(node Node) *Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	if node.ID == 0 {
		r.nextID++
		node.ID = r.nextID
	}
	if node.ID > r.nextID {
		r.nextID = node.ID
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = baseTime
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = baseTime
	}
	copied := node
	r.nodes[node.ID] = &copied
	return &copied
}

// liveNodeLocked возвращает живой узел (вызывается под мьютексом).
func (r *fakeRepo) liveNodeLocked(id int64) *Node {
	node, ok := r.nodes[id]
	if !ok || node.DeletedAt != nil {
		return nil
	}
	return node
}

func (r *fakeRepo) List(_ context.Context) ([]Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		if node.DeletedAt == nil {
			out = append(out, *node)
		}
	}
	return out, nil
}

func (r *fakeRepo) Get(_ context.Context, id int64) (*Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node := r.liveNodeLocked(id)
	if node == nil {
		return nil, ErrNotFound
	}
	copied := *node
	return &copied, nil
}

func (r *fakeRepo) ParentStatus(_ context.Context, id int64) (bool, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[id]
	if !ok {
		return false, false, nil
	}
	return true, node.DeletedAt != nil, nil
}

func (r *fakeRepo) Create(_ context.Context, params CreateParams) (*Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	node := &Node{
		ID:              r.nextID,
		Title:           params.Title,
		ContentHTML:     params.ContentHTML,
		ParentID:        params.ParentID,
		SortOrder:       params.SortOrder,
		IsHiddenForAnon: params.IsHiddenForAnon,
		CreatedAt:       baseTime,
		UpdatedAt:       baseTime,
	}
	if params.ActorID > 0 {
		actor := params.ActorID
		node.CreatedBy = &actor
		node.UpdatedBy = &actor
	}
	r.nodes[node.ID] = node
	copied := *node
	return &copied, nil
}

func (r *fakeRepo) Update(_ context.Context, params UpdateParams) (*Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node := r.liveNodeLocked(params.ID)
	if node == nil {
		return nil, ErrNotFound
	}
	if params.Title != nil {
		node.Title = *params.Title
	}
	if params.ContentHTML != nil {
		node.ContentHTML = *params.ContentHTML
	}
	if params.ParentIDSet {
		node.ParentID = params.ParentID
	}
	if params.SortOrder != nil {
		node.SortOrder = *params.SortOrder
	}
	node.UpdatedAt = baseTime
	if params.ActorID > 0 {
		actor := params.ActorID
		node.UpdatedBy = &actor
	}
	copied := *node
	return &copied, nil
}

func (r *fakeRepo) Move(_ context.Context, nodeID int64, parentID *int64, beforeNodeID *int64, actorID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	node := r.liveNodeLocked(nodeID)
	if node == nil {
		return ErrNotFound
	}
	if parentID != nil {
		parent, ok := r.nodes[*parentID]
		if !ok {
			return ErrParentNotFound
		}
		if parent.DeletedAt != nil {
			return ErrParentDeleted
		}
		if r.isAncestorOrSelfLocked(nodeID, *parentID) {
			return ErrCycle
		}
	}

	siblings := make([]sibling, 0, 8)
	for _, candidate := range r.nodes {
		if candidate.DeletedAt != nil || candidate.ID == nodeID {
			continue
		}
		if sameParent(candidate.ParentID, parentID) {
			siblings = append(siblings, sibling{ID: candidate.ID, SortOrder: candidate.SortOrder, Title: candidate.Title})
		}
	}
	ordered, err := orderWithMove(siblings, nodeID, beforeNodeID)
	if err != nil {
		return err
	}
	for index, id := range ordered {
		r.nodes[id].SortOrder = sortOrderAt(index)
	}
	node.ParentID = parentID
	node.UpdatedAt = baseTime
	if actorID > 0 {
		actor := actorID
		node.UpdatedBy = &actor
	}
	return nil
}

func (r *fakeRepo) SoftDelete(_ context.Context, id int64, at time.Time, deletedBy int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastDeletedBy = deletedBy
	node := r.liveNodeLocked(id)
	if node == nil {
		return 0, ErrNotFound
	}
	subtree := r.subtreeLocked(id)
	for _, member := range subtree {
		deletedAt := at
		r.nodes[member].DeletedAt = &deletedAt
		r.nodes[member].UpdatedAt = at
	}
	if len(subtree) > 0 {
		return len(subtree) - 1, nil
	}
	return 0, nil
}

func (r *fakeRepo) SetVisibility(_ context.Context, id int64, hidden bool, actorID int64) (*Node, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node := r.liveNodeLocked(id)
	if node == nil {
		return nil, ErrNotFound
	}
	node.IsHiddenForAnon = hidden
	node.UpdatedAt = baseTime
	if actorID > 0 {
		actor := actorID
		node.UpdatedBy = &actor
	}
	copied := *node
	return &copied, nil
}

func (r *fakeRepo) Ancestors(_ context.Context, id int64) ([]Breadcrumb, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[id]
	if !ok {
		return nil, ErrNotFound
	}
	chain := make([]Breadcrumb, 0, 8)
	seen := make(map[int64]struct{}, 8)
	for current := node; current != nil; {
		if _, ok := seen[current.ID]; ok {
			break
		}
		seen[current.ID] = struct{}{}
		chain = append(chain, Breadcrumb{ID: current.ID, Title: current.Title})
		if current.ParentID == nil {
			break
		}
		parent, ok := r.nodes[*current.ParentID]
		if !ok {
			break
		}
		current = parent
	}
	// Разворачиваем: от корня до узла.
	for left, right := 0, len(chain)-1; left < right; left, right = left+1, right-1 {
		chain[left], chain[right] = chain[right], chain[left]
	}
	return chain, nil
}

func (r *fakeRepo) IsAncestorOrSelf(_ context.Context, candidate, nodeID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isAncestorOrSelfLocked(candidate, nodeID), nil
}

func (r *fakeRepo) MaxSortOrder(_ context.Context, parentID *int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	maxOrder := 0
	for _, node := range r.nodes {
		if node.DeletedAt != nil || !sameParent(node.ParentID, parentID) {
			continue
		}
		if node.SortOrder > maxOrder {
			maxOrder = node.SortOrder
		}
	}
	return maxOrder, nil
}

func (r *fakeRepo) MediaCandidates(_ context.Context, like string) ([]MediaCandidate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	needle := strings.Trim(like, "%")
	out := make([]MediaCandidate, 0, 4)
	for _, node := range r.nodes {
		if node.DeletedAt != nil || !strings.Contains(node.ContentHTML, needle) {
			continue
		}
		out = append(out, MediaCandidate{ID: node.ID, ContentHTML: node.ContentHTML})
	}
	return out, nil
}

func (r *fakeRepo) NodePublic(_ context.Context, id int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[id]
	if !ok {
		return false, nil
	}
	seen := make(map[int64]struct{}, 8)
	for current := node; current != nil; {
		if _, ok := seen[current.ID]; ok {
			return false, nil
		}
		seen[current.ID] = struct{}{}
		if current.DeletedAt != nil {
			return false, nil
		}
		if current.ParentID == nil {
			return !current.IsHiddenForAnon, nil
		}
		parent, ok := r.nodes[*current.ParentID]
		if !ok {
			return false, nil
		}
		current = parent
	}
	return false, nil
}

// isAncestorOrSelfLocked: candidate — предок nodeID или сам узел.
func (r *fakeRepo) isAncestorOrSelfLocked(candidate, nodeID int64) bool {
	seen := make(map[int64]struct{}, 8)
	for current, ok := r.nodes[nodeID]; ok && current != nil; {
		if current.ID == candidate {
			return true
		}
		if _, visited := seen[current.ID]; visited {
			return false
		}
		seen[current.ID] = struct{}{}
		if current.ParentID == nil {
			return false
		}
		current, ok = r.nodes[*current.ParentID]
	}
	return false
}

// subtreeLocked возвращает id узла и всех потомков (включая удалённых).
func (r *fakeRepo) subtreeLocked(id int64) []int64 {
	out := []int64{id}
	for index := 0; index < len(out); index++ {
		for _, node := range r.nodes {
			if node.ParentID != nil && *node.ParentID == out[index] {
				out = append(out, node.ID)
			}
		}
	}
	return out
}

// sameParent сравнивает nullable parent_id.
func sameParent(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// fakeMedia — заглушка MediaValidator: файлы с областью scope и содержимым.
type fakeMedia struct {
	scopes      map[int64]files.Scope
	content     map[int64][]byte
	linked      []int64
	validateErr error
}

func newFakeMedia() *fakeMedia {
	return &fakeMedia{scopes: make(map[int64]files.Scope), content: make(map[int64][]byte)}
}

func (m *fakeMedia) ValidateMedia(_ context.Context, fileID int64) error {
	if m.validateErr != nil {
		return m.validateErr
	}
	scope, ok := m.scopes[fileID]
	if !ok || scope != files.ScopeKB {
		return ErrMediaNotFound
	}
	return nil
}

func (m *fakeMedia) MarkLinked(_ context.Context, fileIDs []int64) error {
	m.linked = append(m.linked, fileIDs...)
	return nil
}

// newTestService собирает сервис с фейковыми репозиторием и медиа.
func newTestService(t *testing.T) (*Service, *fakeRepo, *fakeMedia) {
	t.Helper()
	repo := newFakeRepo()
	media := newFakeMedia()
	svc := NewService(repo, media, Options{
		Clock:  platform.FixedClock{Time: baseTime},
		Logger: discardLogger(),
	})
	return svc, repo, media
}

// discardLogger — логгер, не засоряющий вывод тестов.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}
