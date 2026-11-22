/**
 * Дерево разделов БЗ с drag-n-drop (KB-2, KB-6, KB-7).
 *
 * Вложенность не ограничена: каждый уровень — свой `<ul role="group">` и
 * собственный `SortableContext`, поэтому перетаскивание работает и внутри
 * уровня (сортировка), и между уровнями (смена родителя). Скрытые корневые
 * разделы помечены замком — флаг наследуется потомками (KB-1).
 *
 * Бизнес-правило: клиент не «угадывает» цикл — он отправляет перенос на
 * сервер, а `409 CYCLE_DETECTED` приводит к откату оптимистичного состояния
 * (это делает `useKbMove`). Здесь только вычисление цели броска.
 */
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core';
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Button, Empty, Flex, Tooltip, Typography } from 'antd';
import {
  CaretDownOutlined,
  CaretRightOutlined,
  FileTextOutlined,
  HolderOutlined,
  LockOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import type { KbTreeNode } from '@/api/types';
import {
  findKbPath,
  isKbMoveNoop,
  resolveKbDropTarget,
  type KbDropTarget,
} from '../tree';

export interface KbMoveRequestTarget extends KbDropTarget {
  nodeId: number;
}

export interface KbTreeProps {
  nodes: KbTreeNode[];
  selectedId: number | null;
  /** Совпавшие с поиском узлы — подсвечиваются. */
  matchIds?: Set<number>;
  /** Перетаскивание доступно всем авторизованным (KB-3); выключается в поиске. */
  canDrag?: boolean;
  onSelect: (id: number) => void;
  onMove?: (target: KbMoveRequestTarget) => void;
  onAddChild?: (parentId: number) => void;
  emptyText?: string;
}

function collectExpandable(nodes: KbTreeNode[], depth: number, maxDepth: number): number[] {
  const ids: number[] = [];
  for (const node of nodes) {
    if (node.children.length && depth < maxDepth) ids.push(node.id);
    ids.push(...collectExpandable(node.children, depth + 1, maxDepth));
  }
  return ids;
}

interface RowProps {
  node: KbTreeNode;
  depth: number;
  selectedId: number | null;
  expanded: boolean;
  matchIds?: Set<number>;
  canDrag: boolean;
  dragging: boolean;
  onToggle: (id: number) => void;
  onSelect: (id: number) => void;
  onAddChild?: (parentId: number) => void;
  children?: ReactNode;
}

function KbTreeRow({
  node,
  depth,
  selectedId,
  expanded,
  matchIds,
  canDrag,
  dragging,
  onToggle,
  onSelect,
  onAddChild,
  children,
}: RowProps) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } =
    useSortable({ id: node.id, data: { parentId: node.parent_id }, disabled: !canDrag });
  const { setNodeRef: setDropRef, isOver } = useDroppable({
    id: `child:${node.id}`,
    data: { parentId: node.id },
    disabled: !canDrag,
  });

  const isSelected = selectedId === node.id;
  const hasChildren = node.children.length > 0;
  const matched = matchIds?.has(node.id) ?? false;

  return (
    <li
      role="treeitem"
      aria-expanded={hasChildren ? expanded : undefined}
      aria-selected={isSelected}
      aria-level={depth}
      data-depth={depth}
      data-node-id={node.id}
      data-testid={`kb-node-${node.id}`}
      style={{ listStyle: 'none' }}
    >
      <div
        ref={setNodeRef}
        className={`kb-tree__row${isSelected ? ' kb-tree__row--selected' : ''}${
          isDragging ? ' kb-tree__row--dragging' : ''
        }`}
        style={{
          paddingInlineStart: (depth - 1) * 14 + 4,
          transform: CSS.Transform.toString(transform),
          transition,
          opacity: isDragging ? 0.5 : 1,
        }}
        onClick={() => onSelect(node.id)}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            onSelect(node.id);
          }
        }}
        tabIndex={0}
        role="button"
        aria-current={isSelected ? 'page' : undefined}
        data-testid={`kb-node-row-${node.id}`}
      >
        {canDrag ? (
          <Tooltip title="Перетащите, чтобы изменить порядок или вложенность">
            <Button
              type="text"
              size="small"
              className="kb-tree__drag"
              icon={<HolderOutlined />}
              aria-label={`Переместить раздел «${node.title}»`}
              data-testid={`kb-drag-${node.id}`}
              ref={setActivatorNodeRef}
              {...attributes}
              {...listeners}
              onClick={(event) => event.stopPropagation()}
              style={{ cursor: 'grab', padding: 0, width: 18, height: 18, minWidth: 18 }}
            />
          </Tooltip>
        ) : null}

        {hasChildren ? (
          <Button
            type="text"
            size="small"
            aria-label={expanded ? 'Свернуть раздел' : 'Развернуть раздел'}
            data-testid={`kb-toggle-${node.id}`}
            icon={expanded ? <CaretDownOutlined /> : <CaretRightOutlined />}
            onClick={(event) => {
              event.stopPropagation();
              onToggle(node.id);
            }}
            style={{ padding: 0, width: 18, height: 18, minWidth: 18 }}
          />
        ) : (
          <span style={{ display: 'inline-block', width: 18, flex: '0 0 18px' }} />
        )}

        <FileTextOutlined
          style={{ color: isSelected ? '#1677ff' : '#8c8c8c', flex: '0 0 auto', fontSize: 13 }}
        />

        <Typography.Text
          ellipsis
          strong={isSelected}
          mark={matched || undefined}
          style={{ flex: 1, minWidth: 0, fontSize: depth > 1 ? 13 : 14 }}
        >
          {node.title}
        </Typography.Text>

        {node.is_hidden_effective ? (
          <Tooltip title="Скрыт для анонимных посетителей">
            <LockOutlined
              style={{ color: '#8c8c8c', fontSize: 12 }}
              data-testid={`kb-lock-${node.id}`}
              aria-label="Скрыт для анонимных посетителей"
            />
          </Tooltip>
        ) : null}

        {onAddChild ? (
          <Tooltip title="Добавить подраздел">
            <Button
              type="text"
              size="small"
              icon={<PlusOutlined />}
              aria-label={`Добавить подраздел в «${node.title}»`}
              data-testid={`kb-add-child-${node.id}`}
              onClick={(event) => {
                event.stopPropagation();
                onAddChild(node.id);
              }}
              style={{ padding: 0, width: 18, height: 18, minWidth: 18 }}
            />
          </Tooltip>
        ) : null}
      </div>

      {/* Куда бросить, чтобы вложить раздел внутрь этого узла (KB-6).
          Элемент существует всегда (высота 0 вне drag), иначе dnd-kit
          не успевает измерить его к началу перетаскивания. */}
      <div
        ref={setDropRef}
        data-testid={`kb-drop-child-${node.id}`}
        className={`kb-tree__drop${dragging && isOver ? ' kb-tree__drop--over' : ''}`}
        style={{
          marginInlineStart: (depth - 1) * 14 + 40,
          height: dragging ? 12 : 0,
          marginBlock: dragging ? 2 : 0,
          border: dragging ? undefined : 'none',
          background: dragging ? undefined : 'transparent',
        }}
        aria-hidden
      />

      {hasChildren && expanded ? (
        <ul role="group" className="kb-tree__children">
          {children}
        </ul>
      ) : null}
    </li>
  );
}

