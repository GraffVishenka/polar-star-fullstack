/**
 * Массовое перемещение оборудования (Ф4, EQ-4).
 *
 * Проверяем: выбор чекбоксами, подтверждение, итог операции и обновление таблицы
 * без перезагрузки, а также ошибку 404 (одной из единиц уже нет — откат).
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import { setAccessToken } from '@/api/client';
import { issueAccessToken } from '@/mocks/auth';
import { API } from '@/mocks/http';
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

function rowCheckbox(id: number): HTMLInputElement {
  const row = document.querySelector<HTMLTableRowElement>(`tr[data-row-key="${id}"]`);
  const checkbox = row?.querySelector<HTMLInputElement>('input[type="checkbox"]');
  if (!checkbox) throw new Error(`Не найден чекбокс строки ${id}`);
  return checkbox;
}

/** Выбирает вариант в antd Select по видимому тексту. */
async function chooseOption(combobox: HTMLElement, label: string) {
  await userEvent.click(combobox);
  // В DOM может оставаться закрытый dropdown предыдущего селекта — берём самый новый,
  // в котором есть нужный текст.
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

describe('EquipmentPage: массовое перемещение', () => {
  beforeEach(() => setAccessToken(issueAccessToken(2)));

  it('перемещает выбранные единицы, показывает итог и обновляет таблицу', async () => {
    renderPage();
    expect(await screen.findByText('Ноутбук Dell Latitude')).toBeInTheDocument();

    await userEvent.click(rowCheckbox(1));
    await userEvent.click(rowCheckbox(2));

    const selectionBar = await screen.findByTestId('selection-bar');
    expect(selectionBar).toHaveTextContent('Выбрано единиц: 2');

    await userEvent.click(screen.getByTestId('bulk-move-open'));

    const dialog = screen.getByTestId('bulk-dialog');
    const comboboxes = within(dialog).getAllByRole('combobox');
    await chooseOption(comboboxes[0]!, 'Центральный офис');
    await chooseOption(comboboxes[1]!, 'Кабинет 101');

    await userEvent.click(screen.getByTestId('bulk-next'));
    expect(await screen.findByTestId('bulk-confirm-summary')).toHaveTextContent(
      'Переместить 2 ед. в «Кабинет 101»?',
    );

    await userEvent.click(screen.getByTestId('bulk-confirm'));
    expect(await screen.findByTestId('bulk-result')).toHaveTextContent('Перемещено единиц: 2');

    await userEvent.click(screen.getByTestId('bulk-done'));

    // Таблица обновилась без перезагрузки: обе единицы теперь в «Кабинете 101».
    await waitFor(() => {
      expect(document.querySelector('tr[data-row-key="1"]')).toHaveTextContent('Кабинет 101');
      expect(document.querySelector('tr[data-row-key="2"]')).toHaveTextContent('Кабинет 101');
    });
    // После успешной операции выбор сбрасывается.
    expect(screen.queryByTestId('selection-bar')).not.toBeInTheDocument();
  });

  it('404 по одной из единиц показывает ошибку и не меняет данные (откат)', async () => {
    server.use(
      http.post(`${API}/equipment/bulk-move`, () =>
        HttpResponse.json(
          {
            code: 'NOT_FOUND',
            message: 'Одна из единиц оборудования не найдена',
            details: { missing_ids: [42] },
            request_id: 'mock-test',
          },
          { status: 404 },
        ),
      ),
    );

    renderPage();
    expect(await screen.findByText('Ноутбук Dell Latitude')).toBeInTheDocument();

    await userEvent.click(rowCheckbox(1));
    await userEvent.click(rowCheckbox(2));
    await userEvent.click(await screen.findByTestId('bulk-move-open'));

    const dialog = screen.getByTestId('bulk-dialog');
    const comboboxes = within(dialog).getAllByRole('combobox');
    await chooseOption(comboboxes[0]!, 'Центральный офис');
    await chooseOption(comboboxes[1]!, 'Кабинет 101');
    await userEvent.click(screen.getByTestId('bulk-next'));
    await userEvent.click(await screen.findByTestId('bulk-confirm'));

    expect(await screen.findByTestId('bulk-error')).toHaveTextContent(
      'Одна из единиц не найдена — перемещение отменено',
    );
    // Данные не изменились, выбор сохранён — можно повторить после обновления списка.
    expect(document.querySelector('tr[data-row-key="1"]')).toHaveTextContent('Склад ИТ');
    expect(screen.getByTestId('selection-bar')).toBeInTheDocument();
  });
});
