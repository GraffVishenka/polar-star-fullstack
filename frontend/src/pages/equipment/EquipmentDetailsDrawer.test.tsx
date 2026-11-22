/**
 * Карточка оборудования и журнал перемещений (Ф4, EQ-3/EQ-5).
 *
 * Проверяем: поля и локация в Drawer, лента журнала, перемещение без перезагрузки
 * (обновляются и локация, и журнал), `409 SAME_WAREHOUSE` и `422 BUILDING_INACTIVE`.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import { setAccessToken } from '@/api/client';
import { API } from '@/mocks/http';
import { issueAccessToken } from '@/mocks/auth';
import { server } from '@/mocks/server';
import { makeAuthUser, makeAuthValue, renderWithProviders } from '@/test/utils';
import { EquipmentPage } from './EquipmentPage';

function renderPage(isAdmin = false) {
  setAccessToken(issueAccessToken(isAdmin ? 1 : 2));
  return renderWithProviders(
    <Routes>
      <Route path="/equipment" element={<EquipmentPage />} />
    </Routes>,
    {
      route: '/equipment',
      auth: makeAuthValue({
        user: isAdmin
          ? makeAuthUser({
              id: 1,
              login: 'admin',
              role: 'ADMIN',
              building_id: null,
              building_name: null,
            })
          : makeAuthUser(),
      }),
    },
  );
}

/** Открывает карточку оборудования по кнопке в строке таблицы. */
async function openDrawer(id: number) {
  await userEvent.click(await screen.findByTestId(`open-equipment-${id}`));
  await screen.findByTestId('equipment-fields');
}

/** Выбирает вариант в antd Select по видимому тексту. */
async function chooseOption(combobox: HTMLElement, label: string | RegExp) {
  await userEvent.click(combobox);
  // В DOM может оставаться закрытый dropdown предыдущего селекта — ищем нужный текст
  // в самом новом dropdown.
  const option = await waitFor(() => {
    const dropdowns = [...document.querySelectorAll<HTMLElement>('.ant-select-dropdown')].reverse();
    for (const dropdown of dropdowns) {
      const found = within(dropdown).queryByText(label);
      if (found) return found;
    }
    throw new Error(`Вариант «${String(label)}» не найден`);
  });
  await userEvent.click(option);
}

/** Самый новый (только что открытый) выпадающий список antd Select. */
function latestDropdown(): HTMLElement {
  const dropdowns = [...document.querySelectorAll<HTMLElement>('.ant-select-dropdown')];
  const last = dropdowns[dropdowns.length - 1];
  if (!last) throw new Error('Выпадающий список не открылся');
  return last;
}

function moveDialogComboboxes(): HTMLElement[] {
  return within(screen.getByTestId('move-dialog')).getAllByRole('combobox');
}

