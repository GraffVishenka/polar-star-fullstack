/**
 * Создание, правка и удаление оборудования (Ф4, EQ-2, TRASH-*).
 *
 * Ключевые проверки: `409 DUPLICATE_REGISTRATION_NUMBER` подсвечивается у поля,
 * удаление доступно только ADMIN и требует подтверждения, правка обновляет
 * карточку и таблицу без перезагрузки.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import { setAccessToken } from '@/api/client';
import { issueAccessToken } from '@/mocks/auth';
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

async function chooseOption(combobox: HTMLElement, label: string) {
  await userEvent.click(combobox);
  const option = await waitFor(() => {
    const dropdowns = [...document.querySelectorAll<HTMLElement>('.ant-select-dropdown')].reverse();
    for (const dropdown of dropdowns) {
      const found = within(dropdown).queryByText(label);
      if (found) return found;
    }
    throw new Error(`Вариант «${label}» не найден`);
  });
  await userEvent.click(option);
}

async function openDrawer(id: number) {
  await userEvent.click(await screen.findByTestId(`open-equipment-${id}`));
  await screen.findByTestId('equipment-fields');
}

/** Заполняет форму: тип, здание и помещение выбираются через antd Select. */
async function fillLocationAndType(typeName: string, buildingName: string, warehouseName: string) {
  const form = screen.getByTestId('equipment-form');
  const comboboxes = within(form).getAllByRole('combobox');
  await chooseOption(comboboxes[0]!, typeName);
  await chooseOption(comboboxes[1]!, buildingName);
  await chooseOption(comboboxes[2]!, warehouseName);
}

describe('EquipmentPage: создание и правка', () => {
  beforeEach(() => setAccessToken(issueAccessToken(2)));

  it('409 DUPLICATE_REGISTRATION_NUMBER подсвечивает поле регистрационного номера', async () => {
    renderPage();
    await userEvent.click(await screen.findByTestId('create-equipment'));

    await userEvent.type(
      within(screen.getByTestId('equipment-form')).getByLabelText('Наименование'),
      'Сканер Epson',
    );
    await userEvent.type(screen.getByLabelText('Серийный номер'), 'EPS-001');
    await userEvent.type(screen.getByTestId('equipment-form-registration'), 'INV-1001');
    await fillLocationAndType('Принтер', 'Центральный офис', 'Кабинет 101');

    await userEvent.click(screen.getByTestId('equipment-form-submit'));

    // Ошибка сервера (409) — и общий баннер, и подсветка конкретного поля.
    expect(await screen.findByTestId('equipment-form-error')).toHaveTextContent(
      'регистрационный номер уже используется',
    );
    await waitFor(() => expect(document.querySelector('.ant-form-item-has-error')).not.toBeNull());
    expect(document.querySelector('.ant-form-item-has-error')).toHaveTextContent(
      'регистрационный номер уже используется другой единицей',
    );
    // Форма осталась открытой с введёнными данными — можно исправить номер.
    expect(screen.getByTestId('equipment-form')).toBeInTheDocument();
  });

  it('создаёт оборудование с уникальным номером и показывает его в таблице', async () => {
    renderPage();
    await userEvent.click(await screen.findByTestId('create-equipment'));

    await userEvent.type(
      within(screen.getByTestId('equipment-form')).getByLabelText('Наименование'),
      'Сканер Epson',
    );
    await userEvent.type(screen.getByLabelText('Серийный номер'), 'EPS-002');
    await userEvent.type(screen.getByTestId('equipment-form-registration'), 'INV-7777');
    await fillLocationAndType('Принтер', 'Центральный офис', 'Кабинет 101');

    await userEvent.click(screen.getByTestId('equipment-form-submit'));

    await waitFor(() => expect(screen.getByText('Сканер Epson')).toBeInTheDocument());
    expect(screen.getByText('INV-7777')).toBeInTheDocument();
    expect(screen.queryByTestId('equipment-form-error')).not.toBeInTheDocument();
  });

  it('правка наименования обновляет карточку и таблицу без перезагрузки', async () => {
    renderPage();
    await openDrawer(1);

    await userEvent.click(screen.getByTestId('edit-equipment'));
    const nameInput = within(screen.getByTestId('equipment-form')).getByLabelText('Наименование');
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, 'Ноутбук Dell Latitude 5420');
    await userEvent.click(screen.getByTestId('equipment-form-submit'));

    await waitFor(() =>
      expect(screen.getByTestId('equipment-fields')).toHaveTextContent(
        'Ноутбук Dell Latitude 5420',
      ),
    );
    // Обновилась и строка таблицы под карточкой.
    expect(document.querySelector('tr[data-row-key="1"]')).toHaveTextContent(
      'Ноутбук Dell Latitude 5420',
    );
  });
});

describe('EquipmentPage: удаление (только ADMIN)', () => {
  it('USER не видит кнопку удаления в карточке', async () => {
    renderPage();
    await openDrawer(1);

    expect(screen.getByTestId('edit-equipment')).toBeInTheDocument();
    expect(screen.queryByTestId('delete-equipment')).not.toBeInTheDocument();
  });

  it('ADMIN удаляет оборудование после подтверждения', async () => {
    renderPage(true);
    await openDrawer(1);

    await userEvent.click(screen.getByTestId('delete-equipment'));
    expect(await screen.findByTestId('confirm-dialog-warning')).toHaveTextContent('корзины');

    await userEvent.click(screen.getByTestId('confirm-dialog-ok'));

    await waitFor(() =>
      expect(screen.queryByText('Ноутбук Dell Latitude')).not.toBeInTheDocument(),
    );
    expect(screen.queryByTestId('equipment-fields')).not.toBeInTheDocument();
  });
});
