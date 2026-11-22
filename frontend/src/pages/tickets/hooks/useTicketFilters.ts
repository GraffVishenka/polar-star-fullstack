/**
 * Фильтры доски заявок: состояние в URL (TICKET-15, Ф2).
 *
 * В URL храним только непустые значения — ссылку можно переслать коллеге,
 * а F5/назад/вперёд восстанавливают ровно тот же вид доски.
 * Пагинация колонок — локальная (кнопка «Показать ещё»), в URL не пишется.
 */
import { useCallback, useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { TICKET_STATUS_ORDER } from '@/app/config';
import type { TicketListParams, TicketStatus } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';

/** Исполнитель: все / без исполнителя / только мои / конкретный сотрудник. */
export type ExecutorFilter = 'all' | 'none' | 'me' | number;

export type TicketSortBy = 'created_at' | 'deadline';
export type TicketSortDir = 'asc' | 'desc';

export interface TicketFilterState {
  statuses: TicketStatus[];
  executor: ExecutorFilter;
  problemTypeId: number | null;
  buildingId: number | 'all';
  dateFrom: string | null;
  dateTo: string | null;
  search: string;
  sortBy: TicketSortBy;
  sortDir: TicketSortDir;
}

export const DEFAULT_FILTERS: TicketFilterState = {
  statuses: [],
  executor: 'all',
  problemTypeId: null,
  buildingId: 'all',
  dateFrom: null,
  dateTo: null,
  search: '',
  sortBy: 'created_at',
  sortDir: 'desc',
};

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

function positiveInt(value: string | null): number | null {
  if (value === null) return null;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
}

function validStatus(value: string): value is TicketStatus {
  return (TICKET_STATUS_ORDER as string[]).includes(value);
}

/** Разбор URL → фильтры. Невалидные значения молча игнорируются (ссылка не ломает страницу). */
export function parseTicketFilters(
  params: URLSearchParams,
  options: { isAdmin: boolean; userBuildingId: number | null },
): TicketFilterState {
  const statuses = [...new Set(params.getAll('status').filter(validStatus))];

  const executorRaw = params.get('executor_id');
  let executor: ExecutorFilter = 'all';
  if (executorRaw === 'none' || executorRaw === 'me') executor = executorRaw;
  else if (executorRaw !== null) {
    const id = positiveInt(executorRaw);
    if (id !== null) executor = id;
  }

  const buildingRaw = params.get('building_id');
  const userBuilding = options.userBuildingId;
  let buildingId: number | 'all' = options.isAdmin ? 'all' : (userBuilding ?? 'all');
  if (options.isAdmin) {
    if (buildingRaw === 'all') buildingId = 'all';
    else {
      const id = positiveInt(buildingRaw);
      if (id !== null) buildingId = id;
    }
  }

  const problemTypeId = positiveInt(params.get('problem_type_id'));
  const dateFromRaw = params.get('date_from');
  const dateToRaw = params.get('date_to');
  const sortBy = params.get('sort_by') === 'deadline' ? 'deadline' : 'created_at';
  const sortDir = params.get('sort_dir') === 'asc' ? 'asc' : 'desc';

  return {
    statuses,
    executor,
    problemTypeId,
    buildingId,
    dateFrom: dateFromRaw && DATE_RE.test(dateFromRaw) ? dateFromRaw : null,
    dateTo: dateToRaw && DATE_RE.test(dateToRaw) ? dateToRaw : null,
    search: (params.get('search') ?? '').trim(),
    sortBy,
    sortDir,
  };
}

/** Фильтры → URL (только непустые значения). */
export function serializeTicketFilters(filters: TicketFilterState): URLSearchParams {
  const params = new URLSearchParams();
  filters.statuses.forEach((status) => params.append('status', status));
  if (filters.executor === 'none' || filters.executor === 'me') {
    params.set('executor_id', filters.executor);
  } else if (typeof filters.executor === 'number') {
    params.set('executor_id', String(filters.executor));
  }
  if (filters.problemTypeId !== null) params.set('problem_type_id', String(filters.problemTypeId));
  if (filters.buildingId !== 'all') params.set('building_id', String(filters.buildingId));
  if (filters.dateFrom) params.set('date_from', filters.dateFrom);
  if (filters.dateTo) params.set('date_to', filters.dateTo);
  if (filters.search) params.set('search', filters.search);
  if (filters.sortBy !== DEFAULT_FILTERS.sortBy) params.set('sort_by', filters.sortBy);
  if (filters.sortDir !== DEFAULT_FILTERS.sortDir) params.set('sort_dir', filters.sortDir);
  return params;
}

/** Параметры запроса к API; у USER здание не передаётся — сервер и так режет по своему (TICKET-7). */
export function toApiTicketParams(
  filters: TicketFilterState,
  options: { isAdmin: boolean; userId: number | null },
): TicketListParams {
  let executorId: number | null | undefined;
  if (filters.executor === 'none') executorId = null;
  else if (filters.executor === 'me') executorId = options.userId ?? undefined;
  else if (typeof filters.executor === 'number') executorId = filters.executor;

  return {
    status: filters.statuses.length ? filters.statuses : undefined,
    executor_id: executorId,
    problem_type_id: filters.problemTypeId ?? undefined,
    building_id: options.isAdmin ? filters.buildingId : undefined,
    date_from: filters.dateFrom ?? undefined,
    date_to: filters.dateTo ?? undefined,
    search: filters.search || undefined,
    sort_by: filters.sortBy,
    sort_dir: filters.sortDir,
  };
}

export function isFiltered(filters: TicketFilterState, isAdmin: boolean): boolean {
  return (
    filters.statuses.length > 0 ||
    filters.executor !== 'all' ||
    filters.problemTypeId !== null ||
    (isAdmin && filters.buildingId !== 'all') ||
    filters.dateFrom !== null ||
    filters.dateTo !== null ||
    filters.search.length > 0 ||
    filters.sortBy !== DEFAULT_FILTERS.sortBy ||
    filters.sortDir !== DEFAULT_FILTERS.sortDir
  );
}

export interface UseTicketFiltersResult {
  filters: TicketFilterState;
  apiParams: TicketListParams;
  isAdmin: boolean;
  hasActiveFilters: boolean;
  setFilter: (patch: Partial<TicketFilterState>) => void;
  resetFilters: () => void;
}

export function useTicketFilters(): UseTicketFiltersResult {
  const { isAdmin, user } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();

  const filters = useMemo(
    () =>
      parseTicketFilters(searchParams, {
        isAdmin,
        userBuildingId: user?.building_id ?? null,
      }),
    // searchParams — источник истины; строка стабильна между рендерами.
    [isAdmin, searchParams, user?.building_id],
  );

  const setFilter = useCallback(
    (patch: Partial<TicketFilterState>) => {
      const next: TicketFilterState = { ...filters, ...patch };
      // USER всегда видит своё здание — даже если в ссылке чужое.
      if (!isAdmin) next.buildingId = user?.building_id ?? 'all';
      setSearchParams(serializeTicketFilters(next), { replace: true });
    },
    [filters, isAdmin, setSearchParams, user?.building_id],
  );

  const resetFilters = useCallback(() => {
    setSearchParams(new URLSearchParams(), { replace: true });
  }, [setSearchParams]);

  const apiParams = useMemo(
    () => toApiTicketParams(filters, { isAdmin, userId: user?.id ?? null }),
    [filters, isAdmin, user?.id],
  );

  return {
    filters,
    apiParams,
    isAdmin,
    hasActiveFilters: isFiltered(filters, isAdmin),
    setFilter,
    resetFilters,
  };
}
