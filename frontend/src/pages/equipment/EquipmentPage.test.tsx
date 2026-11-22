/**
 * `/equipment` — навигация, фильтры в URL и серверная пагинация (Ф4, EQ-7).
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import {
  ADMIN_AUTH,
  USER_AUTH,
  authenticateAs,
  makeAuthValue,
  renderWithProviders,
} from '@/test/utils';
import { EquipmentPage } from './EquipmentPage';

/** Показывает текущий query-строку — так проверяем, что фильтры живут в URL. */
function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location-search">{location.search}</div>;
}

function renderPage(route = '/equipment', isAdmin = false) {
  authenticateAs(isAdmin ? ADMIN_AUTH.id : USER_AUTH.id);
  return renderWithProviders(
    <Routes>
      <Route
        path="/equipment"
        element={
          <>
            <EquipmentPage />
            <LocationProbe />
          </>
        }
      />
    </Routes>,
    {
      route,
      auth: makeAuthValue({ user: isAdmin ? ADMIN_AUTH : USER_AUTH }),
    },
  );
}

describe('EquipmentPage: фильтры и навигация', () => {
  beforeEach(() => {
    authenticateAs(USER_AUTH.id);
  });

  it('применяет фильтр по зданию из URL и показывает только его оборудование', async () => {
    renderPage('/equipment?building=1');

    expect(await screen.findByText('Ноутбук Dell Latitude')).toBeInTheDocument();
    expect(screen.getByText('Принтер HP LaserJet')).toBeInTheDocument();
    expect(screen.queryByText('Принтер Canon')).not.toBeInTheDocument();
    expect(screen.queryByText('Сервер HP ProLiant')).not.toBeInTheDocument();

    // Значение фильтра в селекте соответствует адресу.
    expect(screen.getByTestId('filter-building')).toHaveTextContent('Центральный офис');
  });

  it('выбор здания в дереве обновляет URL и таблицу', async () => {
    renderPage();

    expect(await screen.findByText('Ноутбук Dell Latitude')).toBeInTheDocument();

    await userEvent.click(screen.getByTestId('tree-building-2'));

    await waitFor(() =>
      expect(screen.getByTestId('location-search')).toHaveTextContent('building=2'),
    );
    expect(await screen.findByText('Принтер Canon')).toBeInTheDocument();
    expect(screen.queryByText('Ноутбук Dell Latitude')).not.toBeInTheDocument();
  });

  it('выбор склада в дереве фиксирует здание и помещение в URL', async () => {
    renderPage();

    await userEvent.click(await screen.findByTestId('tree-warehouse-3'));

    await waitFor(() =>
      expect(screen.getByTestId('location-search')).toHaveTextContent('building=2'),
    );
    expect(screen.getByTestId('location-search')).toHaveTextContent('warehouse=3');
    expect(await screen.findByText('Компьютер сборный')).toBeInTheDocument();
    expect(screen.queryByText('Ноутбук Dell Latitude')).not.toBeInTheDocument();
  });

  it('поиск по регистрационному номеру уходит в URL и фильтрует выборку', async () => {
    renderPage();

    const search = await screen.findByTestId('filter-search');
    await userEvent.type(search, 'INV-1003{Enter}');

    await waitFor(() =>
      expect(screen.getByTestId('location-search')).toHaveTextContent('q=INV-1003'),
    );
    expect(await screen.findByText('Принтер HP LaserJet')).toBeInTheDocument();
    expect(screen.queryByText('Монитор LG 24"')).not.toBeInTheDocument();
  });

  it('страница и размер страницы берутся из URL (серверная пагинация)', async () => {
    renderPage('/equipment?size=2&page=2');

    // Порядок стабилен по id: страница 2 размера 2 — единицы 3 и 4.
    expect(await screen.findByText('Принтер HP LaserJet')).toBeInTheDocument();
    expect(screen.getByText('Стол письменный')).toBeInTheDocument();
    expect(screen.queryByText('Ноутбук Dell Latitude')).not.toBeInTheDocument();
    expect(screen.getByText('Всего единиц: 9')).toBeInTheDocument();
  });

  it('показывает бейдж «Неактивно» для оборудования деактивированного здания', async () => {
    renderPage('/equipment?building=4');

    expect(await screen.findByText('Сервер HP ProLiant')).toBeInTheDocument();
    expect(screen.getAllByTestId('inactive-badge').length).toBeGreaterThan(0);
    expect(screen.getByTestId('inactive-building-alert')).toBeInTheDocument();
  });
});
