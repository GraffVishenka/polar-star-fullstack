/**
 * Работа с деревом БЗ на клиенте (KB-2, KB-6, KB-8).
 *
 * Дерево приходит одним запросом `GET /api/kb/tree` и уже отсортировано
 * сервером (`sort_order, title`, KB-8). Здесь — чистые функции без React:
 * их используют дерево, страница статьи и оптимистичное обновление при DnD.
 * Вложенность не ограничена (KB-2) — все обходы рекурсивные и без лимита
 * глубины, поэтому дерево на 5+ уровней работает так же, как на одном.
 */
import type { KbTreeNode } from '@/api/types';

/** Плоское представление узла: удобно для поиска и хлебных крошек. */
export interface FlatKbNode {
  node: KbTreeNode;
  /** Родитель в дереве (`null` у корня). */
  parentId: number | null;
  /** Глубина: корень = 1. */
  depth: number;
  /** Путь от корня до узла включительно (для хлебных крошек). */
  path: KbTreeNode[];
}

/** Обход дерева «в глубину» слева направо. */
export function flattenKbTree(nodes: KbTreeNode[]): FlatKbNode[] {
  const result: FlatKbNode[] = [];
  const walk = (current: KbTreeNode[], parentId: number | null, depth: number, path: KbTreeNode[]) => {
    for (const node of current) {
      const nodePath = [...path, node];
      result.push({ node, parentId, depth, path: nodePath });
      if (node.children.length) walk(node.children, node.id, depth + 1, nodePath);
    }
  };
  walk(nodes, null, 1, []);
  return result;
}

/** Поиск узла по идентификатору (без учёта скрытости — это делает сервер). */
export function findKbNode(nodes: KbTreeNode[], id: number | null | undefined): KbTreeNode | null {
  if (id === null || id === undefined) return null;
  for (const node of nodes) {
    if (node.id === id) return node;
    const found = findKbNode(node.children, id);
    if (found) return found;
  }
  return null;
}

/** Путь от корня до узла включительно; пустой массив, если узла нет. */
export function findKbPath(nodes: KbTreeNode[], id: number | null | undefined): KbTreeNode[] {
  if (id === null || id === undefined) return [];
  return flattenKbTree(nodes).find((entry) => entry.node.id === id)?.path ?? [];
}

/** Максимальная глубина дерева (для тестов KB-2: должно быть ≥ 5). */
export function kbTreeDepth(nodes: KbTreeNode[]): number {
  if (!nodes.length) return 0;
  return nodes.reduce((max, node) => Math.max(max, 1 + kbTreeDepth(node.children)), 0);
}

/** Все идентификаторы поддерева, включая сам узел. */
export function collectKbSubtreeIds(node: KbTreeNode): number[] {
  const ids = [node.id];
  for (const child of node.children) ids.push(...collectKbSubtreeIds(child));
  return ids;
}

/**
 * Проверка цикла KB-2: `candidateParentId` лежит внутри поддерева `nodeId`.
 * Клиент использует её, чтобы не показывать заведомо неверную подсказку,
 * а окончательное решение всё равно за сервером (`409 CYCLE_DETECTED`).
 */
export function isKbDescendant(
  nodes: KbTreeNode[],
  nodeId: number,
  candidateParentId: number | null,
): boolean {
  if (candidateParentId === null) return false;
  const node = findKbNode(nodes, nodeId);
  if (!node) return false;
  return collectKbSubtreeIds(node).includes(candidateParentId);
}

/** Обновление узла в дереве без мутации исходного массива. */
export function patchKbTreeNode(
  nodes: KbTreeNode[],
  id: number,
  patch: Partial<Pick<KbTreeNode, 'title' | 'is_hidden_for_anon'>>,
): KbTreeNode[] {
  return nodes.map((node) => {
    if (node.id === id) return { ...node, ...patch };
    if (!node.children.length) return node;
    return { ...node, children: patchKbTreeNode(node.children, id, patch) };
  });
}

/** Удаление поддерева (оптимистичное обновление после KB-3). */
export function removeKbTreeNode(nodes: KbTreeNode[], id: number): KbTreeNode[] {
  return nodes
    .filter((node) => node.id !== id)
    .map((node) =>
      node.children.length ? { ...node, children: removeKbTreeNode(node.children, id) } : node,
    );
}

/**
 * Перестройка дерева после перетаскивания (KB-6).
 *
 * Переносит `nodeId` в `parentId` перед `beforeNodeId` (или в конец, если не
 * задан). Возвращает новое дерево с пересчитанными `sort_order` (шаг 10, как
 * на сервере) и `has_children`: оптимистичное состояние совпадает с ответом
 * API, поэтому после refetch «не дёргается».
 */
