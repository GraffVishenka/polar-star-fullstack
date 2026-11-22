/**
 * Оптимистичные обновления доски (Ф2).
 *
 * Смена статуса/исполнителя двигает карточку между колонками до ответа сервера.
 * При ошибке (403/404/422) вызывающий код откатывает снимок кэша — см.
 * `useUpdateTicket`.
 */
import type { InfiniteData, QueryClient, QueryKey } from '@tanstack/react-query';
import { queryKeys } from '@/app/queryClient';
import type { Page, Ticket, TicketStatus, TicketUpdateRequest } from '@/api/types';
import { BOARD_QUERY_PREFIX } from './useTicketColumn';

export type BoardData = InfiniteData<Page<Ticket>>;
export type TicketQueriesSnapshot = Array<[QueryKey, unknown]>;

/** Снимок всех кэшей заявок (доска + карточка + комментарии). */
export function snapshotTicketQueries(qc: QueryClient): TicketQueriesSnapshot {
  return qc.getQueriesData({ queryKey: ['tickets'] });
}

export function restoreTicketQueries(qc: QueryClient, snapshot: TicketQueriesSnapshot): void {
  snapshot.forEach(([key, data]) => qc.setQueryData(key, data));
}

function columnStatusFromKey(key: QueryKey): TicketStatus | null {
  const status = key[3];
  return typeof status === 'string' ? (status as TicketStatus) : null;
}

/** Ищет заявку в кэше карточки или в любой колонке доски. */
export function findCachedTicket(qc: QueryClient, id: number): Ticket | undefined {
  const detail = qc.getQueryData<Ticket>(queryKeys.ticket(id));
  if (detail) return detail;

  const boards = qc.getQueriesData<BoardData>({ queryKey: BOARD_QUERY_PREFIX });
  for (const [, data] of boards) {
    const found = data?.pages.flatMap((page) => page.items).find((ticket) => ticket.id === id);
    if (found) return found;
  }
  return undefined;
}

function withTotalDelta(data: BoardData, delta: number): BoardData {
  if (!data.pages.length) return data;
  return {
    ...data,
    pages: data.pages.map((page, index) =>
      index === 0 ? { ...page, total: Math.max(0, page.total + delta) } : page,
    ),
  };
}

function replaceInPages(data: BoardData, updated: Ticket): BoardData {
  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      items: page.items.map((ticket) => (ticket.id === updated.id ? updated : ticket)),
    })),
  };
}

function removeFromPages(data: BoardData, id: number): BoardData {
  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      items: page.items.filter((ticket) => ticket.id !== id),
    })),
  };
}

function prependToPages(data: BoardData, ticket: Ticket): BoardData {
  if (!data.pages.length) return data;
  return {
    ...data,
    pages: data.pages.map((page, index) =>
      index === 0 ? { ...page, items: [ticket, ...page.items] } : page,
    ),
  };
}

/**
 * Применяет оптимистичный патч к кэшу карточки и колонкам.
 * Возвращает обновлённую заявку (или null, если её нет в кэше).
 */
export function optimisticTicketPatch(
  qc: QueryClient,
  id: number,
  patch: TicketUpdateRequest,
): Ticket | null {
  const base = findCachedTicket(qc, id);
  if (!base) return null;

  const updated: Ticket = {
    ...base,
    ...patch,
    // Поля, которых нет в оптимистичном патче, остаются от серверных данных.
    executor_name:
      patch.executor_id === undefined
        ? base.executor_name
        : patch.executor_id === null
          ? null
          : base.executor_name,
  };

  qc.setQueryData<Ticket>(queryKeys.ticket(id), updated);

  const boards = qc.getQueriesData<BoardData>({ queryKey: BOARD_QUERY_PREFIX });
  for (const [key, data] of boards) {
    if (!data) continue;
    const column = columnStatusFromKey(key);
    if (!column) continue;

    const present = data.pages.some((page) => page.items.some((ticket) => ticket.id === id));
    const belongsToColumn = column === updated.status;

    if (belongsToColumn) {
      qc.setQueryData<BoardData>(
        key,
        present ? replaceInPages(data, updated) : withTotalDelta(prependToPages(data, updated), 1),
      );
    } else if (present) {
      qc.setQueryData<BoardData>(key, withTotalDelta(removeFromPages(data, id), -1));
    }
  }

  return updated;
}

/** Перезагружает список/колонки после успешной мутации. */
export function invalidateTicketQueries(qc: QueryClient): Promise<void> {
  return qc.invalidateQueries({ queryKey: ['tickets'] });
}
