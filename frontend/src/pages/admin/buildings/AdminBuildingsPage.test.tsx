/**
 * `/admin/buildings` — здания и склады (Ф7, EQ-1/EQ-6).
 *
 * Проверяем: таблицу зданий со счётчиком складов и бейджем неактивности,
 * запрет удаления используемого здания (причины из `details.reasons`),
 * деактивацию с исчезновением из публичного выбора, запрет удаления непустого
 * склада и уникальность имени помещения.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it } from 'vitest';
import { fetchPublicBuildings } from '@/api/public';
import { ADMIN_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { AdminBuildingsPage } from './AdminBuildingsPage';

function renderPage() {
  authenticateAs(ADMIN_AUTH.id);
  return renderWithProviders(<AdminBuildingsPage />, {
    auth: makeAuthValue({ user: ADMIN_AUTH }),
  });
}

describe('AdminBuildingsPage: здания', () => {
  beforeEach(() => authenticateAs(ADMIN_AUTH.id));

  it('показывает здания, число складов и бейдж неактивности', async () => {
    renderPage();

    expect(await screen.findByTestId('buildings-table')).toBeInTheDocument();
    const centralRow = document.querySelector('tr[data-row-key="1"]');
    expect(centralRow).toHaveTextContent('Центральный офис');
    expect(centralRow).toHaveTextContent('2');
    expect(screen.getByTestId('building-inactive-4')).toHaveTextContent('Неактивно');

    // По умолчанию выбран первый дом — видны его помещения.
    const panel = await screen.findByTestId('warehouses-panel');
    expect(panel).toHaveTextContent('Кабинет 101');
    expect(panel).toHaveTextContent('Склад ИТ');
    expect(panel).not.toHaveTextContent('Цех А');
  });

  it('запрещает удалить используемое здание и показывает причины из details.reasons', async () => {
    renderPage();
    await screen.findByTestId('buildings-table');

    await userEvent.click(screen.getByTestId('building-delete-1'));
    await userEvent.click(await screen.findByTestId('confirm-dialog-ok'));

    expect((await screen.findAllByText('Здание нельзя удалить')).length).toBeGreaterThan(0);
    const reasons = await screen.findByTestId('infra-error-reasons');
    expect(reasons).toHaveTextContent('Складов/кабинетов: 2');
    expect(reasons).toHaveTextContent('Заявок:');
    // Подсказка о деактивации вместо удаления (EQ-6).
    expect(screen.getByTestId('infra-error-content')).toHaveTextContent(/деактивируйте/i);
    // Здание осталось в таблице.
    expect(document.querySelector('tr[data-row-key="1"]')).toBeInTheDocument();
  });

  it('деактивация убирает здание из публичного выбора зданий', async () => {
    renderPage();
    await screen.findByTestId('buildings-table');

    // Пока здание активно — оно есть в публичном списке.
    await waitFor(async () => {
      const publicBefore = await fetchPublicBuildings();
      expect(publicBefore.some((building) => building.id === 1)).toBe(true);
    });

    await userEvent.click(screen.getByTestId('building-active-1'));
    await userEvent.click(await screen.findByTestId('confirm-dialog-ok'));

    await waitFor(async () => {
      const publicAfter = await fetchPublicBuildings();
      expect(publicAfter.some((building) => building.id === 1)).toBe(false);
    });
    await waitFor(() =>
      expect(screen.getByTestId('building-active-1')).toHaveAttribute('aria-checked', 'false'),
    );
    expect(screen.getByTestId('building-inactive-1')).toHaveTextContent('Неактивно');
  });

  it('создаёт здание и добавляет его в таблицу', async () => {
    renderPage();
    await screen.findByTestId('buildings-table');

    await userEvent.click(screen.getByTestId('building-add'));
    await userEvent.type(screen.getByLabelText('Название'), 'Новый корпус');
    await userEvent.type(screen.getByLabelText('Адрес'), 'ул. Новая, 1');
    await userEvent.click(screen.getByTestId('building-form-submit'));

    expect(await screen.findByText('Новый корпус')).toBeInTheDocument();
  });
});

describe('AdminBuildingsPage: склады', () => {
  beforeEach(() => authenticateAs(ADMIN_AUTH.id));

  it('не даёт удалить непустой склад и сохраняет данные', async () => {
    renderPage();
    const panel = await screen.findByTestId('warehouses-panel');
    expect(panel).toHaveTextContent('Склад ИТ');

    await userEvent.click(screen.getByTestId('warehouse-delete-2'));
    await userEvent.click(await screen.findByTestId('confirm-dialog-ok'));

    expect((await screen.findAllByText('Помещение нельзя удалить')).length).toBeGreaterThan(0);
    expect(await screen.findByTestId('infra-error-reasons')).toHaveTextContent(
      'Единиц оборудования: 2',
    );
    // WAREHOUSE_NOT_EMPTY: данные не потеряны, склад остаётся в списке.
    expect(screen.getByTestId('warehouses-panel')).toHaveTextContent('Склад ИТ');
  });

  it('удаляет пустое помещение', async () => {
    renderPage();
    await screen.findByTestId('warehouses-panel');

    // Все помещения сида заняты оборудованием — создаём пустое и удаляем его.
    await userEvent.click(screen.getByTestId('warehouse-add'));
    const form = await screen.findByTestId('warehouse-form');
    await userEvent.type(within(form).getByLabelText('Название помещения'), 'Временный склад');
    await userEvent.click(screen.getByTestId('warehouse-form-submit'));

    const panel = await screen.findByTestId('warehouses-panel');
    await waitFor(() => expect(panel).toHaveTextContent('Временный склад'));

    const createdRow = screen
      .getAllByTestId(/^warehouse-delete-/)
      .find((button) => button.closest('tr')?.textContent?.includes('Временный склад'));
    expect(createdRow).toBeDefined();
    await userEvent.click(createdRow as HTMLElement);
    await userEvent.click(await screen.findByTestId('confirm-dialog-ok'));

    await waitFor(() =>
      expect(screen.getByTestId('warehouses-panel')).not.toHaveTextContent('Временный склад'),
    );
    expect(screen.queryByTestId('infra-error-content')).not.toBeInTheDocument();
  });

  it('запрещает дублировать имя помещения в одном здании', async () => {
    renderPage();
    await screen.findByTestId('warehouses-panel');

    await userEvent.click(screen.getByTestId('warehouse-add'));
    const form = await screen.findByTestId('warehouse-form');
    await userEvent.type(within(form).getByLabelText('Название помещения'), 'Кабинет 101');
    await userEvent.click(screen.getByTestId('warehouse-form-submit'));

    expect(await screen.findByTestId('warehouse-form-error')).toHaveTextContent(
      'уже есть в здании',
    );
    // Форма остаётся открытой для исправления имени.
    expect(screen.getByTestId('warehouse-form')).toBeInTheDocument();
  });

  it('создаёт помещение и показывает его в панели', async () => {
    renderPage();
    await screen.findByTestId('warehouses-panel');

    await userEvent.click(screen.getByTestId('warehouse-add'));
    const form = await screen.findByTestId('warehouse-form');
    await userEvent.type(within(form).getByLabelText('Название помещения'), 'Серверная');
    await userEvent.click(screen.getByTestId('warehouse-form-submit'));

    expect(await screen.findByText('Серверная')).toBeInTheDocument();
  });
});
