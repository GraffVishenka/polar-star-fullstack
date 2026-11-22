/** MSW: статистика (§3.8, §3.13). */
import { http } from 'msw';
import { guard } from '../auth';
import { computeStats } from '../db';
import { API, fileResponse, forbidden, json, validation } from '../http';

function periodFrom(request: Request): {
  from?: string;
  to?: string;
  userId?: number;
  buildingId?: number;
} {
  const url = new URL(request.url);
  const userId = Number(url.searchParams.get('user_id'));
  const buildingId = Number(url.searchParams.get('building_id'));
  return {
    from: url.searchParams.get('from') ?? undefined,
    to: url.searchParams.get('to') ?? undefined,
    userId: Number.isSafeInteger(userId) && userId > 0 ? userId : undefined,
    buildingId: Number.isSafeInteger(buildingId) && buildingId > 0 ? buildingId : undefined,
  };
}

export const statsHandlers = [
  // STATS-1: личная статистика — всем авторизованным, без ограничения здания.
  http.get(`${API}/stats/me`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const period = periodFrom(request);
    return json(computeStats(auth.user.id, null, period.from, period.to));
  }),

  // STATS-3: Excel — свой отчёт всем, чужой/по зданию только ADMIN.
  http.get(`${API}/stats/export.xlsx`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const period = periodFrom(request);

    if (auth.user.role !== 'ADMIN' && (period.userId ?? auth.user.id) !== auth.user.id) {
      return forbidden('Чужой отчёт доступен только администратору');
    }
    if (auth.user.role !== 'ADMIN' && period.buildingId) {
      return forbidden('Фильтр по зданию доступен только администратору');
    }
    // STATS-3: период по `completed_at`; обратный диапазон — ошибка валидации.
    if (period.from && period.to && period.from > period.to) {
      return validation('Дата начала периода позже даты окончания', {
        from: 'должна быть <= to',
      });
    }

    const stats = computeStats(
      period.userId ?? (auth.user.role === 'ADMIN' ? null : auth.user.id),
      period.buildingId ?? null,
      period.from,
      period.to,
    );
    const body = [
      'Сводка (демо MSW)',
      `Сотрудник;${stats.user_name}`,
      `Период;${stats.from || '—'};${stats.to || '—'}`,
      `Завершено всего;${stats.total_done}`,
      `Завершено за период;${stats.done_in_period}`,
      `Среднее время, ч;${stats.avg_resolution_hours ?? '—'}`,
      `Медиана, ч;${stats.median_resolution_hours ?? '—'}`,
      `В срок, %;${stats.on_time_percent ?? '—'}`,
      `Просрочено;${stats.overdue_count}`,
    ].join('\n');

    const login = auth.user.login;
    return fileResponse(
      body,
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      `stat_${login}_${stats.from || 'all'}_${stats.to || 'all'}.xlsx`,
    );
  }),
];

export { periodFrom };
