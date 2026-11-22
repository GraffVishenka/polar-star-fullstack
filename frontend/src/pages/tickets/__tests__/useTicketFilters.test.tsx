/** Фильтры доски: URL ↔ состояние, параметры запроса, блокировка здания у USER. */
import { act, renderHook } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { describe, expect, it } from 'vitest';
import { AuthContext, type AuthContextValue } from '@/auth/AuthContext';
import { createTestQueryClient, makeAuthUser, makeAuthValue } from '@/test/utils';
import {
  DEFAULT_FILTERS,
  isFiltered,
  parseTicketFilters,
  serializeTicketFilters,
  toApiTicketParams,
  useTicketFilters,
} from '../hooks/useTicketFilters';

const userAuth = makeAuthValue();
const adminAuth = makeAuthValue({
  user: makeAuthUser({ id: 1, role: 'ADMIN', building_id: null, building_name: null }),
});

function renderFilters(route: string, auth: AuthContextValue) {
  const client = createTestQueryClient();
  return renderHook(() => useTicketFilters(), {
    wrapper: ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>
        <AuthContext.Provider value={auth}>
          <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>
        </AuthContext.Provider>
      </QueryClientProvider>
    ),
  });
}

describe('parseTicketFilters', () => {
  it('по умолчанию у ADMIN «Все здания», у USER — своё', () => {
    expect(
      parseTicketFilters(new URLSearchParams(), { isAdmin: true, userBuildingId: null }),
    ).toEqual(DEFAULT_FILTERS);
    expect(
      parseTicketFilters(new URLSearchParams(), { isAdmin: false, userBuildingId: 1 }).buildingId,
    ).toBe(1);
  });

  it('читает статусы, исполнителя, даты, сортировку и поиск', () => {
    const params = new URLSearchParams(
      'status=new&status=done&executor_id=none&problem_type_id=2&date_from=2026-03-01&date_to=2026-03-31&search=Иван&sort_by=deadline&sort_dir=asc',
    );
    const filters = parseTicketFilters(params, { isAdmin: true, userBuildingId: null });

    expect(filters.statuses).toEqual(['new', 'done']);
    expect(filters.executor).toBe('none');
    expect(filters.problemTypeId).toBe(2);
    expect(filters.dateFrom).toBe('2026-03-01');
    expect(filters.dateTo).toBe('2026-03-31');
    expect(filters.search).toBe('Иван');
    expect(filters.sortBy).toBe('deadline');
    expect(filters.sortDir).toBe('asc');
  });

  it('игнорирует мусор в ссылке и не ломает страницу', () => {
    const params = new URLSearchParams(
      'status=broken&executor_id=abc&date_from=01.03.2026&building_id=NaN',
    );
    const filters = parseTicketFilters(params, { isAdmin: true, userBuildingId: null });

    expect(filters.statuses).toEqual([]);
    expect(filters.executor).toBe('all');
    expect(filters.dateFrom).toBeNull();
    expect(filters.buildingId).toBe('all');
  });

  it('USER не может переключить здание через ссылку (TICKET-7)', () => {
    const filters = parseTicketFilters(new URLSearchParams('building_id=2'), {
      isAdmin: false,
      userBuildingId: 1,
    });
    expect(filters.buildingId).toBe(1);
  });

  it('сериализация не пишет пустые значения', () => {
    const params = serializeTicketFilters(DEFAULT_FILTERS);
    expect(params.toString()).toBe('');

    const withFilters = serializeTicketFilters({
      ...DEFAULT_FILTERS,
      statuses: ['new'],
      executor: 'me',
      search: 'каб',
      dateFrom: '2026-03-01',
    });
    expect(withFilters.getAll('status')).toEqual(['new']);
    expect(withFilters.get('executor_id')).toBe('me');
    expect(withFilters.get('search')).toBe('каб');
    expect(withFilters.get('date_from')).toBe('2026-03-01');
  });
});

describe('toApiTicketParams', () => {
  it('передаёт «без исполнителя» как null (сентинел none в URL)', () => {
    const params = toApiTicketParams(
      { ...DEFAULT_FILTERS, executor: 'none' },
      { isAdmin: true, userId: 1 },
    );
    expect(params.executor_id).toBeNull();
  });

  it('«только мои» превращается в id текущего пользователя', () => {
    const params = toApiTicketParams(
      { ...DEFAULT_FILTERS, executor: 'me' },
      { isAdmin: false, userId: 2 },
    );
    expect(params.executor_id).toBe(2);
  });

  it('USER не отправляет building_id — сервер режет по своему зданию', () => {
    const params = toApiTicketParams(
      { ...DEFAULT_FILTERS, buildingId: 2 },
      { isAdmin: false, userId: 2 },
    );
    expect(params.building_id).toBeUndefined();
  });

  it('ADMIN отправляет выбранное здание, «Все здания» — без параметра', () => {
    expect(
      toApiTicketParams({ ...DEFAULT_FILTERS, buildingId: 3 }, { isAdmin: true, userId: 1 })
        .building_id,
    ).toBe(3);
    expect(
      toApiTicketParams({ ...DEFAULT_FILTERS, buildingId: 'all' }, { isAdmin: true, userId: 1 })
        .building_id,
    ).toBe('all');
  });
});

describe('useTicketFilters', () => {
  it('меняет фильтр и пишет его в URL', () => {
    const { result } = renderFilters('/tickets', userAuth);

    act(() => {
      result.current.setFilter({ search: 'каб. 204', statuses: ['in_progress'] });
    });

    expect(result.current.filters.search).toBe('каб. 204');
    expect(result.current.filters.statuses).toEqual(['in_progress']);
    expect(result.current.apiParams.search).toBe('каб. 204');
    expect(result.current.hasActiveFilters).toBe(true);
  });

  it('сброс возвращает фильтры по умолчанию', () => {
    const { result } = renderFilters('/tickets?search=test&status=done', userAuth);

    expect(result.current.hasActiveFilters).toBe(true);
    act(() => result.current.resetFilters());

    expect(result.current.filters.search).toBe('');
    expect(result.current.filters.statuses).toEqual([]);
    expect(result.current.hasActiveFilters).toBe(false);
    expect(isFiltered(result.current.filters, false)).toBe(false);
  });

  it('у ADMIN по умолчанию «Все здания» и здание меняется', () => {
    const { result } = renderFilters('/tickets', adminAuth);
    expect(result.current.filters.buildingId).toBe('all');

    act(() => result.current.setFilter({ buildingId: 2 }));
    expect(result.current.filters.buildingId).toBe(2);
    expect(result.current.apiParams.building_id).toBe(2);
  });
});
