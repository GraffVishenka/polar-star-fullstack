/** Статистика (§3.8): личная `/stats/me`, сводная `/admin/stats`, Excel. */
import { apiRequest } from './client';
import type { AdminStatsParams, Page, StatsParams, StatsResponse, Ticket } from './types';

/** STATS-1/STATS-4: личная статистика доступна всем авторизованным. */
export function fetchMyStats(params: StatsParams = {}): Promise<StatsResponse> {
  return apiRequest<StatsResponse>('/stats/me', { query: params });
}

/** STATS-1: сводная статистика — только ADMIN. */
export function fetchAdminStats(params: AdminStatsParams = {}): Promise<StatsResponse> {
  return apiRequest<StatsResponse>('/admin/stats', { query: params });
}

/** STATS-3: путь Excel-выгрузки (скачивание — через apiDownload). */
export function statsExportPath(params: AdminStatsParams = {}): string {
  const search = new URLSearchParams();
  if (params.user_id) search.set('user_id', String(params.user_id));
  if (params.building_id) search.set('building_id', String(params.building_id));
  if (params.from) search.set('from', params.from);
  if (params.to) search.set('to', params.to);
  const query = search.toString();
  return `/stats/export.xlsx${query ? `?${query}` : ''}`;
}

export type { Page, StatsResponse, Ticket };
