package kb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/graffvishenka/polar-star-service/backend/internal/auth"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform"
	"github.com/graffvishenka/polar-star-service/backend/internal/platform/apierr"
)

// Service — бизнес-логика базы знаний (KB-1..KB-8).
type Service struct {
	repo   Repository
	media  MediaValidator
	clock  platform.Clock
	logger *slog.Logger
}

// Options — зависимости сервиса.
type Options struct {
	Clock  platform.Clock
	Logger *slog.Logger
}

// NewService создаёт сервис БЗ.
func NewService(repo Repository, media MediaValidator, opts Options) *Service {
	if opts.Clock == nil {
		opts.Clock = platform.RealClock{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Service{repo: repo, media: media, clock: opts.Clock, logger: opts.Logger}
}

// Tree отдаёт дерево одним запросом (KB-8): сортировка sort_order, title внутри
// родителя, вложенность не ограничена. public=true — представление для анонима:
// скрытые корневые разделы и всё их поддерево исключаются (KB-7).
func (s *Service) Tree(ctx context.Context, public bool) ([]*TreeNode, error) {
	nodes, err := s.repo.List(ctx)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return buildTree(nodes, public), nil
}

// Node отдаёт узел с хлебными крошками. Для public=true скрытый/удалённый узел
// и потомок скрытого корня неотличимы от несуществующего (404, KB-7).
func (s *Service) Node(ctx context.Context, id int64, public bool) (*Node, []Breadcrumb, error) {
	node, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	if public {
		visible, verr := s.repo.NodePublic(ctx, id)
		if verr != nil {
			return nil, nil, apierr.Internal(verr)
		}
		if !visible {
			return nil, nil, apierr.NotFound("Статья не найдена")
		}
	}
	crumbs, err := s.repo.Ancestors(ctx, id)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	return node, crumbs, nil
}

// Create создаёт узел (KB-3: любой авторизованный). content_html санитизируется
// (KB-4), ссылки на медиа проверяются и помечаются использованными (KB-5).
func (s *Service) Create(ctx context.Context, actor *auth.User, in CreateInput) (*Node, []Breadcrumb, error) {
	title, err := validateTitle(in.Title)
	if err != nil {
		return nil, nil, validation("title", err.Error())
	}
	if err := validateContent(in.ContentHTML); err != nil {
		return nil, nil, validation("content_html", err.Error())
	}
	content := Sanitize(in.ContentHTML)
	mediaIDs, err := s.checkMedia(ctx, content)
	if err != nil {
		return nil, nil, err
	}

	// KB-1: is_hidden_for_anon допустим только у корневых разделов; у потомков
	// наследуется видимость корня, поэтому значение игнорируется.
	hidden := false
	sortOrder := 0
	if in.ParentID != nil {
		parentID := *in.ParentID
		if parentID <= 0 {
			return nil, nil, validation("parent_id", "некорректный идентификатор")
		}
		if err := s.checkParent(ctx, parentID); err != nil {
			return nil, nil, err
		}
	} else {
		hidden = in.IsHiddenForAnon
	}

	if in.SortOrder != nil {
		if *in.SortOrder < 0 {
			return nil, nil, validation("sort_order", "не может быть отрицательным")
		}
		sortOrder = *in.SortOrder
	} else {
		maxOrder, merr := s.repo.MaxSortOrder(ctx, in.ParentID)
		if merr != nil {
			return nil, nil, apierr.Internal(merr)
		}
		sortOrder = maxOrder + SortOrderStep
	}

	node, err := s.repo.Create(ctx, CreateParams{
		Title:           title,
		ContentHTML:     content,
		ParentID:        in.ParentID,
		SortOrder:       sortOrder,
		IsHiddenForAnon: hidden,
		ActorID:         actorID(actor),
	})
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	s.markMedia(ctx, mediaIDs)
	crumbs, err := s.repo.Ancestors(ctx, node.ID)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	return node, crumbs, nil
}

// Update применяет частичные изменения (KB-3). Смена родителя проверяется на
// цикл (409 CYCLE_DETECTED) и на живого родителя (400 для удалённого, KB-2).
func (s *Service) Update(ctx context.Context, actor *auth.User, id int64, in UpdateInput) (*Node, []Breadcrumb, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}

	params := UpdateParams{ID: id, ActorID: actorID(actor), ParentIDSet: in.ParentID.Set}
	var mediaIDs []int64

	if in.Title != nil {
		title, terr := validateTitle(*in.Title)
		if terr != nil {
			return nil, nil, validation("title", terr.Error())
		}
		params.Title = &title
	}
	if in.ContentHTML != nil {
		if cerr := validateContent(*in.ContentHTML); cerr != nil {
			return nil, nil, validation("content_html", cerr.Error())
		}
		content := Sanitize(*in.ContentHTML)
		ids, merr := s.checkMedia(ctx, content)
		if merr != nil {
			return nil, nil, merr
		}
		mediaIDs = ids
		params.ContentHTML = &content
	}
	if in.SortOrder != nil {
		if *in.SortOrder < 0 {
			return nil, nil, validation("sort_order", "не может быть отрицательным")
		}
		params.SortOrder = in.SortOrder
	}

	parentChanged := false
	if in.ParentID.Set {
		if in.ParentID.Value == nil {
			// parent_id: null — перемещение в корень.
			params.ParentID = nil
			parentChanged = existing.ParentID != nil
		} else {
			target := *in.ParentID.Value
			if target <= 0 {
				return nil, nil, validation("parent_id", "некорректный идентификатор")
			}
			if target == id {
				return nil, nil, cycleError()
			}
			if err := s.checkParent(ctx, target); err != nil {
				return nil, nil, err
			}
			ancestor, aerr := s.repo.IsAncestorOrSelf(ctx, id, target)
			if aerr != nil {
				return nil, nil, apierr.Internal(aerr)
			}
			if ancestor {
				return nil, nil, cycleError()
			}
			parentChanged = existing.ParentID == nil || *existing.ParentID != target
			params.ParentID = &target
		}
	}
	// При смене родителя без явного sort_order узел встаёт в конец списка детей.
	if parentChanged && in.SortOrder == nil {
		maxOrder, merr := s.repo.MaxSortOrder(ctx, params.ParentID)
		if merr != nil {
			return nil, nil, apierr.Internal(merr)
		}
		next := maxOrder + SortOrderStep
		params.SortOrder = &next
	}

	updated, err := s.repo.Update(ctx, params)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	s.markMedia(ctx, mediaIDs)
	crumbs, err := s.repo.Ancestors(ctx, updated.ID)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	return updated, crumbs, nil
}

// Move перемещает узел (KB-6, drag-n-drop): новый родитель, позиция «перед
// узлом» и переиндексация sort_order с шагом 10. Цикл → 409 CYCLE_DETECTED.
func (s *Service) Move(ctx context.Context, actor *auth.User, id int64, in MoveInput) (*Node, []Breadcrumb, error) {
	if !in.ParentID.Set {
		return nil, nil, validation("parent_id", "обязательное поле (null — перемещение в корень)")
	}
	if _, err := s.repo.Get(ctx, id); err != nil {
		return nil, nil, mapRepoError(err)
	}

	var parentID *int64
	if in.ParentID.Value != nil {
		target := *in.ParentID.Value
		if target <= 0 {
			return nil, nil, validation("parent_id", "некорректный идентификатор")
		}
		if target == id {
			return nil, nil, cycleError()
		}
		if err := s.checkParent(ctx, target); err != nil {
			return nil, nil, err
		}
		ancestor, err := s.repo.IsAncestorOrSelf(ctx, id, target)
		if err != nil {
			return nil, nil, apierr.Internal(err)
		}
		if ancestor {
			return nil, nil, cycleError()
		}
		parentID = &target
	}
	if in.BeforeNodeID != nil {
		if *in.BeforeNodeID <= 0 {
			return nil, nil, validation("before_node_id", "некорректный идентификатор")
		}
		if *in.BeforeNodeID == id {
			return nil, nil, validation("before_node_id", "нельзя встать перед самим собой")
		}
	}

	if err := s.repo.Move(ctx, id, parentID, in.BeforeNodeID, actorID(actor)); err != nil {
		return nil, nil, mapMoveError(err)
	}
	node, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	crumbs, err := s.repo.Ancestors(ctx, id)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	return node, crumbs, nil
}

// Delete отправляет узел с поддеревом в корзину (KB-3, только ADMIN — проверка
// маршрута) и возвращает число удалённых потомков.
// Delete отправляет узел с поддеревом в корзину (KB-3, TRASH-1) и фиксирует
// автора удаления для списка корзины. Восстановление — блок Б10.
func (s *Service) Delete(ctx context.Context, actor *auth.User, id int64) (int, error) {
	descendants, err := s.repo.SoftDelete(ctx, id, s.clock.Now().UTC(), actorID(actor))
	if err != nil {
		return 0, mapRepoError(err)
	}
	s.logger.Info("kb: узел отправлен в корзину", "node_id", id, "descendants", descendants)
	return descendants, nil
}

// SetVisibility переключает видимость корневого раздела (KB-7, только ADMIN).
func (s *Service) SetVisibility(ctx context.Context, actor *auth.User, id int64, hidden bool) (*Node, []Breadcrumb, error) {
	node, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	if node.ParentID != nil {
		return nil, nil, apierr.Validation("Скрывать можно только корневые разделы",
			map[string]any{"is_hidden_for_anon": "допустимо только для корня"})
	}
	updated, err := s.repo.SetVisibility(ctx, id, hidden, actorID(actor))
	if err != nil {
		return nil, nil, mapRepoError(err)
	}
	crumbs, err := s.repo.Ancestors(ctx, id)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}
	return updated, crumbs, nil
}