export function moveKbTreeNode(
  nodes: KbTreeNode[],
  nodeId: number,
  parentId: number | null,
  beforeNodeId: number | null,
): KbTreeNode[] {
  const without = removeKbTreeNode(nodes, nodeId);
  const moving = findKbNode(nodes, nodeId);
  if (!moving) return nodes;

  const insert = (list: KbTreeNode[], at: number): KbTreeNode[] => {
    const entry: KbTreeNode = {
      ...moving,
      parent_id: parentId,
      is_hidden_for_anon: parentId === null ? moving.is_hidden_for_anon : false,
    };
    const next = [...list];
    next.splice(at, 0, entry);
    return next;
  };

  const rebuild = (list: KbTreeNode[]): KbTreeNode[] => {
    return list.map((node, index) => {
      const children = rebuild(node.children);
      return {
        ...node,
        sort_order: (index + 1) * 10,
        has_children: children.length > 0,
        children,
      };
    });
  };

  if (parentId === null) {
    const beforeIndex =
      beforeNodeId === null ? -1 : without.findIndex((node) => node.id === beforeNodeId);
    const at = beforeIndex === -1 ? without.length : beforeIndex;
    return rebuild(insert(without, at));
  }

  const parent = findKbNode(without, parentId);
  if (!parent) return nodes;

  const withChild = (list: KbTreeNode[]): KbTreeNode[] =>
    list.map((node) => {
      if (node.id !== parentId) {
        return node.children.length ? { ...node, children: withChild(node.children) } : node;
      }
      const siblings = node.children;
      const beforeIndex =
        beforeNodeId === null ? -1 : siblings.findIndex((child) => child.id === beforeNodeId);
      const at = beforeIndex === -1 ? siblings.length : beforeIndex;
      return { ...node, children: insert(siblings, at), has_children: true };
    });

  return rebuild(withChild(without));
}

export interface KbDropTarget {
  parentId: number | null;
  beforeNodeId: number | null;
}

/**
 * Куда положить узел при отпускании: `overId` — id строки дерева
 * («перед этим узлом, в его родителя»), `child:<id>` (внутрь раздела)
 * или `root-end` (в конец корневого уровня — чтобы вынести подраздел наверх).
 */
export function resolveKbDropTarget(
  nodes: KbTreeNode[],
  overId: string | number,
): KbDropTarget | null {
  const raw = String(overId);
  if (raw === 'root-end') return { parentId: null, beforeNodeId: null };
  if (raw.startsWith('child:')) {
    const parentId = Number(raw.slice('child:'.length));
    if (!Number.isSafeInteger(parentId) || !findKbNode(nodes, parentId)) return null;
    return { parentId, beforeNodeId: null };
  }
  const overNodeId = Number(raw);
  if (!Number.isSafeInteger(overNodeId)) return null;
  const overNode = findKbNode(nodes, overNodeId);
  if (!overNode) return null;
  return { parentId: overNode.parent_id, beforeNodeId: overNode.id };
}

/** Перемещение не изменит порядок (бросили на себя или сразу за собой). */
export function isKbMoveNoop(
  nodes: KbTreeNode[],
  nodeId: number,
  target: KbDropTarget,
): boolean {
  const node = findKbNode(nodes, nodeId);
  if (!node) return true;
  if (target.parentId === nodeId) return true;
  if (target.parentId !== node.parent_id) return false;
  if (target.beforeNodeId === nodeId) return true;
  const siblings = (target.parentId === null ? nodes : (findKbNode(nodes, target.parentId)?.children ?? []));
  const currentIndex = siblings.findIndex((item) => item.id === nodeId);
  const nextId = currentIndex >= 0 ? (siblings[currentIndex + 1]?.id ?? null) : null;
  return target.beforeNodeId === nextId;
}

export interface KbSearchResult {
  tree: KbTreeNode[];
  /** Узлы, чей заголовок совпал с запросом (для подсветки). */
  matchIds: Set<number>;
  /** Совпадений не найдено. */
  empty: boolean;
}

/**
 * Поиск по заголовкам (запрос без учёта регистра). Родители совпавших узлов
 * сохраняются, чтобы путь до результата оставался видимым.
 */
export function filterKbTree(nodes: KbTreeNode[], query: string): KbSearchResult {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return { tree: nodes, matchIds: new Set(), empty: false };

  const matchIds = new Set<number>();
  const filter = (list: KbTreeNode[]): KbTreeNode[] =>
    list.reduce<KbTreeNode[]>((acc, node) => {
      const children = filter(node.children);
      const matched = node.title.toLowerCase().includes(normalized);
      if (matched) matchIds.add(node.id);
      if (matched || children.length) acc.push({ ...node, children });
      return acc;
    }, []);

  const tree = filter(nodes);
  return { tree, matchIds, empty: tree.length === 0 };
}

/** Количество узлов дерева (для подписей «N разделов»). */
export function countKbNodes(nodes: KbTreeNode[]): number {
  return nodes.reduce((total, node) => total + 1 + countKbNodes(node.children), 0);
}

/** Следующий sort_order в наборе соседей (шаг 10, KB-6). */
export function nextKbSortOrder(siblings: KbTreeNode[]): number {
  return siblings.length ? Math.max(...siblings.map((node) => node.sort_order)) + 10 : 10;
}
