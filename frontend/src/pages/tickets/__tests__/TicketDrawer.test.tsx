/**
 * Карточка заявки в Drawer: права USER/ADMIN, «Назначить себя», смена здания
 * с предупреждением о сбросе исполнителя (TICKET-9), удаление в корзину (TICKET-12).
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { registerToastApi } from '@/app/toast';
import {
  ADMIN_AUTH,
  USER_AUTH,
  authenticateAs,
  makeAuthValue,
  renderWithProviders,
} from '@/test/utils';
import { TicketsPage } from '../TicketsPage';
import { pickSelectOption } from './testHelpers';

const adminAuth = makeAuthValue({ user: ADMIN_AUTH });
const userAuth = makeAuthValue({ user: USER_AUTH });

function toastSpy() {
  const api = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() };
  registerToastApi(api);
  return api;
}

async function openTicket(id: number) {
  const card = await screen.findByTestId(`ticket-card-${id}`);
  await userEvent.click(card);
  await screen.findByTestId('ticket-drawer-content');
}

describe('TicketDrawer: USER', () => {
  it('показывает поля заявки и позволяет назначить себя исполнителем', async () => {
    authenticateAs(2);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: userAuth });

    await openTicket(1);

    expect(screen.getByText('Заявка №1')).toBeInTheDocument();
    expect(screen.getByText('каб. 204')).toBeInTheDocument();
    expect(screen.getByTestId('drawer-executor-name')).toHaveTextContent('Не назначен');

    await userEvent.click(screen.getByTestId('assign-self'));

    await waitFor(() =>
      expect(screen.getByTestId('drawer-executor-name')).toHaveTextContent('Иванов Иван Иванович'),
    );
    expect(screen.getByText('вы исполнитель')).toBeInTheDocument();
  });

  it('не показывает ADMIN-действия: дедлайн, здание, удаление', async () => {
    authenticateAs(2);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: userAuth });

    await openTicket(2);

    expect(screen.queryByTestId('drawer-deadline')).not.toBeInTheDocument();
    expect(screen.queryByTestId('drawer-building-select')).not.toBeInTheDocument();
    expect(screen.queryByTestId('drawer-executor-select')).not.toBeInTheDocument();
    expect(screen.queryByTestId('delete-ticket')).not.toBeInTheDocument();
    expect(screen.getByTestId('drawer-executor-name')).toHaveTextContent('Иванов Иван Иванович');
  });
});

describe('TicketDrawer: ADMIN', () => {
  it('показывает управление дедлайном, зданием и удалением', async () => {
    authenticateAs(1);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: adminAuth });

    await openTicket(2);

    expect(screen.getByTestId('drawer-deadline')).toBeInTheDocument();
    expect(screen.getByTestId('drawer-building-select')).toBeInTheDocument();
    expect(screen.getByTestId('drawer-executor-select')).toBeInTheDocument();
    expect(screen.getByTestId('delete-ticket')).toBeInTheDocument();
  });

  it('предупреждает о сбросе USER-исполнителя при смене здания и показывает executor_reset', async () => {
    authenticateAs(1);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: adminAuth });

    await openTicket(2);
    expect(screen.getByTestId('drawer-executor-select')).toHaveTextContent('Иванов Иван Иванович');

    await pickSelectOption('drawer-building-select', 'Производственный корпус');

    const warning = await screen.findByTestId('confirm-dialog-warning');
    expect(warning).toHaveTextContent(/Иванов Иван Иванович/);
    expect(warning).toHaveTextContent(/будет снят/);
    expect(screen.getByTestId('confirm-dialog-body')).toHaveTextContent('Производственный корпус');

    await userEvent.click(screen.getByTestId('confirm-dialog-ok'));

    await waitFor(() => expect(screen.getByTestId('executor-reset-notice')).toBeInTheDocument());
    await waitFor(() =>
      expect(screen.getByTestId('drawer-executor-select')).toHaveTextContent('Не назначен'),
    );
    // Заявка действительно переехала в другое здание.
    expect(screen.getByTestId('ticket-drawer-content')).toHaveTextContent(
      'Производственный корпус',
    );
  });

  it('удаляет заявку в корзину только после подтверждения', async () => {
    const toast = toastSpy();
    authenticateAs(1);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: adminAuth });

    await openTicket(1);
    await userEvent.click(screen.getByTestId('delete-ticket'));

    expect(await screen.findByTestId('confirm-dialog-body')).toHaveTextContent(
      'Восстановить её можно',
    );
    await userEvent.click(screen.getByTestId('confirm-dialog-ok'));

    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith('Заявка №1 перемещена в корзину'),
    );
    await waitFor(() => expect(screen.queryByTestId('ticket-card-1')).not.toBeInTheDocument());
  });
});
