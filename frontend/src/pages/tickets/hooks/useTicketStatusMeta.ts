/**
 * Названия и цвета статусов (TICKET-1).
 *
 * Источник истины — настройки Admin-панели, но `GET /api/admin/settings`
 * доступен только ADMIN. Поэтому:
 *  - ADMIN берёт `statuses` из настроек (Ф6 их редактирует);
 *  - USER использует значения по умолчанию из `app/config` — заявки всё равно
 *    приходят с `status_label`/`status_color`, поэтому карточки и теги
 *    показывают серверные значения.
 */
import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { fetchSettings } from '@/api/admin';
import type { TicketStatus, TicketStatusSetting } from '@/api/types';
import { TICKET_STATUS_COLORS, TICKET_STATUS_LABELS, TICKET_STATUS_ORDER } from '@/app/config';
import { queryKeys } from '@/app/queryClient';
import { useAuth } from '@/auth/AuthContext';

export interface UseTicketStatusMetaResult {
  statuses: TicketStatusSetting[];
  /** Метаданные статуса: из настроек, иначе — значения по умолчанию. */
  meta: (status: TicketStatus) => TicketStatusSetting;
  isLoading: boolean;
}

export function useTicketStatusMeta(): UseTicketStatusMetaResult {
  const { isAdmin } = useAuth();
  const settings = useQuery({
    queryKey: queryKeys.settings,
    queryFn: fetchSettings,
    enabled: isAdmin,
    staleTime: 5 * 60_000,
    retry: false,
  });

  const statuses = useMemo<TicketStatusSetting[]>(() => {
    return TICKET_STATUS_ORDER.map((code) => {
      const fromSettings = settings.data?.statuses.find((status) => status.code === code);
      return {
        code,
        label: fromSettings?.label?.trim() || TICKET_STATUS_LABELS[code],
        color: fromSettings?.color?.trim() || TICKET_STATUS_COLORS[code],
      };
    });
  }, [settings.data]);

  const meta = useMemo(
    () =>
      (status: TicketStatus): TicketStatusSetting =>
        statuses.find((item) => item.code === status) ?? {
          code: status,
          label: TICKET_STATUS_LABELS[status] ?? status,
          color: TICKET_STATUS_COLORS[status] ?? '#8c8c8c',
        },
    [statuses],
  );

  return { statuses, meta, isLoading: isAdmin && settings.isLoading };
}
