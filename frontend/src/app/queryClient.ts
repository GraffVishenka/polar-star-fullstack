/** Клиент TanStack Query и общие ключи кэша (используются всеми разделами). */
import { QueryClient } from '@tanstack/react-query';
import { ApiError } from '@/api/errors';

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // 4xx повторять бессмысленно, 5xx/сеть — до двух раз.
        retry: (failureCount, error) => {
          if (error instanceof ApiError && error.isClientError) return false;
          return failureCount < 2;
        },
        staleTime: 30_000,
        refetchOnWindowFocus: false,
        gcTime: 5 * 60_000,
      },
      mutations: { retry: false },
    },
  });
}

export const queryClient = createQueryClient();

/** Единые ключи кэша: доменные агенты дополняют своими префиксами. */
export const queryKeys = {
  me: ['auth', 'me'] as const,
  publicBuildings: ['public', 'buildings'] as const,
  /** Один ключ на публичные настройки: `app_name` и статусы (PUB-1, Б7). */
  publicSettings: ['public', 'settings'] as const,
  publicTickets: (buildingId?: number | null) =>
    ['public', 'tickets', buildingId ?? 'all'] as const,
  publicTicket: (token: string) => ['public', 'ticket', token] as const,
  publicKbTree: ['public', 'kb', 'tree'] as const,
  publicKbNode: (id: number) => ['public', 'kb', 'node', id] as const,
  tickets: (params?: unknown) => ['tickets', params ?? {}] as const,
  ticket: (id: number) => ['tickets', 'item', id] as const,
  ticketComments: (id: number, page = 1) => ['tickets', 'comments', id, page] as const,
  kbTree: ['kb', 'tree'] as const,
  kbNode: (id: number) => ['kb', 'node', id] as const,
  buildings: ['buildings'] as const,
  warehouses: (buildingId?: number) => ['warehouses', buildingId ?? 'all'] as const,
  equipment: (params?: unknown) => ['equipment', params ?? {}] as const,
  equipmentItem: (id: number) => ['equipment', 'item', id] as const,
  equipmentMovements: (id: number) => ['equipment', 'movements', id] as const,
  settings: ['admin', 'settings'] as const,
  problemTypes: ['admin', 'problem-types'] as const,
  equipmentTypes: ['admin', 'equipment-types'] as const,
  holidays: ['admin', 'holidays'] as const,
  adminUsers: (params?: unknown) => ['admin', 'users', params ?? {}] as const,
  adminStats: (params?: unknown) => ['admin', 'stats', params ?? {}] as const,
  myStats: (params?: unknown) => ['stats', 'me', params ?? {}] as const,
  trash: (type: string, page = 1) => ['admin', 'trash', type, page] as const,
  assignableUsers: (ticketId?: number) => ['users', 'assignable', ticketId ?? 'any'] as const,
};
