/** Тесты чистых функций дерева БЗ (KB-2, KB-6, KB-8). */
import { describe, expect, it } from 'vitest';
import type { KbTreeNode } from '@/api/types';
import {
  countKbNodes,
  filterKbTree,
  findKbNode,
  findKbPath,
  flattenKbTree,
  isKbDescendant,
  isKbMoveNoop,
  kbTreeDepth,
  moveKbTreeNode,
  nextKbSortOrder,
  patchKbTreeNode,
  removeKbTreeNode,
  resolveKbDropTarget,
} from './tree';

function node(
  id: number,
  title: string,
  parentId: number | null,
  sortOrder: number,
  children: KbTreeNode[] = [],
  extra: Partial<KbTreeNode> = {},
): KbTreeNode {
  return {
    id,
    title,
    parent_id: parentId,
    sort_order: sortOrder,
    is_hidden_for_anon: false,
    is_hidden_effective: false,
    has_children: children.length > 0,
    children,
    ...extra,
  };
}

/**
 * Дерево глубины 6 (KB-2: вложенность не ограничена) + второй корень
 * для проверки сортировки и переноса между родителями.
 */
function makeTree(): KbTreeNode[] {
  const six = node(6, 'Рабочий календарь', 5, 10);
  const five = node(5, 'Дедлайн', 4, 10, [six]);
  const four = node(4, 'Поля формы', 3, 10, [five]);
  const three = node(3, 'Как создать заявку', 2, 10, [four]);
  const two = node(2, 'Заявки', 1, 10, [three]);
  const one = node(1, 'Инструкции', null, 10, [two]);
  const nine = node(9, 'Ведомость', 7, 20);
  const eight = node(8, 'Перемещения', 7, 10);
  const seven = node(7, 'Оборудование', null, 20, [eight, nine]);
  return [one, seven];
}

describe('обход дерева', () => {
  it('поддерживает глубину 6 и строит путь до листа', () => {
    const tree = makeTree();
    expect(kbTreeDepth(tree)).toBe(6);
    expect(countKbNodes(tree)).toBe(9);

    const flat = flattenKbTree(tree);
    const leaf = flat.find((entry) => entry.node.id === 6);
    expect(leaf?.depth).toBe(6);
    expect(leaf?.path.map((item) => item.id)).toEqual([1, 2, 3, 4, 5, 6]);
    expect(findKbPath(tree, 6)).toHaveLength(6);
    expect(findKbPath(tree, 999)).toEqual([]);
  });

  it('находит узел и определяет вложенность для проверки цикла', () => {
    const tree = makeTree();
    expect(findKbNode(tree, 5)?.title).toBe('Дедлайн');
    expect(isKbDescendant(tree, 2, 5)).toBe(true);
    expect(isKbDescendant(tree, 2, 1)).toBe(false);
    expect(isKbDescendant(tree, 2, null)).toBe(false);
  });
});

describe('поиск по заголовкам', () => {
  it('оставляет совпавшие узлы и их предков, отмечая совпадения', () => {
    const result = filterKbTree(makeTree(), 'дедлайн');
    expect(result.empty).toBe(false);
    expect([...result.matchIds]).toEqual([5]);
    // Путь до найденного узла сохранён: 1 → 2 → 3 → 4 → 5.
    expect(flattenKbTree(result.tree).map((entry) => entry.node.id)).toEqual([1, 2, 3, 4, 5]);
  });

  it('возвращает пустой результат и исходное дерево при пустом запросе', () => {
    const tree = makeTree();
    expect(filterKbTree(tree, '   ').tree).toBe(tree);
    const empty = filterKbTree(tree, 'нет такого раздела');
    expect(empty.empty).toBe(true);
    expect(empty.tree).toEqual([]);
  });
});

