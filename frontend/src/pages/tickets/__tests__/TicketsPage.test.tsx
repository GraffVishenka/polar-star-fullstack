/**
 * Доска заявок (Ф2): 4 колонки, серверные счётчики, права USER/ADMIN,
 * фильтры из URL и «без исполнителя», баннер AUTH-9.
 */
import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import {
  ADMIN_AUTH,
  USER_AUTH,
  authenticateAs,
  makeAuthUser,
  makeAuthValue,
  renderWithProviders,
} from '@/test/utils';
import { TicketsPage } from '../TicketsPage';

const adminAuth = makeAuthValue({ user: ADMIN_AUTH });
const userAuth = makeAuthValue({ user: USER_AUTH });

function renderPage(route = '/tickets', auth = adminAuth) {
  return renderWithProviders(<TicketsPage />, { route, auth });
}

function columnCards(status: string) {
  return within(screen.getByTestId(`ticket-column-list-${status}`)).queryAllByTestId(
    /^ticket-card-/,
  );
}

describe('TicketsPage: доска ADMIN', () => {
  it('показывает 4 колонки и серверные счётчики по всем зданиям', async () => {
    authenticateAs(1);
    renderPage();

    await screen.findByTestId('ticket-card-1');

    expect(screen.getByTestId('ticket-column-new')).toBeInTheDocument();
    expect(screen.getByTestId('ticket-column-in_progress')).toBeInTheDocument();
    expect(screen.getByTestId('ticket-column-deferred')).toBeInTheDocument();
    expect(screen.getByTestId('ticket-column-done')).toBeInTheDocument();

    await waitFor(() => expect(columnCards('new')).toHaveLength(4));
    await waitFor(() => expect(columnCards('in_progress')).toHaveLength(3));
    await waitFor(() => expect(columnCards('deferred')).toHaveLength(2));
    await waitFor(() => expect(columnCards('done')).toHaveLength(3));

    expect(screen.getByTestId('ticket-column-count-new')).toHaveTextContent('4');
    expect(screen.getByTestId('ticket-column-count-in_progress')).toHaveTextContent('3');
    expect(screen.getByTestId('ticket-column-count-deferred')).toHaveTextContent('2');
    expect(screen.getByTestId('ticket-column-count-done')).toHaveTextContent('3');
  });

  it('фильтр здания доступен ADMIN и по умолчанию «Все здания»', async () => {
    authenticateAs(1);
    renderPage();
    await screen.findByTestId('ticket-card-1');

    expect(screen.getByTestId('building-select').closest('.ant-select')).not.toHaveClass(
      'ant-select-disabled',
    );
    expect(screen.getByText(/ADMIN видит все здания/)).toBeInTheDocument();
  });

  it('показывает просрочку только по серверному флагу is_overdue', async () => {
    authenticateAs(1);
    renderPage();

    const overdueCard = await screen.findByTestId('ticket-card-2');
    expect(overdueCard).toHaveAttribute('data-overdue', 'true');
    expect(overdueCard).toHaveTextContent('Просрочено');

    // Заявка 1 не просрочена (дедлайн ещё не наступил) — метки нет.
    expect(screen.getByTestId('ticket-card-1')).toHaveAttribute('data-overdue', 'false');
    expect(screen.getByTestId('ticket-card-1')).not.toHaveTextContent('Просрочено');
  });
});

describe('TicketsPage: права USER (TICKET-7)', () => {
  it('видит только заявки своего здания, а фильтр здания заблокирован', async () => {
    authenticateAs(2);
    renderPage('/tickets', userAuth);

    await screen.findByTestId('ticket-card-1');

    expect(columnCards('new')).toHaveLength(1);
    expect(columnCards('in_progress')).toHaveLength(1);
    expect(columnCards('deferred')).toHaveLength(1);
    expect(columnCards('done')).toHaveLength(2);

    // Заявки других зданий отсутствуют.
    expect(screen.queryByTestId('ticket-card-5')).not.toBeInTheDocument();
    expect(screen.queryByTestId('ticket-card-9')).not.toBeInTheDocument();

    expect(screen.getByTestId('building-select').closest('.ant-select')).toHaveClass(
      'ant-select-disabled',
    );
    expect(screen.getByText(/USER видит только заявки своего здания/)).toBeInTheDocument();
  });

  it('сотрудник без здания видит баннер AUTH-9 и не грузит доску', async () => {
    const noBuilding = makeAuthUser({ building_id: null, building_name: null });
    authenticateAs(noBuilding.id);
    renderPage('/tickets', makeAuthValue({ user: noBuilding }));

    expect(await screen.findByTestId('building-not-assigned')).toHaveTextContent(
      'Здание не назначено',
    );
    expect(screen.queryByTestId('tickets-board')).not.toBeInTheDocument();
  });
});

describe('TicketsPage: фильтры в URL', () => {
  it('поиск по заявителю ограничивает карточки', async () => {
    authenticateAs(1);
    renderPage('/tickets?search=Сидоров');

    expect(await screen.findByTestId('ticket-card-2')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByTestId('ticket-card-1')).not.toBeInTheDocument());
    expect(columnCards('in_progress')).toHaveLength(1);
  });

  it('фильтр «без исполнителя» (executor_id=none) работает через URL', async () => {
    authenticateAs(1);
    renderPage('/tickets?executor_id=none');

    await screen.findByTestId('ticket-card-1');

    // Без исполнителя: 1, 5, 10, 11 (new), 9 (in_progress).
    await waitFor(() => expect(columnCards('new')).toHaveLength(4));
    expect(columnCards('in_progress')).toHaveLength(1);
    expect(columnCards('deferred')).toHaveLength(0);
    expect(columnCards('done')).toHaveLength(0);

    await waitFor(() => expect(screen.queryByTestId('ticket-card-2')).not.toBeInTheDocument());
    expect(screen.getByTestId('ticket-card-9')).toBeInTheDocument();
    expect(screen.getByTestId('ticket-card-5')).toBeInTheDocument();
  });

  it('фильтр по статусу скрывает другие колонки', async () => {
    authenticateAs(1);
    renderPage('/tickets?status=deferred');

    await screen.findByTestId('ticket-card-3');

    await waitFor(() => expect(columnCards('deferred')).toHaveLength(2));
    expect(columnCards('new')).toHaveLength(0);
    expect(columnCards('in_progress')).toHaveLength(0);
    expect(columnCards('done')).toHaveLength(0);
    expect(screen.getAllByText('Статус не выбран')).toHaveLength(3);
    expect(screen.getByTestId('filters-reset')).toBeEnabled();
  });
});
