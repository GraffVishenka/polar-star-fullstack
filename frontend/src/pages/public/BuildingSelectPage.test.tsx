/**
 * `/` — выбор здания (PUB-1, TICKET-6): карточки активных зданий со счётчиками,
 * редирект по cookie, ссылка «Сменить здание» с `?change=1`.
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { ANON_BUILDING_COOKIE } from '@/hooks/useAnonBuilding';
import { BuildingSelectPage } from './BuildingSelectPage';

function renderPage(route = '/') {
  return renderWithProviders(
    <Routes>
      <Route path="/" element={<BuildingSelectPage />} />
      <Route path="/b/:buildingId" element={<div>dashboard page</div>} />
    </Routes>,
    { route, auth: makeAnonymousAuthValue() },
  );
}

describe('BuildingSelectPage', () => {
  it('показывает только активные здания со счётчиками', async () => {
    renderPage();

    expect(await screen.findByTestId('building-card-1')).toBeInTheDocument();
    expect(screen.getByTestId('building-card-2')).toBeInTheDocument();
    expect(screen.getByTestId('building-card-3')).toBeInTheDocument();
    // Неактивное здание (id 4) в публичный список не попадает (TICKET-6).
    expect(screen.queryByTestId('building-card-4')).not.toBeInTheDocument();
    expect(screen.queryByText('Старый корпус')).not.toBeInTheDocument();

    expect(screen.getByTestId('building-card-1')).toHaveTextContent('Центральный офис');
    expect(screen.getByTestId('building-card-1')).toHaveTextContent('Новые');
    expect(screen.getByTestId('building-card-1')).toHaveTextContent('В работе');
    expect(screen.getByTestId('building-card-1')).toHaveTextContent('Отложены');
  });

  it('с валидной cookie сразу ведёт на дашборд здания (PUB-1)', async () => {
    document.cookie = `${ANON_BUILDING_COOKIE}=2; Path=/`;
    renderPage();

    expect(await screen.findByText('dashboard page')).toBeInTheDocument();
    expect(screen.queryByTestId('building-select-page')).not.toBeInTheDocument();
  });

  it('`?change=1` отключает авторедирект и даёт выбрать другое здание', async () => {
    document.cookie = `${ANON_BUILDING_COOKIE}=2; Path=/`;
    renderPage('/?change=1');

    expect(await screen.findByTestId('building-select-page')).toBeInTheDocument();
    expect(screen.queryByText('dashboard page')).not.toBeInTheDocument();
  });

  it('клик по карточке запоминает здание и открывает его дашборд', async () => {
    renderPage();
    const card = await screen.findByTestId('building-card-3');

    await userEvent.click(card);

    expect(await screen.findByText('dashboard page')).toBeInTheDocument();
    await waitFor(() =>
      expect(document.cookie).toContain(`${ANON_BUILDING_COOKIE}=3`),
    );
  });
});
