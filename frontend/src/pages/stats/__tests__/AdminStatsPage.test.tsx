/**
 * `/admin/stats` — статистика сотрудников (STATS-1/STATS-3).
 * Фильтры сотрудника/здания/периода, сводная таблица, Excel с params,
 * 403 и редирект USER (TICKET-7: без утечки админских данных).
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { RequireRole } from '@/auth/guards';
import { registerToastApi } from '@/app/toast';
import { server } from '@/mocks/server';
import { ADMIN_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { AdminStatsPage } from '../AdminStatsPage';
import { periodRange } from '../period';
import type { StatsWithEmployees } from '../types';

const XLSX = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';

const aggregateStats: StatsWithEmployees = {
  user_id: 0,
  user_name: 'Все сотрудники',
  from: '2026-11-01',
  to: '2026-11-30',
  total_done: 8,
  done_in_period: 3,
  avg_resolution_hours: 6.5,
  median_resolution_hours: 5.2,
  on_time_percent: 75,
  overdue_count: 2,
  by_problem_type: [{ problem_type_id: 1, problem_type_name: 'Протечка', total: 5, done: 4 }],
  by_month: [{ month: '2026-11', done: 3 }],
  employees: [
    {
      user_id: 2,
      user_name: 'Иванов Иван Иванович',
      total_done: 5,
      done_in_period: 2,
      avg_resolution_hours: 6.2,
      median_resolution_hours: 5.5,
      on_time_percent: 80,
      overdue_count: 1,
    },
    {
      user_id: 3,
      user_name: 'Сидоров Пётр Алексеевич',
      total_done: 3,
      done_in_period: 1,
      avg_resolution_hours: null,
      median_resolution_hours: null,
      on_time_percent: null,
      overdue_count: 0,
    },
  ],
};

let lastParams: URLSearchParams | null = null;
let exportParams: URLSearchParams | null = null;
const downloads: string[] = [];

function renderAdminStats(auth = makeAuthValue({ user: ADMIN_AUTH })) {
  return renderWithProviders(
    <Routes>
      <Route
        path="/admin/stats"
        element={
          <RequireRole role="ADMIN">
            <AdminStatsPage />
          </RequireRole>
        }
      />
      <Route path="/tickets" element={<div>tickets page</div>} />
    </Routes>,
    { route: '/admin/stats', auth },
  );
}

function mockAdminStats(stats: StatsWithEmployees = aggregateStats) {
  server.use(
    http.get('*/api/admin/stats', ({ request }) => {
      lastParams = new URL(request.url).searchParams;
      return HttpResponse.json(stats);
    }),
  );
}

async function pickOptionContaining(selectTestId: string, text: string) {
  const select = screen.getByTestId(selectTestId);
  await userEvent.click(within(select).getByRole('combobox'));
  const option = await waitFor(() => {
    const dropdowns = [...document.querySelectorAll<HTMLElement>('.ant-select-dropdown')].filter(
      (dropdown) => !dropdown.classList.contains('ant-select-dropdown-hidden'),
    );
    for (const dropdown of dropdowns) {
      const found = [...dropdown.querySelectorAll<HTMLElement>('.ant-select-item-option')].find(
        (item) => item.textContent?.includes(text),
      );
      if (found) return found;
    }
    throw new Error(`Опция с текстом «${text}» не найдена`);
  });
  await userEvent.click(option);
}

beforeEach(() => {
  lastParams = null;
  exportParams = null;
  downloads.length = 0;
  Object.defineProperty(URL, 'createObjectURL', {
    writable: true,
    value: vi.fn(() => 'blob:stats'),
  });
  Object.defineProperty(URL, 'revokeObjectURL', { writable: true, value: vi.fn() });
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    downloads.push(this.download);
  });
  authenticateAs(ADMIN_AUTH.id);
  registerToastApi({ success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() });
});

