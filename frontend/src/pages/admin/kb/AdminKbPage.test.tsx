/**
 * `/admin/kb` — модерация базы знаний (Ф7, KB-3/KB-7).
 *
 * Проверяем: список корневых разделов с наследованием видимости, тумблер
 * `is_hidden_for_anon` (меняет состояние MSW-мока) и удаление раздела
 * с подтверждением и числом потомков из ответа API.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { fetchKbTree } from '@/api/kb';
import { ADMIN_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { AdminKbPage } from './AdminKbPage';

function renderPage() {
  authenticateAs(ADMIN_AUTH.id);
  return renderWithProviders(<AdminKbPage />, {
    auth: makeAuthValue({ user: ADMIN_AUTH }),
  });
}

describe('AdminKbPage: видимость и удаление', () => {
  beforeEach(() => authenticateAs(ADMIN_AUTH.id));

  it('показывает корневые разделы, скрытый раздел и наследование видимости', async () => {
    renderPage();

    expect(await screen.findByTestId('kb-table')).toBeInTheDocument();
    expect(screen.getByTestId('kb-title-1')).toHaveTextContent('Инструкции');
    expect(screen.getByTestId('kb-title-7')).toHaveTextContent('Оборудование');

    // Корневой раздел 10 скрыт от анонимов (KB-7) — бейдж и выключенный тумблер.
    expect(screen.getByTestId('kb-hidden-10')).toBeInTheDocument();
    expect(screen.getByTestId('kb-visibility-10')).toHaveAttribute('aria-checked', 'false');

    // У вложенного узла своего тумблера нет — видимость наследуется.
    const childRow = screen.getByTestId('kb-title-2').closest('tr') as HTMLElement;
    expect(within(childRow).getByText('наследуется от корня')).toBeInTheDocument();
    expect(within(childRow).queryByTestId('kb-visibility-2')).not.toBeInTheDocument();
  });

  it('тумблер скрывает корневой раздел для анонимов', async () => {
    renderPage();
    await screen.findByTestId('kb-table');

    expect(screen.getByTestId('kb-visibility-1')).toHaveAttribute('aria-checked', 'true');
    await userEvent.click(screen.getByTestId('kb-visibility-1'));

    // Состояние меняется в моках (KB-7): раздел и потомки скрыты.
    await waitFor(async () => {
      const tree = await fetchKbTree();
      const root = tree.find((node) => node.id === 1);
      expect(root?.is_hidden_for_anon).toBe(true);
    });
    expect(await screen.findByTestId('kb-hidden-1')).toBeInTheDocument();
  });

  it('удаляет раздел с подтверждением и показывает число потомков из ответа API', async () => {
    renderPage();
    await screen.findByTestId('kb-table');

    await userEvent.click(screen.getByTestId('kb-delete-1'));

    // У «Инструкций» пять потомков (2 → 3 → 4 → 5 → 6).
    const confirmBody = await screen.findByTestId('confirm-dialog-body');
    expect(confirmBody).toHaveTextContent('потомки (5)');
    expect(screen.getByTestId('confirm-dialog-warning')).toHaveTextContent('TRASH-5');

    await userEvent.click(screen.getByTestId('confirm-dialog-ok'));

    // Сервер вернул descendants_deleted — показываем его в результате (KB-3).
    expect(await screen.findByText(/потомков удалено: 5/)).toBeInTheDocument();

    await waitFor(() => expect(screen.queryByTestId('kb-title-1')).not.toBeInTheDocument());
    const tree = await fetchKbTree();
    expect(tree.some((node) => node.id === 1)).toBe(false);
    expect(tree.some((node) => node.id === 6)).toBe(false);
  });

  it('возвращает скрытый раздел анонимам повторным переключением', async () => {
    renderPage();
    await screen.findByTestId('kb-table');

    await userEvent.click(screen.getByTestId('kb-visibility-10'));
    await waitFor(async () => {
      const tree = await fetchKbTree();
      expect(tree.find((node) => node.id === 10)?.is_hidden_for_anon).toBe(false);
    });
    await waitFor(() => expect(screen.queryByTestId('kb-hidden-10')).not.toBeInTheDocument());
  });
});
