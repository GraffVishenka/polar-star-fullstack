/**
 * `/b/:buildingId` и `/b/all` — каркас публичного дашборда (TICKET-5, PUB-2, PUB-4).
 * Три колонки без `done`, сводка «Все здания», 404 для неактивного здания.
 */
import { screen } from '@testing-library/react';
import { Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { BuildingDashboardPage } from './BuildingDashboardPage';

function renderPage(route: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/b/:buildingId" element={<BuildingDashboardPage />} />
    </Routes>,
    { route, auth: makeAnonymousAuthValue() },
  );
}

describe('BuildingDashboardPage', () => {
  it('показывает здание и 3 колонки без завершённых заявок', async () => {
    renderPage('/b/1');

    expect(await screen.findByTestId('building-dashboard-page')).toBeInTheDocument();
    // Название встречается в заголовке и в плашке здания — оба варианта валидны.
    expect(screen.getAllByText('Центральный офис').length).toBeGreaterThan(0);
    expect(screen.getByTestId('public-column-new')).toBeInTheDocument();
    expect(screen.getByTestId('public-column-in_progress')).toBeInTheDocument();
    expect(screen.getByTestId('public-column-deferred')).toBeInTheDocument();
    // TICKET-5: `done` в публичном дашборде отсутствует полностью.
    expect(screen.queryByTestId('public-column-done')).not.toBeInTheDocument();
  });

  it('в режиме «Все здания» показывает сводку и блокирует создание заявки', async () => {
    renderPage('/b/all');

    expect(await screen.findByTestId('building-dashboard-page')).toBeInTheDocument();
    expect(screen.getAllByText('Все здания').length).toBeGreaterThan(0);
    expect(screen.getByTestId('public-status-columns')).toBeInTheDocument();
    expect(screen.getByTestId('create-ticket-button')).toBeDisabled();
  });

  it('неактивное или несуществующее здание даёт экран «Здание не найдено» (PUB-2)', async () => {
    renderPage('/b/4');

    expect(await screen.findByText('Здание не найдено')).toBeInTheDocument();
  });
});
