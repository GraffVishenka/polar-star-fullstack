/**
 * Оптимистичное обновление доски: смена статуса сразу двигает карточку,
 * а при 403 откатывается и показывает русскую ошибку (Ф2).
 */
import { HttpResponse, delay, http } from 'msw';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { registerToastApi } from '@/app/toast';
import { server } from '@/mocks/server';
import { USER_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { TicketsPage } from '../TicketsPage';

const userAuth = makeAuthValue({ user: USER_AUTH });

describe('смена статуса: оптимистично с откатом', () => {
  it('двигает карточку в целевую колонку, затем откатывает при 403', async () => {
    const toast = {
      success: vi.fn(),
      error: vi.fn(),
      warning: vi.fn(),
      info: vi.fn(),
    };
    registerToastApi(toast);

    // Сервер отклоняет правку: USER не исполнитель (TICKET-11).
    server.use(
      http.patch('*/api/tickets/:id', async () => {
        await delay(120);
        return HttpResponse.json(
          {
            code: 'NOT_EXECUTOR',
            message: 'not executor',
            details: null,
            request_id: 'req-403',
          },
          { status: 403 },
        );
      }),
    );

    authenticateAs(2);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: userAuth });

    const card = await screen.findByTestId('ticket-card-2');
    await userEvent.click(card);
    await screen.findByTestId('ticket-drawer-content');

    // Статус заявки 2 — «В работе»; меняем на «Отложена».
    await userEvent.click(within(screen.getByTestId('drawer-status')).getByText('Отложена'));

    // Оптимистично карточка уже в колонке «Отложена».
    await waitFor(() =>
      expect(
        within(screen.getByTestId('ticket-column-list-deferred')).getByTestId('ticket-card-2'),
      ).toBeInTheDocument(),
    );

    // Сервер ответил 403 — состояние откатилось, показан тост.
    await waitFor(() =>
      expect(
        within(screen.getByTestId('ticket-column-list-in_progress')).getByTestId('ticket-card-2'),
      ).toBeInTheDocument(),
    );
    expect(
      within(screen.getByTestId('ticket-column-list-deferred')).queryByTestId('ticket-card-2'),
    ).not.toBeInTheDocument();
    expect(toast.error).toHaveBeenCalledWith('Править заявку может только её исполнитель');
  });
});
