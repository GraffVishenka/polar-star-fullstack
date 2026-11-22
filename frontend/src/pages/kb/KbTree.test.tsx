/**
 * Тесты дерева БЗ: вложенность ≥ 5 (KB-2), замок скрытых разделов (KB-7),
 * поиск по заголовкам и действия узла.
 */
import { describe, expect, it, vi } from 'vitest';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { KbTreeNode } from '@/api/types';
import { renderWithProviders } from '@/test/utils';
import { KbTree } from './components/KbTree';
import { filterKbTree } from './tree';

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
  const twelve = node(12, 'Заявки на проверку', 10, 20, [], { is_hidden_effective: true });
  const eleven = node(11, 'Пользователи и роли', 10, 10, [], { is_hidden_effective: true });
  const ten = node(10, 'Администрирование', null, 30, [eleven, twelve], {
    is_hidden_for_anon: true,
    is_hidden_effective: true,
  });
  return [one, seven, ten];
}

describe('KbTree', () => {
  it('раскрывает путь до узла 6-го уровня и помечает скрытые разделы замком', () => {
    renderWithProviders(<KbTree nodes={makeTree()} selectedId={6} onSelect={vi.fn()} />);

    // Путь 1→2→3→4→5→6 раскрыт автоматически, глубина видна в data-depth.
    const leaf = screen.getByTestId('kb-node-6');
    expect(leaf).toHaveAttribute('data-depth', '6');
    expect(screen.getByTestId('kb-node-1')).toHaveAttribute('data-depth', '1');
    expect(screen.getByTestId('kb-node-5')).toBeInTheDocument();

    // Замок у скрытого корня и у его потомка (флаг наследуется, KB-1).
    expect(screen.getByTestId('kb-lock-10')).toBeInTheDocument();
    expect(screen.getByTestId('kb-lock-11')).toBeInTheDocument();
    expect(screen.queryByTestId('kb-lock-1')).toBeNull();

    // Зоны для перемещения: внутрь раздела и в конец корневого уровня (KB-6).
    expect(screen.getByTestId('kb-drop-child-1')).toBeInTheDocument();
    expect(screen.getByTestId('kb-drop-root')).toBeInTheDocument();
  });

  it('показывает результат поиска по заголовкам с подсветкой', () => {
    const { tree, matchIds } = filterKbTree(makeTree(), 'дедлайн');
    renderWithProviders(
      // selectedId раскрывает путь к найденному узлу (как в KbPage после поиска).
      <KbTree nodes={tree} selectedId={5} matchIds={matchIds} onSelect={vi.fn()} />,
    );

    expect(screen.getByTestId('kb-node-5')).toBeInTheDocument();
    // Раздел вне результата поиска не рендерится.
    expect(screen.queryByTestId('kb-node-7')).toBeNull();
    expect(document.querySelector('mark')).not.toBeNull();
  });

  it('позволяет выбрать раздел и открыть создание подраздела', async () => {
    const onSelect = vi.fn();
    const onAddChild = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(
      <KbTree
        nodes={makeTree()}
        selectedId={null}
        onSelect={onSelect}
        onAddChild={onAddChild}
      />,
    );

    await user.click(screen.getByTestId('kb-node-row-2'));
    expect(onSelect).toHaveBeenCalledWith(2);

    await user.click(screen.getByTestId('kb-add-child-2'));
    expect(onAddChild).toHaveBeenCalledWith(2);
    // Клик по кнопке «+» не меняет выбранный раздел.
    expect(onSelect).toHaveBeenCalledTimes(1);
  });

  it('показывает пустое состояние, когда разделов нет', () => {
    renderWithProviders(
      <KbTree nodes={[]} selectedId={null} onSelect={vi.fn()} emptyText="Разделов пока нет" />,
    );
    expect(screen.getByText('Разделов пока нет')).toBeInTheDocument();
  });
});
