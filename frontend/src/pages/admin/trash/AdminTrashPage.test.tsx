/**
 * `/admin/trash` — корзина (Ф7, TRASH-1..TRASH-5).
 *
 * Проверяем: вкладки, данные списка (кто и когда удалил, дата автоочистки),
 * предупреждение о деактивированном здании, восстановление заявки,
 * окончательное удаление с вводом подтверждения, конфликт регистрационного
 * номера при восстановлении оборудования и восстановление раздела БЗ.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { fetchTrash } from '@/api/admin';
import { fetchKbTree } from '@/api/kb';
import { fetchTicket } from '@/api/tickets';
import { ADMIN_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { AdminTrashPage } from './AdminTrashPage';
import { autoCleanupDate, formatDeletedAt } from './trashUtils';

function renderPage() {
  authenticateAs(ADMIN_AUTH.id);
  return renderWithProviders(<AdminTrashPage />, {
    auth: makeAuthValue({ user: ADMIN_AUTH }),
  });
}

async function openTab(name: string) {
  await userEvent.click(screen.getByRole('tab', { name }));
}

describe('AdminTrashPage: список и восстановление', () => {
  beforeEach(() => authenticateAs(ADMIN_AUTH.id));

  it('показывает дату удаления, автора, автоочистку и предупреждение о здании', async () => {
    renderPage();

    const restoreButton = await screen.findByTestId('trash-restore-ticket-13');
    const row = restoreButton.closest('tr') as HTMLElement;
    expect(row).toHaveTextContent('Заявка №13');
    expect(row).toHaveTextContent('Администратор Системы');
    // TRASH-4: дата автоочистки = дата удаления + 90 дней.
    const cleanup = within(row).getByTestId('trash-cleanup-ticket-13');
    expect(cleanup).toHaveTextContent(/\d{2}\.\d{2}\.\d{4}/);
    // TRASH-2: здание заявки деактивировано — предупреждаем до восстановления.
    expect(within(row).getByTestId('trash-warning-ticket-13')).toHaveTextContent(
      'Здание деактивировано',
    );
    expect(formatDeletedAt(new Date().toISOString())).toMatch(/\d{2}\.\d{2}\.\d{4}/);
    expect(autoCleanupDate(new Date().toISOString())).toMatch(/\d{2}\.\d{2}\.\d{4}/);
  });

  it('восстанавливает заявку: она уходит из корзины и снова доступна API', async () => {
    renderPage();
    await screen.findByTestId('trash-restore-ticket-13');

    await userEvent.click(screen.getByTestId('trash-restore-ticket-13'));
    expect(await screen.findByTestId('confirm-dialog-warning')).toHaveTextContent(
      'Здание деактивировано',
    );
    await userEvent.click(screen.getByTestId('confirm-dialog-ok'));

    await waitFor(() =>
      expect(screen.queryByTestId('trash-restore-ticket-13')).not.toBeInTheDocument(),
    );
    // Состояние мока: в корзине заявки больше нет, а сама заявка доступна (TRASH-2).
    const trash = await fetchTrash({ type: 'ticket' });
    expect(trash.items).toHaveLength(0);
    await expect(fetchTicket(13)).resolves.toMatchObject({ id: 13, building_id: 4 });
  });

  it('требует ввод подтверждения перед окончательным удалением (TRASH-3)', async () => {
    renderPage();
    await screen.findByTestId('trash-purge-ticket-13');

    await userEvent.click(screen.getByTestId('trash-purge-ticket-13'));
    const dialog = await screen.findByTestId('purge-dialog');
    expect(dialog).toHaveTextContent('необратимо');
    const submit = screen.getByTestId('purge-submit');
    expect(submit).toBeDisabled();

    await userEvent.type(screen.getByTestId('purge-confirm-input'), 'УДАЛИТЬ');
    await waitFor(() => expect(submit).toBeEnabled());

    await userEvent.click(submit);
    await waitFor(() =>
      expect(screen.queryByTestId('trash-purge-ticket-13')).not.toBeInTheDocument(),
    );
    const trash = await fetchTrash({ type: 'ticket' });
    expect(trash.items).toHaveLength(0);
    await expect(fetchTicket(13)).rejects.toMatchObject({ status: 404 });
  });

  it('не восстанавливает оборудование с занятым регистрационным номером', async () => {
    renderPage();
    await screen.findByTestId('trash-card');
    await openTab('Оборудование');

    await userEvent.click(await screen.findByTestId('trash-restore-equipment-10'));
    await userEvent.click(await screen.findByTestId('confirm-dialog-ok'));

    // TRASH-2: конфликт регистрационного номера → 409, элемент остаётся в корзине.
    // Текст есть и в предупреждении строки, и в тосте — проверяем наличие.
    expect((await screen.findAllByText(/Регистрационный номер уже занят/)).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByTestId('trash-restore-equipment-10')).toBeInTheDocument();
  });

  it('восстанавливает раздел БЗ из снимка (без потомков)', async () => {
    renderPage();
    await screen.findByTestId('trash-card');
    await openTab('База знаний');

    const restore = await screen.findByTestId('trash-restore-kb-990');
    const row = restore.closest('tr') as HTMLElement;
    expect(row).toHaveTextContent('Архивная инструкция');
    expect(within(row).getByTestId('trash-warning-kb-990')).toHaveTextContent('TRASH-5');

    await userEvent.click(restore);
    await userEvent.click(await screen.findByTestId('confirm-dialog-ok'));

    await waitFor(() =>
      expect(screen.queryByTestId('trash-restore-kb-990')).not.toBeInTheDocument(),
    );
    const tree = await fetchKbTree();
    expect(tree.some((node) => node.title === 'Архивная инструкция')).toBe(true);
  });
});