interface LevelProps {
  nodes: KbTreeNode[];
  depth: number;
  parentId: number | null;
  selectedId: number | null;
  expanded: Set<number>;
  matchIds?: Set<number>;
  canDrag: boolean;
  dragging: boolean;
  onToggle: (id: number) => void;
  onSelect: (id: number) => void;
  onAddChild?: (parentId: number) => void;
}

function KbTreeLevel({
  nodes,
  depth,
  parentId,
  selectedId,
  expanded,
  matchIds,
  canDrag,
  dragging,
  onToggle,
  onSelect,
  onAddChild,
}: LevelProps) {
  return (
    <SortableContext
      items={nodes.map((node) => node.id)}
      strategy={verticalListSortingStrategy}
      id={`parent-${parentId ?? 'root'}`}
    >
      <ul
        role={depth === 1 ? 'tree' : 'group'}
        aria-label={depth === 1 ? 'Разделы базы знаний' : undefined}
        className="kb-tree"
        data-testid={depth === 1 ? 'kb-tree' : `kb-tree-level-${depth}`}
      >
        {nodes.map((node) => (
          <KbTreeRow
            key={node.id}
            node={node}
            depth={depth}
            selectedId={selectedId}
            expanded={expanded.has(node.id)}
            matchIds={matchIds}
            canDrag={canDrag}
            dragging={dragging}
            onToggle={onToggle}
            onSelect={onSelect}
            onAddChild={onAddChild}
          >
            <KbTreeLevel
              nodes={node.children}
              depth={depth + 1}
              parentId={node.id}
              selectedId={selectedId}
              expanded={expanded}
              matchIds={matchIds}
              canDrag={canDrag}
              dragging={dragging}
              onToggle={onToggle}
              onSelect={onSelect}
              onAddChild={onAddChild}
            />
          </KbTreeRow>
        ))}
      </ul>
    </SortableContext>
  );
}