// PublicKBFile реализует files.KBMediaAuthorizer: сообщает, что файл scope=kb
// вставлен в content_html хотя бы одного публичного (корень не скрыт, ничего не
// удалено) узла. Используется публичной выдачей `GET /api/public/files/{id}`
// без токена; проверку scope=kb делает files.Service.
func (s *Service) PublicKBFile(ctx context.Context, fileID int64) (bool, error) {
	return s.publicMediaReferenced(ctx, fileID)
}

// publicMediaReferenced ищет публичный узел, ссылающийся на файл. Отбор в SQL
// широкий (LIKE по подстроке), точное совпадение id проверяется в Go — иначе
// `/api/files/1` совпадал бы с `/api/files/12`.
func (s *Service) publicMediaReferenced(ctx context.Context, fileID int64) (bool, error) {
	pattern := "%/api/files/" + strconv.FormatInt(fileID, 10) + "%"
	candidates, err := s.repo.MediaCandidates(ctx, pattern)
	if err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		if !ContentReferencesFile(candidate.ContentHTML, fileID) {
			continue
		}
		visible, verr := s.repo.NodePublic(ctx, candidate.ID)
		if verr != nil {
			return false, verr
		}
		if visible {
			return true, nil
		}
	}
	return false, nil
}

// checkParent проверяет родителя для создания/перемещения (KB-2).
func (s *Service) checkParent(ctx context.Context, parentID int64) error {
	exists, deleted, err := s.repo.ParentStatus(ctx, parentID)
	if err != nil {
		return apierr.Internal(err)
	}
	if !exists {
		return apierr.NotFound("Родительский узел не найден")
	}
	if deleted {
		return apierr.BadRequest(apierr.CodeValidation, "Родительский узел удалён в корзину")
	}
	return nil
}

