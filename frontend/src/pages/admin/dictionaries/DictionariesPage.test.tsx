/**
 * Тесты `/admin/dictionaries` (Ф6): деактивация вместо удаления, счётчик
 * использования, переименование и порядок сортировки.
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { db } from '@/mocks/db';
import { authenticateAs, makeAuthUser, makeAuthValue, renderWithProviders } from '@/test/utils';
import { DictionariesPage } from './DictionariesPage';

const ADMIN = makeAuthUser({
  id: 1,
  login: 'admin',
  full_name: 'Администратор Системы',
  role: 'ADMIN',
  building_id: null,
  building_name: null,
});

function renderPage() {
  return renderWithProviders(<DictionariesPage />, {
    auth: makeAuthValue({ user: ADMIN }),
  });
}

beforeEach(() => {
  authenticateAs(1);
});

describe('DictionariesPage', () => {
  it('показывает справочник, счётчик использования и объясняет запрет удаления', async () => {
    renderPage();

    await waitFor(() =>
      expect(screen.getByTestId('dictionary-table-problem')).toBeInTheDocument(),
    );
    expect(await screen.findByText('Освещение')).toBeInTheDocument();
    expect(screen.getByTestId('dictionaries-info')).toHaveTextContent('Удаление запрещено');

    const usage = db.tickets.filter(
      (ticket) => ticket.problem_type_id === 1 && ticket.deleted_at === null,
    ).length;
    expect(usage).toBeGreaterThan(0);
    expect(screen.getByTestId('dictionary-usage-problem-1')).toHaveTextContent(String(usage));

    // Кнопка удаления недоступна всегда — вместо неё деактивация.
    expect(screen.getByTestId('dictionary-delete-problem-1')).toBeDisabled();
    // Неиспользуемое значение показывает «не используется».
    expect(screen.getByTestId('dictionary-usage-problem-6')).toHaveTextContent('не используется');
  });

  it('деактивирует вид проблемы, не удаляя его из истории', async () => {
    renderPage();
    const user = userEvent.setup();
    await screen.findByTestId('dictionary-table-problem');

    const activeSwitch = await screen.findByTestId('dictionary-active-problem-1');
    expect(activeSwitch).toBeChecked();
    await user.click(activeSwitch);

    await waitFor(() => expect(db.problemTypes.find((item) => item.id === 1)?.is_active).toBe(false));
    expect(await screen.findByTestId('dictionary-inactive-problem-1')).toHaveTextContent('неактивен');
    // Заявка исторически ссылается на деактивированный вид — значение осталось.
    expect(db.tickets.some((ticket) => ticket.problem_type_id === 1)).toBe(true);
  });

  it('переименовывает справочник и сохраняет порядок сортировки', async () => {
    renderPage();
    const user = userEvent.setup();
    await screen.findByTestId('dictionary-table-problem');

    await user.click(await screen.findByTestId('dictionary-edit-problem-2'));
    const nameInput = await screen.findByTestId('dictionary-name-input');
    await user.clear(nameInput);
    await user.type(nameInput, 'Сантехника и протечки');
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));

    await waitFor(() => {
      const updated = db.problemTypes.find((item) => item.id === 2);
      expect(updated?.name).toBe('Сантехника и протечки');
      expect(updated?.sort_order).toBe(20);
    });
    expect(await screen.findByText('Сантехника и протечки')).toBeInTheDocument();
  });

  it('создаёт новое значение справочника с порядком сортировки', async () => {
    renderPage();
    const user = userEvent.setup();
    await screen.findByTestId('dictionary-table-problem');

    await user.click(await screen.findByTestId('dictionary-add-problem'));
    const modal = await screen.findByTestId('dictionary-form-modal');
    await user.type(within(modal).getByTestId('dictionary-name-input'), 'Лифты и подъёмники');
    await user.type(within(modal).getByTestId('dictionary-sort-input'), '70');
    await user.click(within(modal).getByRole('button', { name: 'Сохранить' }));

    await waitFor(() => {
      const created = db.problemTypes.find((item) => item.name === 'Лифты и подъёмники');
      expect(created).toBeTruthy();
      expect(created?.sort_order).toBe(70);
      expect(created?.is_active).toBe(true);
    });
    expect(await screen.findByText('Лифты и подъёмники')).toBeInTheDocument();
  });

  it('деактивирует тип оборудования во второй вкладке', async () => {
    renderPage();
    const user = userEvent.setup();
    await screen.findByTestId('dictionary-table-problem');

    await user.click(screen.getByRole('tab', { name: 'Типы оборудования' }));
    const switchControl = await screen.findByTestId('dictionary-active-equipment-2');
    await user.click(switchControl);

    await waitFor(() =>
      expect(db.equipmentTypes.find((item) => item.id === 2)?.is_active).toBe(false),
    );
    expect(await screen.findByTestId('dictionary-inactive-equipment-2')).toHaveTextContent(
      'неактивен',
    );
  });
});
