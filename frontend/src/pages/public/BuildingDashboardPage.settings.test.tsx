/**
 * Публичный дашборд использует `GET /api/public/settings` (Б7): имя приложения
 * и подписи статусов из Admin-панели, с фолбэком на `app/config` при ошибке.
 */
import { describe, expect, it } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { APP_NAME } from '@/app/config';
import { server } from '@/mocks/server';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { BuildingDashboardPage } from './BuildingDashboardPage';

const CUSTOM_SETTINGS = {
  app_name: 'Сервис XYZ',
  statuses: [
    { code: 'new', label: 'Свежая', color: '#111111' },
    { code: 'in_progress', label: 'В работе', color: '#222222' },
    { code: 'deferred', label: 'Отложена', color: '#333333' },
    { code: 'done', label: 'Завершена', color: '#444444' },
  ],
};

function renderPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/b/:buildingId" element={<BuildingDashboardPage />} />
      <Route path="/" element={<div>home</div>} />
    </Routes>,
    { route: '/b/1', auth: makeAnonymousAuthValue() },
  );
}

function useEmptyTickets() {
  server.use(http.get('*/api/public/tickets', () => HttpResponse.json({ items: [], total: 0 })));
}

describe('BuildingDashboardPage + публичные настройки', () => {
  it('берёт app_name и подписи статусов из настроек', async () => {
    useEmptyTickets();
    server.use(http.get('*/api/public/settings', () => HttpResponse.json(CUSTOM_SETTINGS)));

    renderPage();
    await waitFor(() => expect(screen.getByTestId('public-column-new')).toBeInTheDocument());

    expect(screen.getByText(/Сервис XYZ · открытых заявок/)).toBeInTheDocument();
    expect(within(screen.getByTestId('public-column-new')).getByText('Свежая')).toBeInTheDocument();
    expect(
      within(screen.getByTestId('public-column-in_progress')).getByText('В работе'),
    ).toBeInTheDocument();
  });

  it('при недоступном /public/settings показывает значения по умолчанию', async () => {
    useEmptyTickets();
    server.use(
      http.get('*/api/public/settings', () =>
        HttpResponse.json({ code: 'INTERNAL_ERROR' }, { status: 500 }),
      ),
    );

    renderPage();
    await waitFor(() => expect(screen.getByTestId('public-column-new')).toBeInTheDocument());

    expect(screen.getByText(new RegExp(`${APP_NAME} · открытых заявок`))).toBeInTheDocument();
    expect(within(screen.getByTestId('public-column-new')).getByText('Новая')).toBeInTheDocument();
  });
});