describe('перемещение узлов (KB-6)', () => {
  it('сортирует соседей и переиндексирует sort_order шагом 10', () => {
    const moved = moveKbTreeNode(makeTree(), 9, 7, 8);
    const equipment = findKbNode(moved, 7)!;
    expect(equipment.children.map((child) => child.id)).toEqual([9, 8]);
    expect(equipment.children.map((child) => child.sort_order)).toEqual([10, 20]);
    expect(nextKbSortOrder(equipment.children)).toBe(30);
  });

  it('меняет родителя, сохраняя неизменность исходного дерева', () => {
    const original = makeTree();
    const moved = moveKbTreeNode(original, 8, 3, null);
    expect(findKbNode(moved, 8)?.parent_id).toBe(3);
    // Узел 3 уже имеет потомка 4: перенос в конец даёт sort_order 20.
    expect(findKbNode(moved, 3)?.children.map((child) => child.id)).toEqual([4, 8]);
    expect(findKbNode(moved, 8)?.sort_order).toBe(20);
    // Исходное дерево не мутировано: откат при 409 вернёт его как есть.
    expect(findKbNode(original, 8)?.parent_id).toBe(7);
    expect(findKbNode(moved, 7)?.children.map((child) => child.id)).toEqual([9]);
    expect(findKbNode(moved, 3)?.has_children).toBe(true);
  });

  it('не меняет дерево, если родитель не найден', () => {
    const tree = makeTree();
    expect(moveKbTreeNode(tree, 8, 999, null)).toBe(tree);
  });

  it('не строит заведомо цикличное дерево (перенос внутрь своего потомка)', () => {
    // Цикл ловит сервер (409), но и локальная перестройка не должна
    // создавать «висящий» узел: раздел 2 уносит с собой 3,4,5,6.
    const tree = makeTree();
    expect(moveKbTreeNode(tree, 2, 5, null)).toBe(tree);
  });
});

describe('цель броска и no-op', () => {
  it('бросок на узел — вставка перед ним в его родителя', () => {
    expect(resolveKbDropTarget(makeTree(), 5)).toEqual({ parentId: 4, beforeNodeId: 5 });
  });

  it('бросок на зону child:<id> — внутрь раздела', () => {
    expect(resolveKbDropTarget(makeTree(), 'child:2')).toEqual({ parentId: 2, beforeNodeId: null });
    expect(resolveKbDropTarget(makeTree(), 'child:999')).toBeNull();
    expect(resolveKbDropTarget(makeTree(), 999)).toBeNull();
  });

  it('бросок на root-end — в конец корневого уровня (вынос подраздела наверх)', () => {
    expect(resolveKbDropTarget(makeTree(), 'root-end')).toEqual({
      parentId: null,
      beforeNodeId: null,
    });
    const moved = moveKbTreeNode(makeTree(), 8, null, null);
    expect(findKbNode(moved, 8)?.parent_id).toBeNull();
    // Новый корень — последний: 10, 20, 30.
    expect(findKbNode(moved, 8)?.sort_order).toBe(30);
    expect(moved.map((item) => item.id)).toEqual([1, 7, 8]);
  });

  it('не считает перемещением бросок на себя или сразу за собой', () => {
    const tree = makeTree();
    expect(isKbMoveNoop(tree, 8, { parentId: 7, beforeNodeId: 8 })).toBe(true);
    expect(isKbMoveNoop(tree, 8, { parentId: 7, beforeNodeId: 9 })).toBe(true);
    expect(isKbMoveNoop(tree, 8, { parentId: 7, beforeNodeId: null })).toBe(false);
    expect(isKbMoveNoop(tree, 8, { parentId: 8, beforeNodeId: null })).toBe(true);
  });
});

describe('локальные изменения', () => {
  it('удаляет поддерево и обновляет узел без мутаций', () => {
    const tree = makeTree();
    const removed = removeKbTreeNode(tree, 3);
    expect(findKbNode(removed, 3)).toBeNull();
    expect(findKbNode(removed, 6)).toBeNull();
    expect(findKbNode(removed, 2)?.children).toEqual([]);
    expect(findKbNode(tree, 3)).not.toBeNull();

    const patched = patchKbTreeNode(tree, 2, { title: 'Заявки (новое)', is_hidden_for_anon: true });
    expect(findKbNode(patched, 2)?.title).toBe('Заявки (новое)');
    expect(findKbNode(tree, 2)?.title).toBe('Заявки');
  });
});