describe('EquipmentDetailsDrawer: карточка и журнал', () => {
  beforeEach(() => setAccessToken(issueAccessToken(2)));

  it('показывает поля, текущую локацию и журнал перемещений таймлайном', async () => {
    renderPage();
    await openDrawer(1);

    const fields = screen.getByTestId('equipment-fields');
    expect(fields).toHaveTextContent('Ноутбук Dell Latitude');
    expect(fields).toHaveTextContent('Компьютер');
    expect(fields).toHaveTextContent('DL-4410-01');
    expect(fields).toHaveTextContent('INV-1001');

    const location = screen.getByTestId('equipment-location');
    expect(location).toHaveTextContent('Центральный офис');
    expect(location).toHaveTextContent('Склад ИТ');
    // EQ-8: ведомость помещения — заглушка до Ф8.
    expect(screen.getByTestId('inventory-stub')).toBeDisabled();

    const journal = await screen.findByTestId('movement-list');
    expect(within(journal).getByTestId('movement-item-1')).toHaveTextContent('Кабинет 101');
    expect(within(journal).getByTestId('movement-item-1')).toHaveTextContent('Склад ИТ');
    expect(within(journal).getByTestId('movement-item-1')).toHaveTextContent(
      'Иванов Иван Иванович',
    );
    expect(within(journal).getByTestId('movement-item-1')).toHaveTextContent('Перенос на склад ИТ');
  });

  it('перемещение из карточки обновляет локацию и журнал без перезагрузки', async () => {
    renderPage();
    await openDrawer(1);

    await userEvent.click(screen.getByTestId('open-move-dialog'));

    const [, warehouseSelect] = moveDialogComboboxes();
    // Здание подставлено текущее — меняем только помещение.
    await chooseOption(warehouseSelect, 'Кабинет 101');

    await userEvent.type(screen.getByTestId('move-comment'), 'Перенос на ремонт');
    await userEvent.click(screen.getByTestId('move-submit'));

    await waitFor(() =>
      expect(screen.getByTestId('equipment-location')).toHaveTextContent('Кабинет 101'),
    );
    const journal = await screen.findByTestId('movement-list');
    await waitFor(() => expect(within(journal).getAllByTestId(/^movement-item-/)).toHaveLength(2));
    expect(journal).toHaveTextContent('Перенос на ремонт');
  });

  it('в журнале помечает неактивное здание бейджем «Неактивно» (EQ-1)', async () => {
    renderPage();
    await openDrawer(9);

    expect(screen.getByTestId('equipment-location')).toHaveTextContent('Старый корпус');
    const journal = await screen.findByTestId('movement-list');
    const entry = within(journal).getByTestId('movement-item-3');
    expect(entry).toHaveTextContent('Выведен из эксплуатации, перенесён в архив');
    expect(within(entry).getByTestId('inactive-badge')).toBeInTheDocument();
  });

  it('409 SAME_WAREHOUSE показывается в диалоге и не меняет локацию', async () => {
    renderPage();
    await openDrawer(1);

    await userEvent.click(screen.getByTestId('open-move-dialog'));

    const [, warehouseSelect] = moveDialogComboboxes();
    await chooseOption(warehouseSelect, 'Склад ИТ');
    await userEvent.click(screen.getByTestId('move-submit'));

    expect(await screen.findByTestId('move-error')).toHaveTextContent(
      'Оборудование уже находится в этом помещении',
    );
    expect(screen.getByTestId('move-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('equipment-location')).toHaveTextContent('Склад ИТ');
  });

  it('USER не видит неактивное здание в списке целей, ADMIN видит с бейджем', async () => {
    renderPage();
    await openDrawer(1);
    await userEvent.click(screen.getByTestId('open-move-dialog'));

    const [buildingSelect] = moveDialogComboboxes();
    await userEvent.click(buildingSelect);
    const dropdown = latestDropdown();
    expect(within(dropdown).queryByText('Старый корпус')).not.toBeInTheDocument();
  });

  it('ADMIN видит неактивное здание с бейджем «Неактивно»', async () => {
    renderPage(true);
    await openDrawer(1);
    await userEvent.click(screen.getByTestId('open-move-dialog'));

    const [buildingSelect] = moveDialogComboboxes();
    await userEvent.click(buildingSelect);
    const dropdown = latestDropdown();
    expect(within(dropdown).getByText('Старый корпус')).toBeInTheDocument();
    expect(within(dropdown).getAllByTestId('inactive-badge').length).toBeGreaterThan(0);
  });

  it('422 BUILDING_INACTIVE от сервера объясняется понятным текстом (EQ-3)', async () => {
    server.use(
      http.post(`${API}/equipment/:id/move`, () =>
        HttpResponse.json(
          {
            code: 'BUILDING_INACTIVE',
            message: 'Нельзя переместить оборудование в неактивное здание',
            details: null,
            request_id: 'mock-test',
          },
          { status: 422 },
        ),
      ),
    );

    renderPage();
    await openDrawer(1);
    await userEvent.click(screen.getByTestId('open-move-dialog'));

    const [, warehouseSelect] = moveDialogComboboxes();
    await chooseOption(warehouseSelect, 'Кабинет 101');
    await userEvent.click(screen.getByTestId('move-submit'));

    expect(await screen.findByTestId('move-error')).toHaveTextContent(
      'это доступно только администратору',
    );
  });
});