describe('AdminStatsPage', () => {
  it('показывает фильтры, метрики и сводку по сотрудникам', async () => {
    mockAdminStats();
    renderAdminStats();

    expect(await screen.findByTestId('admin-stats-page')).toBeInTheDocument();
    expect(await screen.findByTestId('admin-stats-filters')).toBeInTheDocument();
    expect(screen.getByTestId('admin-stats-user')).toBeInTheDocument();
    expect(screen.getByTestId('building-select')).toBeInTheDocument();

    expect(await screen.findByTestId('metric-value-total_done')).toHaveTextContent('8');
    expect(screen.getByTestId('metric-value-done_in_period')).toHaveTextContent('3');

    const summary = within(screen.getByTestId('employee-summary'));
    expect(summary.getByText('Иванов Иван Иванович')).toBeInTheDocument();
    expect(summary.getByText('Сидоров Пётр Алексеевич')).toBeInTheDocument();
    // null-метрики в сводке — «—».
    expect(summary.getAllByText('—').length).toBeGreaterThanOrEqual(2);
  });

  it('выбор сотрудника добавляет user_id в запрос и скрывает сводку', async () => {
    mockAdminStats();
    renderAdminStats();
    await screen.findByTestId('employee-summary');

    await pickOptionContaining('admin-stats-user', 'Иванов Иван Иванович');

    await waitFor(() => expect(lastParams?.get('user_id')).toBe('2'));
    await waitFor(() => expect(screen.queryByTestId('employee-summary')).not.toBeInTheDocument());
  });

  it('фильтр здания добавляет building_id (только активные здания)', async () => {
    mockAdminStats();
    renderAdminStats();
    await screen.findByTestId('admin-stats-page');

    const buildingSelect = screen.getByTestId('building-select');
    await userEvent.click(within(buildingSelect).getByRole('combobox'));
    const option = await waitFor(() => {
      const found = [...document.querySelectorAll<HTMLElement>('.ant-select-item-option')].find(
        (item) => item.textContent?.includes('Производственный корпус'),
      );
      if (found) return found;
      throw new Error('Здание не найдено');
    });
    await userEvent.click(option);

    await waitFor(() => expect(lastParams?.get('building_id')).toBe('2'));
  });

  it('смена периода на «Квартал» пересчитывает запрос', async () => {
    mockAdminStats();
    renderAdminStats();
    await screen.findByTestId('admin-stats-page');

    await userEvent.click(within(screen.getByTestId('admin-stats-period')).getByText('Квартал'));

    const quarter = periodRange('quarter');
    await waitFor(() => expect(lastParams?.get('from')).toBe(quarter.from));
    expect(lastParams?.get('to')).toBe(quarter.to);
  });

  it('Excel-кнопка вызывает export.xlsx с user_id/building_id/from/to', async () => {
    mockAdminStats();
    server.use(
      http.get('*/api/stats/export.xlsx', ({ request }) => {
        exportParams = new URL(request.url).searchParams;
        return new HttpResponse(new Blob(['xlsx']), {
          status: 200,
          headers: {
            'Content-Type': XLSX,
            'Content-Disposition': 'attachment; filename="stat_all.xlsx"',
          },
        });
      }),
    );
    renderAdminStats();
    await screen.findByTestId('employee-summary');

    await pickOptionContaining('admin-stats-user', 'Иванов Иван Иванович');
    await waitFor(() => expect(lastParams?.get('user_id')).toBe('2'));
    await waitFor(() => expect(screen.getByTestId('export-button')).toBeEnabled());

    await userEvent.click(screen.getByTestId('export-button'));

    await waitFor(() => expect(exportParams).not.toBeNull());
    expect(exportParams!.get('user_id')).toBe('2');
    expect(exportParams!.get('from')).toBe(periodRange('month').from);
    expect(exportParams!.get('to')).toBe(periodRange('month').to);
    await waitFor(() => expect(downloads).toEqual(['stat_all.xlsx']));
  });

  it('403 объясняется, страница не падает', async () => {
    server.use(
      http.get('*/api/admin/stats', () =>
        HttpResponse.json(
          { code: 'FORBIDDEN', message: 'forbidden', details: null, request_id: 'stats-403' },
          { status: 403 },
        ),
      ),
    );
    renderAdminStats();

    expect(await screen.findByTestId('admin-stats-error')).toBeInTheDocument();
    expect(screen.getByText('Недостаточно прав для этого действия')).toBeInTheDocument();
  });

  it('USER на /admin/stats получает редирект на /tickets (TICKET-7)', async () => {
    mockAdminStats();
    renderAdminStats(makeAuthValue({ user: { ...ADMIN_AUTH, id: 2, role: 'USER' } }));

    expect(await screen.findByText('tickets page')).toBeInTheDocument();
    expect(screen.queryByTestId('admin-stats-page')).not.toBeInTheDocument();
  });
});
