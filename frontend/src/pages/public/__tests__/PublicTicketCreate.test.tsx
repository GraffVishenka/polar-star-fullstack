/**
 * Публичная форма создания заявки (TICKET-4, PUB-5):
 * подстановка/смена здания, валидация, экран со ссылкой `/t/{token}`,
 * обновление cookie и переход в дашборд выбранного здания.
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { registerToastApi } from '@/app/toast';
import { ANON_BUILDING_COOKIE, readAnonBuildingCookie } from '@/hooks/useAnonBuilding';
import { db } from '@/mocks/db';
import { pickSelectOption } from '@/test/selectHelpers';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { BuildingDashboardPage } from '../BuildingDashboardPage';

function renderPage(route = '/b/1') {
  return renderWithProviders(
    <Routes>
      <Route path="/b/:buildingId" element={<BuildingDashboardPage />} />
      <Route path="/t/:token" element={<div>token page</div>} />
      <Route path="/" element={<div>home page</div>} />
    </Routes>,
    { route, auth: makeAnonymousAuthValue() },
  );
}

async function openCreateForm() {
  await userEvent.click(await screen.findByTestId('create-ticket-button'));
  await screen.findByTestId('public-create-modal');
}

async function fillForm(applicant = 'Гость Посетитель') {
  await userEvent.type(screen.getByTestId('public-create-applicant'), applicant);
  await userEvent.type(screen.getByTestId('public-create-cabinet'), 'каб. 12');
  await pickSelectOption('public-create-problem', 'Протечка / сантехника');
  await userEvent.type(screen.getByTestId('public-create-description'), 'Течёт кран в столовой');
}

describe('PublicTicketCreateModal', () => {
  afterEach(() => {
    registerToastApi(null);
  });

  it('валидирует обязательные поля и не отправляет пустую форму', async () => {
    renderPage();
    await openCreateForm();

    await userEvent.click(screen.getByTestId('public-create-submit'));

    expect(await screen.findByText('Укажите заявителя')).toBeInTheDocument();
    expect(screen.getByText('Укажите кабинет')).toBeInTheDocument();
    expect(screen.getByText('Добавьте описание')).toBeInTheDocument();
    expect(screen.getAllByText('Выберите вид проблемы').length).toBeGreaterThan(1);
    expect(db.tickets).toHaveLength(13);
  });

  it('создаёт заявку в подставленном здании, показывает ссылку и копирует её', async () => {
    const toast = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() };
    registerToastApi(toast);
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    });

    renderPage();
    await openCreateForm();

    // Здание подставлено из дашборда (building 1).
    expect(screen.getByTestId('public-create-building')).toHaveTextContent('Центральный офис');

    await fillForm();
    await userEvent.click(screen.getByTestId('public-create-submit'));

    expect(await screen.findByTestId('public-create-success')).toBeInTheDocument();
    const created = db.tickets.at(-1)!;
    expect(created.building_id).toBe(1);
    expect(created.is_anonymous).toBe(true);
    expect(created.access_token).toBeTruthy();

    const link = screen.getByTestId('public-create-link');
    expect(link).toHaveTextContent(`/t/${created.access_token}`);

    await userEvent.click(screen.getByTestId('public-create-copy'));
    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(expect.stringContaining(`/t/${created.access_token}`)),
    );
    expect(toast.success).toHaveBeenCalledWith('Ссылка скопирована');

    // PUB-3: cookie обновлена на здание созданной заявки.
    expect(readAnonBuildingCookie()).toBe(1);
  });

  it('при смене здания в форме обновляет cookie и ведёт в дашборд выбранного здания', async () => {
    renderPage();
    await openCreateForm();

    await pickSelectOption('public-create-building', 'Производственный корпус');
    await fillForm('Гость Второго Здания');
    await userEvent.click(screen.getByTestId('public-create-submit'));

    await screen.findByTestId('public-create-success');
    expect(db.tickets.at(-1)?.building_id).toBe(2);

    // PUB-3: cookie переключается на выбранное здание сразу после создания.
    await waitFor(() => expect(readAnonBuildingCookie()).toBe(2));

    await userEvent.click(screen.getByTestId('public-create-go-dashboard'));

    expect(
      await screen.findByRole('heading', { name: 'Производственный корпус' }),
    ).toBeInTheDocument();
    await waitFor(() => expect(document.cookie).toContain(`${ANON_BUILDING_COOKIE}=2`));
  });
});
