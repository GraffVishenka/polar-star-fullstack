/**
 * TreeNav — дерево навигации произвольной вложенности (KB-2: глубина ≥ 5).
 *
 * Компонент рекурсивный и не ограничивает глубину: каждый уровень — свой
 * `<li role="treeitem">` с `data-depth`, поэтому вложенность проверяется тестом.
 * Скрытые для анонимов разделы помечаются замком (KB-7).
 */
import { useMemo, useState, type ReactNode } from 'react';
import { Button, Empty, Flex, Space, Tooltip, Typography } from 'antd';
import {
  CaretDownOutlined,
  CaretRightOutlined,
  FileTextOutlined,
  LockOutlined,
} from '@ant-design/icons';

export interface TreeNavItem {
  id: number;
  title: string;
  children?: TreeNavItem[];
  /** Итоговая скрытость раздела (наследуется от корня, KB-1/KB-7). */
  isHidden?: boolean;
}

export interface TreeNavProps {
  nodes: TreeNavItem[];
  selectedId?: number | null;
  onSelect?: (item: TreeNavItem) => void;
  /** Автораскрытие уровней: 1 — только корни. */
  defaultExpandDepth?: number;
  /** Показать кнопки «Развернуть/Свернуть всё». */
  showExpandControls?: boolean;
  emptyText?: string;
  ariaLabel?: string;
  /** Дополнительное действие справа от узла (например, тумблер видимости). */
  renderSuffix?: (item: TreeNavItem) => ReactNode;
}

function collectIds(
  nodes: TreeNavItem[],
  depth: number,
  maxDepth: number,
  acc: Set<number>,
): Set<number> {
  for (const node of nodes) {
    if (depth < maxDepth && node.children?.length) acc.add(node.id);
    if (node.children?.length) collectIds(node.children, depth + 1, maxDepth, acc);
  }
  return acc;
}

function allIds(nodes: TreeNavItem[], acc: Set<number> = new Set()): Set<number> {
  for (const node of nodes) {
    if (node.children?.length) {
      acc.add(node.id);
      allIds(node.children, acc);
    }
  }
  return acc;
}

interface TreeItemProps {
  item: TreeNavItem;
  depth: number;
  expanded: Set<number>;
  selectedId?: number | null;
  onToggle: (id: number) => void;
  onSelect?: (item: TreeNavItem) => void;
  renderSuffix?: (item: TreeNavItem) => ReactNode;
}

function TreeItem({
  item,
  depth,
  expanded,
  selectedId,
  onToggle,
  onSelect,
  renderSuffix,
}: TreeItemProps) {
  const hasChildren = Boolean(item.children?.length);
  const isOpen = expanded.has(item.id);
  const isSelected = selectedId === item.id;

  return (
    <li
      role="treeitem"
      aria-expanded={hasChildren ? isOpen : undefined}
      aria-selected={isSelected}
      data-depth={depth}
      data-node-id={item.id}
      data-testid={`tree-node-${item.id}`}
    >
      <div
        className="tree-nav__row"
        style={{ paddingInlineStart: (depth - 1) * 16 + 4 }}
        onClick={() => onSelect?.(item)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            onSelect?.(item);
          }
        }}
        tabIndex={0}
        aria-current={isSelected ? 'page' : undefined}
      >
        {hasChildren ? (
          <Button
            type="text"
            size="small"
            aria-label={isOpen ? 'Свернуть раздел' : 'Развернуть раздел'}
            data-testid={`tree-toggle-${item.id}`}
            icon={isOpen ? <CaretDownOutlined /> : <CaretRightOutlined />}
            onClick={(event) => {
              event.stopPropagation();
              onToggle(item.id);
            }}
            style={{ padding: 0, width: 18, height: 18, minWidth: 18 }}
          />
        ) : (
          <span style={{ display: 'inline-block', width: 18, flex: '0 0 18px' }} />
        )}
        <FileTextOutlined style={{ color: isSelected ? '#1677ff' : '#8c8c8c', flex: '0 0 auto' }} />
        <Typography.Text
          ellipsis
          strong={isSelected}
          style={{ flex: 1, minWidth: 0, fontSize: depth > 1 ? 13 : 14 }}
        >
          {item.title}
        </Typography.Text>
        {item.isHidden ? (
          <Tooltip title="Скрыт для анонимных посетителей">
            <LockOutlined style={{ color: '#8c8c8c' }} data-testid="tree-lock" />
          </Tooltip>
        ) : null}
        {renderSuffix?.(item)}
      </div>
      {hasChildren && isOpen ? (
        <ul role="group" className="tree-nav__children">
          {item.children!.map((child) => (
            <TreeItem
              key={child.id}
              item={child}
              depth={depth + 1}
              expanded={expanded}
              selectedId={selectedId}
              onToggle={onToggle}
              onSelect={onSelect}
              renderSuffix={renderSuffix}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

export function TreeNav({
  nodes,
  selectedId,
  onSelect,
  defaultExpandDepth = 2,
  showExpandControls = false,
  emptyText = 'Разделов пока нет',
  ariaLabel = 'Дерево разделов',
  renderSuffix,
}: TreeNavProps) {
  const initialExpanded = useMemo(
    () => collectIds(nodes, 1, defaultExpandDepth, new Set<number>()),
    // Пересчёт только при смене дерева/глубины — ручное раскрытие не сбрасываем.
    [nodes, defaultExpandDepth],
  );
  const [expanded, setExpanded] = useState<Set<number>>(initialExpanded);

  const toggle = (id: number) => {
    setExpanded((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  if (!nodes.length) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={emptyText} />;
  }

  return (
    <Flex vertical gap={8} data-testid="tree-nav">
      {showExpandControls ? (
        <Space size={4}>
          <Button size="small" type="link" onClick={() => setExpanded(allIds(nodes))}>
            Развернуть всё
          </Button>
          <Button size="small" type="link" onClick={() => setExpanded(new Set())}>
            Свернуть всё
          </Button>
        </Space>
      ) : null}
      <ul role="tree" aria-label={ariaLabel} className="tree-nav">
        {nodes.map((node) => (
          <TreeItem
            key={node.id}
            item={node}
            depth={1}
            expanded={expanded}
            selectedId={selectedId}
            onToggle={toggle}
            onSelect={onSelect}
            renderSuffix={renderSuffix}
          />
        ))}
      </ul>
    </Flex>
  );
}