// checkMedia санитизированного HTML: извлекает ссылки и проверяет каждую
// (KB-5). Возвращает id для отметки used.
func (s *Service) checkMedia(ctx context.Context, contentHTML string) ([]int64, error) {
	ids, overflow := ExtractMediaIDs(contentHTML)
	if overflow {
		return nil, apierr.Validation("Слишком много ссылок на медиа в статье",
			map[string]any{"content_html": fmt.Sprintf("максимум %d уникальных файлов", maxMediaRefs)})
	}
	for _, id := range ids {
		if err := s.media.ValidateMedia(ctx, id); err != nil {
			if errors.Is(err, ErrMediaNotFound) {
				return nil, apierr.New(400, apierr.CodeInvalidMediaRef,
					"Ссылка на медиа недопустима: файл не найден или не относится к базе знаний").
					WithDetails(map[string]any{"url": "/api/files/" + strconv.FormatInt(id, 10)})
			}
			return nil, apierr.Internal(err)
		}
	}
	return ids, nil
}

// markMedia помечает файлы использованными. Ошибка не отменяет сохранение
// статьи: медиа БЗ уже помечено при загрузке (Б3), отметка здесь — подстраховка
// от удаления фоновой очисткой orphan.
func (s *Service) markMedia(ctx context.Context, ids []int64) {
	if len(ids) == 0 {
		return
	}
	if err := s.media.MarkLinked(ctx, ids); err != nil {
		s.logger.Warn("kb: не удалось отметить медиа использованным",
			"file_ids", ids, "error", err)
	}
}

