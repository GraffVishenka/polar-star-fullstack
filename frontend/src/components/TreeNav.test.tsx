/** TreeNav: рекурсивное дерево и глубина ≥ 5 (KB-2). */
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { TreeNav, type TreeNavItem } from './TreeNav';

function buildDeepTree(depth: number): TreeNavItem[] {
  const root: TreeNavItem = { id: 1, title: 'Уровень 1' };
  let current = root;
  for (let level = 2; level <= depth; level += 1) {
    const child: TreeNavItem = { id: level, title: `Уровень ${level}` };
    current.children = [child];
    current = child;
  }
  return [root];
}

describe('TreeNav', () => {
  it('рендерит дерево глубины 6 с корректным data-depth', () => {
    render(<TreeNav nodes={buildDeepTree(6)} defaultExpandDepth={6} selectedId={6} />);

    for (let level = 1; level <= 6; level += 1) {
      const node = screen.getByTestId(`tree-node-${level}`);
      expect(node).toHaveAttribute('data-depth', String(level));
      expect(node).toHaveTextContent(`Уровень ${level}`);
    }
    expect(screen.getByTestId('tree-node-6')).toHaveAttribute('aria-selected', 'true');
  });

  it('скрывает вложенные уровни при defaultExpandDepth=1 и раскрывает по клику', () => {
    render(<TreeNav nodes={buildDeepTree(3)} defaultExpandDepth={1} />);

    expect(screen.queryByTestId('tree-node-2')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('tree-toggle-1'));

    expect(screen.getByTestId('tree-node-2')).toBeInTheDocument();
    expect(screen.queryByTestId('tree-node-3')).not.toBeInTheDocument();
  });

  it('вызывает onSelect и помечает скрытые разделы замком', () => {
    const onSelect = vi.fn();
    const nodes: TreeNavItem[] = [
      { id: 10, title: 'Публичный раздел' },
      { id: 11, title: 'Скрытый раздел', isHidden: true },
    ];

    render(<TreeNav nodes={nodes} onSelect={onSelect} />);

    fireEvent.click(screen.getByText('Скрытый раздел'));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 11 }));
    expect(screen.getByTestId('tree-lock')).toBeInTheDocument();
  });

  it('показывает пустое состояние без узлов', () => {
    render(<TreeNav nodes={[]} emptyText="Разделов нет" />);
    expect(screen.getByText('Разделов нет')).toBeInTheDocument();
    expect(screen.queryByTestId('tree-nav')).not.toBeInTheDocument();
  });
});
