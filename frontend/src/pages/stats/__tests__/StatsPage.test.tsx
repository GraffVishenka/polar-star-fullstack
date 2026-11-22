/**
 * `/stats` — личная статистика (STATS-1/STATS-2/STATS-4).
 * Проверяем метрики, null → «—», пустое состояние, смену периода и ошибки.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { StatsResponse } from '@/api/types';
import { registerToastApi } from '@/app/toast';
import { server } from '@/mocks/server';
import { USER_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { StatsPage } from '../StatsPage';
import { periodRange } from '../period';

const baseStats: StatsResponse = {
  user_id: 2,
  user_name: 'Иванов Иван Иванович',
  from: '2026-11-01',
  to: '2026-11-30',
  total_done: 12,
  done_in_period: 5,
  avg_resolution_hours: 8.4,
  median_resolution_hours: 7.5,
  on_time_percent: 90,
  overdue_count: 2,
  by_problem_type: [
    { problem_type_id: 1, problem_type_name: 'Протечка', total: 8, done: 6 },
    { problem_type_id: 2, problem_type_name: 'Электрика', total: 4, done: 3 },
  ],
  by_month: [
    { month: '2026-10', done: 7 },
    { month: '2026-11', done: 5 },
  ],
};

let lastParams: URLSearchParams | null = null;

function renderStatsPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/stats" element={<StatsPage />} />
    </Routes>,
    { route: '/stats', auth: makeAuthValue({ user: USER_AUTH }) },
  );
}

function mockStats(stats: StatsResponse) {
  server.use(
    http.get('*/api/stats/me', ({ request }) => {
      lastParams = new URL(request.url).searchParams;
      return HttpResponse.json(stats);
    }),
  );
}

beforeEach(() => {
  lastParams = null;
  authenticateAs(USER_AUTH.id);
  registerToastApi({ success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() });
});

describe('StatsPage', () => {
  it('показывает метрики, разбивку по видам проблем и месяцам', async () => {
    mockStats(baseStats);
    renderStatsPage();

    expect(await screen.findByTestId('stats-page')).toBeInTheDocument();
    expect(await screen.findByTestId('metric-value-total_done')).toHaveTextContent('12');
    expect(screen.getByTestId('metric-value-done_in_period')).toHaveTextContent('5');
    expect(screen.getByTestId('metric-value-avg_resolution_hours')).toHaveTextContent('8,4 ч');
    expect(screen.getByTestId('metric-value-median_resolution_hours')).toHaveTextContent('7,5 ч');
    expect(screen.getByTestId('metric-value-on_time_percent')).toHaveTextContent('90 %');
    expect(screen.getByTestId('metric-value-overdue_count')).toHaveTextContent('2');

    const problems = within(screen.getByTestId('breakdown-problem-types'));
    expect(problems.getByText('Протечка')).toBeInTheDocument();
    expect(problems.getByText('Электрика')).toBeInTheDocument();

    const months = within(screen.getByTestId('breakdown-months'));
    expect(months.getByText('октябрь 2026')).toBeInTheDocument();
    expect(months.getByText('ноябрь 2026')).toBeInTheDocument();

    // Период по умолчанию — текущий месяц (STATS-4).
    const month = periodRange('month');
    await waitFor(() => expect(lastParams?.get('from')).toBe(month.from));
    expect(lastParams?.get('to')).toBe(month.to);
  });

  it('null-метрики показываются как «—»', async () => {
    mockStats({
      ...baseStats,
      avg_resolution_hours: null,
      median_resolution_hours: null,
      on_time_percent: null,
    });
    renderStatsPage();

    expect(await screen.findByTestId('metric-value-avg_resolution_hours')).toHaveTextContent('—');
    expect(screen.getByTestId('metric-value-median_resolution_hours')).toHaveTextContent('—');
    expect(screen.getByTestId('metric-value-on_time_percent')).toHaveTextContent('—');
  });

  it('показывает пустое состояние, если завершённых заявок нет', async () => {
    mockStats({
      ...baseStats,
      total_done: 0,
      done_in_period: 0,
      avg_resolution_hours: null,
      median_resolution_hours: null,
      on_time_percent: null,
      overdue_count: 0,
      by_problem_type: [],
      by_month: [],
    });
    renderStatsPage();

    expect(await screen.findByText('Нет завершённых заявок')).toBeInTheDocument();
    expect(screen.queryByTestId('breakdown-problem-types')).not.toBeInTheDocument();
    expect(screen.getByTestId('metric-value-total_done')).toHaveTextContent('0');
  });

  it('смена периода на «Квартал» пересчитывает запрос', async () => {
    mockStats(baseStats);
    renderStatsPage();
    await screen.findByTestId('stats-page');

    await userEvent.click(within(screen.getByTestId('stats-period')).getByText('Квартал'));

    const quarter = periodRange('quarter');
    await waitFor(() => expect(lastParams?.get('from')).toBe(quarter.from));
    expect(lastParams?.get('to')).toBe(quarter.to);
  });

  it('ошибка загрузки объясняется и повторяется по кнопке', async () => {
    let calls = 0;
    server.use(
      http.get('*/api/stats/me', () => {
        calls += 1;
        return HttpResponse.json(
          { code: 'INTERNAL_ERROR', message: 'boom', details: null, request_id: 'stats-500' },
          { status: 500 },
        );
      }),
    );

    renderStatsPage();

    expect(await screen.findByTestId('stats-error')).toBeInTheDocument();
    expect(screen.getByText('Ошибка сервера, попробуйте позже')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Повторить' }));
    await waitFor(() => expect(calls).toBeGreaterThanOrEqual(2));
  });
});