// buildTree собирает дерево из плоского списка узлов.
//
// public=true (KB-7): скрытые корневые разделы отбрасываются вместе с
// поддеревом; узлы, чей родитель отсутствует (данные в корзине), не
// показываются вовсе, чтобы не вынести скрытую ветку в корень.
func buildTree(nodes []Node, public bool) []*TreeNode {
	byID := make(map[int64]*TreeNode, len(nodes))
	ordered := make([]*TreeNode, 0, len(nodes))
	for i := range nodes {
		node := nodes[i]
		item := &TreeNode{Node: node}
		byID[node.ID] = item
		ordered = append(ordered, item)
	}

	roots := make([]*TreeNode, 0, 8)
	for _, item := range ordered {
		if item.ParentID == nil {
			roots = append(roots, item)
			continue
		}
		parent, ok := byID[*item.ParentID]
		if ok {
			parent.Children = append(parent.Children, item)
			continue
		}
		if !public {
			// Авторизованный видит «осиротевший» узел в корне, чтобы данные
			// можно было починить.
			roots = append(roots, item)
		}
	}

	if public {
		visible := make([]*TreeNode, 0, len(roots))
		for _, root := range roots {
			if !root.IsHiddenForAnon {
				visible = append(visible, root)
			}
		}
		roots = visible
	}
	sortTree(roots)
	return roots
}

// sortTree сортирует узлы по sort_order, title, id на каждом уровне (KB-8).
func sortTree(items []*TreeNode) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.SortOrder != b.SortOrder {
			return a.SortOrder < b.SortOrder
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.ID < b.ID
	})
	for _, item := range items {
		sortTree(item.Children)
	}
}

// actorID возвращает id автора правки (0, если пользователь не передан —
// маршруты БЗ всегда защищены аутентификацией).
func actorID(actor *auth.User) int64 {
	if actor == nil {
		return 0
	}
	return actor.ID
}

// validateTitle нормализует и проверяет заголовок (KB-1: 1..200).
func validateTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	n := len([]rune(title))
	if n < 1 || n > MaxTitleRunes {
		return "", fmt.Errorf("заголовок должен содержать от 1 до %d символов", MaxTitleRunes)
	}
	return title, nil
}

// validateContent проверяет длину content_html до санитизации (KB-1).
func validateContent(contentHTML string) error {
	if n := len([]rune(contentHTML)); n > MaxContentRunes {
		return fmt.Errorf("содержимое статьи не должно превышать %d символов", MaxContentRunes)
	}
	return nil
}

// validation — 422 с указанием поля.
func validation(field, message string) *apierr.Error {
	return apierr.Validation("Некорректные данные статьи",
		map[string]any{field: message})
}

// cycleError — 409 CYCLE_DETECTED (KB-2).
func cycleError() *apierr.Error {
	return apierr.Conflict(apierr.CodeCycleDetected,
		"Нельзя переместить раздел в собственное поддерево")
}

// mapRepoError переводит ошибки репозитория в коды контракта.
func mapRepoError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apierr.NotFound("Статья не найдена")
	case errors.Is(err, ErrParentNotFound):
		return apierr.NotFound("Родительский узел не найден")
	case errors.Is(err, ErrParentDeleted):
		return apierr.BadRequest(apierr.CodeValidation, "Родительский узел удалён в корзину")
	case errors.Is(err, ErrCycle):
		return cycleError()
	case errors.Is(err, ErrVisibilityRootOnly):
		return apierr.Validation("Скрывать можно только корневые разделы", nil)
	default:
		return apierr.Internal(err)
	}
}

// mapMoveError переводит ошибки перемещения в коды контракта (KB-2, KB-6).
func mapMoveError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrParentNotFound):
		return apierr.NotFound("Статья не найдена")
	case errors.Is(err, ErrParentDeleted):
		return apierr.BadRequest(apierr.CodeValidation, "Родительский узел удалён в корзину")
	case errors.Is(err, ErrCycle):
		return cycleError()
	case errors.Is(err, ErrBeforeNotFound):
		return apierr.Validation("Узел before_node_id не найден среди соседей нового родителя",
			map[string]any{"before_node_id": "не является соседом"})
	default:
		return apierr.Internal(err)
	}
}
