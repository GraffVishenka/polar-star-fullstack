/**
 * Данные одной колонки доски: бесконечная серверная пагинация (TICKET-15).
 *
 * Каждая колонка — отдельный запрос со своим статусом: так счётчик берётся из
 * серверного `total`, а «Показать ещё» догружает следующую страницу.
 */
import { useMemo } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { fetchTickets } from '@/api/tickets';
import type { Ticket, TicketListParams, TicketStatus } from '@/api/types';

export const COLUMN_SIZE = 20;

/** Ключ колонки: `['tickets','board',<фильтры>,<статус>]` (индекс 3 — статус). */
export function boardQueryKey(filtersKey: string, status: TicketStatus) {
  return ['tickets', 'board', filtersKey, status] as const;
}

export const BOARD_QUERY_PREFIX = ['tickets', 'board'] as const;

export interface TicketColumnData {
  tickets: Ticket[];
  total: number;
  isLoading: boolean;
  isFetching: boolean;
  isError: boolean;
  error: unknown;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
  refetch: () => void;
}

function dedupe(tickets: Ticket[]): Ticket[] {
  const seen = new Set<number>();
  return tickets.filter((ticket) => {
    if (seen.has(ticket.id)) return false;
    seen.add(ticket.id);
    return true;
  });
}

export function useTicketColumn(
  status: TicketStatus,
  apiParams: TicketListParams,
  filtersKey: string,
  enabled = true,
): TicketColumnData {
  const query = useInfiniteQuery({
    queryKey: boardQueryKey(filtersKey, status),
    queryFn: ({ pageParam }) =>
      fetchTickets({
        ...apiParams,
        status: [status],
        page: pageParam,
        size: COLUMN_SIZE,
      }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.size < lastPage.total ? lastPage.page + 1 : undefined,
    staleTime: 15_000,
    enabled,
  });

  const tickets = useMemo(
    () => dedupe(query.data?.pages.flatMap((page) => page.items) ?? []),
    [query.data],
  );

  return {
    tickets,
    total: query.data?.pages[0]?.total ?? 0,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    isError: query.isError,
    error: query.error,
    hasNextPage: query.hasNextPage,
    isFetchingNextPage: query.isFetchingNextPage,
    fetchNextPage: () => void query.fetchNextPage(),
    refetch: () => void query.refetch(),
  };
}
