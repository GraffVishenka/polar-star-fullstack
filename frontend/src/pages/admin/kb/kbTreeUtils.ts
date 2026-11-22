/**
 * Разбор дерева БЗ для раздела модерации (§3.5 KB-1, KB-3, KB-7).
 * Полноценный редактор делает fe-2 в `pages/kb`, здесь — простая таблица-дерево.
 */
import type { KbTreeNode } from '@/api/types';

export interface KbAdminRow {
  id: number;
  title: string;
  parent_id: number | null;
  isRoot: boolean;
  /** Тумблер доступен только у корневых разделов (KB-1). */
  is_hidden_for_anon: boolean;
  /** Итоговая видимость с учётом наследования (KB-7). */
  is_hidden_effective: boolean;
  /** Число потомков: их удалит каскад, но они не восстановятся (KB-3/TRASH-5). */
  descendants: number;
  children: KbAdminRow[];
}

/** Количество потомков любого уровня (не считая сам узел). */
export function countDescendants(node: KbTreeNode): number {
  return node.children.reduce((total, child) => total + 1 + countDescendants(child), 0);
}

/** Дерево API → строки antd Table (дети идут в `children`). */
export function toKbAdminRows(nodes: KbTreeNode[], parentId: number | null = null): KbAdminRow[] {
  return nodes.map((node) => ({
    id: node.id,
    title: node.title,
    parent_id: node.parent_id ?? parentId,
    isRoot: (node.parent_id ?? parentId) === null,
    is_hidden_for_anon: node.is_hidden_for_anon,
    is_hidden_effective: node.is_hidden_effective,
    descendants: countDescendants(node),
    children: toKbAdminRows(node.children ?? [], node.id),
  }));
}

/** Плоский список всех строк — для поиска выбранного узла по id. */
export function flattenKbRows(rows: KbAdminRow[]): KbAdminRow[] {
  return rows.flatMap((row) => [row, ...flattenKbRows(row.children)]);
}
