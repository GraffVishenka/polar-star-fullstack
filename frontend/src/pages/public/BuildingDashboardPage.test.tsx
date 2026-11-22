/**
 * Публичный дашборд `/b/:buildingId` (PUB-2, PUB-4, TICKET-5).
 *
 * Проверяем: 3 колонки без `done`, только разрешённые поля, отсутствие
 * клиентского расчёта просрочки, режим «Все здания» без создания заявки,
 * 404 для неактивного здания.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { BuildingDashboardPage } from './BuildingDashboardPage';

function renderPage(route: string) {
  return renderWithProviders(
    <Routes>
      <Route path="/b/:buildingId" element={<BuildingDashboardPage />} />
      <Route path="/t/:token" element={<div>token page</div>} />
      <Route path="/" element={<div>home page</div>} />
    </Routes>,
    { route, auth: makeAnonymousAuthValue() },
  );
}

function columnCards(status: string) {
  return within(screen.getByTestId(`public-column-${status}`)).queryAllByTestId(
    /^public-ticket-card-/,
  );
}

describe('BuildingDashboardPage', () => {
  it('показывает 3 колонки и карточки только текущего здания', async () => {
    renderPage('/b/1');

    expect(await screen.findByTestId('building-dashboard-page')).toBeInTheDocument();
    expect(screen.getByTestId('public-column-new')).toBeInTheDocument();
    expect(screen.getByTestId('public-column-in_progress')).toBeInTheDocument();
    expect(screen.getByTestId('public-column-deferred')).toBeInTheDocument();
    // TICKET-5: завершённые заявки в публичный дашборд не попадают.
    expect(screen.queryByTestId('public-column-done')).not.toBeInTheDocument();

    await waitFor(() => expect(columnCards('new')).toHaveLength(1));
    expect(columnCards('in_progress')).toHaveLength(1);
    expect(columnCards('deferred')).toHaveLength(1);
    expect(screen.getByTestId('public-ticket-card-1')).toBeInTheDocument();
    // Заявки других зданий не показываются.
    expect(screen.queryByTestId('public-ticket-card-5')).not.toBeInTheDocument();
  });

  it('карточка содержит только разрешённые поля и не считает просрочку сама', async () => {
    renderPage('/b/1');

    const card = await screen.findByTestId('public-ticket-card-2');
    expect(card).toHaveTextContent('Сидоров Пётр Алексеевич');
    expect(card).toHaveTextContent('каб. 310');
    expect(card).toHaveTextContent('Протечка / сантехника');
    expect(card).toHaveTextContent('Иванов Иван Иванович');

    // Дедлайн в прошлом, но публичный API не отдаёт is_overdue — клиент не угадывает (NFR-7).
    expect(card).not.toHaveTextContent('Просрочено');
    expect(card).not.toHaveTextContent('Описание');
  });

  it('режим «Все здания» показывает сводку и блокирует создание заявки', async () => {
    renderPage('/b/all');

    expect(await screen.findByTestId('building-dashboard-page')).toBeInTheDocument();
    expect(screen.getAllByText('Все здания').length).toBeGreaterThan(0);

    await waitFor(() => expect(columnCards('new')).toHaveLength(4));
    expect(columnCards('in_progress')).toHaveLength(3);
    expect(columnCards('deferred')).toHaveLength(2);

    // Видно, к какому зданию относится заявка.
    expect(
      within(screen.getByTestId('public-column-new')).getAllByText(/корпус|офис|Склад/i).length,
    ).toBeGreaterThan(0);

    expect(screen.getByTestId('create-ticket-button')).toBeDisabled();
  });

  it('для неактивного здания показывает 404', async () => {
    renderPage('/b/4');
    expect(await screen.findByTestId('public-not-found')).toBeInTheDocument();
    expect(screen.getByText('Здание не найдено')).toBeInTheDocument();
  });

  it('для несуществующего здания показывает 404', async () => {
    renderPage('/b/999');
    expect(await screen.findByTestId('public-not-found')).toBeInTheDocument();
  });

  it('кнопка создания открывает публичную форму с подставленным зданием', async () => {
    renderPage('/b/1');

    const button = await screen.findByTestId('create-ticket-button');
    expect(button).toBeEnabled();
    await userEvent.click(button);

    expect(await screen.findByTestId('public-create-modal')).toBeInTheDocument();
    expect(screen.getByTestId('public-create-building')).toBeInTheDocument();
  });
});