function KbRootDropZone({ active, canDrag }: { active: boolean; canDrag: boolean }) {
  // Отдельный компонент: useDroppable должен вызываться ВНУТРИ DndContext,
  // иначе зона не зарегистрируется и вынести раздел в корень будет нельзя.
  const { setNodeRef, isOver } = useDroppable({ id: 'root-end', disabled: !canDrag });
  return (
    <div
      ref={setNodeRef}
      data-testid="kb-drop-root"
      className={`kb-tree__drop${active && isOver ? ' kb-tree__drop--over' : ''}`}
      style={{
        height: active ? 16 : 0,
        marginBlock: active ? 2 : 0,
        border: active ? undefined : 'none',
        background: active ? undefined : 'transparent',
      }}
      aria-hidden
    />
  );
}

export function KbTree({
  nodes,
  selectedId,
  matchIds,
  canDrag = true,
  onSelect,
  onMove,
  onAddChild,
  emptyText = 'Разделов пока нет',
}: KbTreeProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const initialExpanded = useMemo(() => new Set(collectExpandable(nodes, 1, 2)), [nodes]);
  const [expanded, setExpanded] = useState<Set<number>>(initialExpanded);
  const [draggingId, setDraggingId] = useState<number | null>(null);

  // Путь до выбранного узла раскрываем автоматически (глубина 5+ остаётся видимой).
  const selectedPathKey = useMemo(
    () =>
      findKbPath(nodes, selectedId)
        .map((node) => node.id)
        .join(','),
    [nodes, selectedId],
  );
  useEffect(() => {
    if (!selectedPathKey) return;
    const ids = selectedPathKey.split(',').map(Number);
    setExpanded((current) => {
      const next = new Set(current);
      ids.forEach((id) => next.add(id));
      return next;
    });
  }, [selectedPathKey]);

  const handleDragStart = (event: DragStartEvent) => {
    setDraggingId(Number(event.active.id));
  };

  const handleDragEnd = (event: DragEndEvent) => {
    setDraggingId(null);
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const nodeId = Number(active.id);
    if (!Number.isSafeInteger(nodeId)) return;
    const target = resolveKbDropTarget(nodes, over.id);
    if (!target || isKbMoveNoop(nodes, nodeId, target)) return;
    onMove?.({ nodeId, ...target });
  };

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
    <DndContext
      sensors={sensors}
      collisionDetection={closestCenter}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      onDragCancel={() => setDraggingId(null)}
    >
      <Flex vertical gap={4} data-testid="kb-tree-wrap">
        <KbTreeLevel
          nodes={nodes}
          depth={1}
          parentId={null}
          selectedId={selectedId}
          expanded={expanded}
          matchIds={matchIds}
          canDrag={canDrag}
          dragging={draggingId !== null}
          onToggle={toggle}
          onSelect={onSelect}
          onAddChild={onAddChild}
        />
        <KbRootDropZone active={draggingId !== null} canDrag={canDrag} />
      </Flex>
    </DndContext>
  );
}
