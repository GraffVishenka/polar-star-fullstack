/**
 * Публичные настройки приложения (`GET /api/public/settings`, Б7, PUB-1).
 *
 * Публичный дашборд и шапка анонимной части должны показывать актуальное
 * `app_name` и названия/цвета статусов из Admin-панели. Контракт отдаёт только
 * эти два поля; календарь, часовой пояс и срок наружу не уходят.
 *
 * Ключ кэша один (`queryKeys.publicSettings`), поэтому сколько бы компонентов
 * ни вызывали хук, запрос уходит один раз (React Query дедуплицирует по ключу).
 * Любая ошибка/неполный ответ → безопасный фолбэк на значения из `app/config`,
 * чтобы публичные страницы не ломались при недоступном API.
 */
import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { fetchPublicSettings } from '@/api/dictionaries';
import type { PublicSettings, TicketStatus } from '@/api/types';
import {
  APP_NAME,
  TICKET_STATUS_COLORS,
  TICKET_STATUS_LABELS,
  TICKET_STATUS_ORDER,
} from '@/app/config';
import { queryKeys } from '@/app/queryClient';

export interface PublicStatusMeta {
  label: string;
  color: string;
}

/** Значения по умолчанию — те же, что в `app/config` (на случай недоступности API). */
export const FALLBACK_PUBLIC_SETTINGS: PublicSettings = {
  app_name: APP_NAME,
  statuses: TICKET_STATUS_ORDER.map((code) => ({
    code,
    label: TICKET_STATUS_LABELS[code],
    color: TICKET_STATUS_COLORS[code],
  })),
};

/**
 * Дополняет ответ сервера значениями по умолчанию: статусы всегда 4 и в
 * фиксированном порядке (TICKET-1), отсутствующие подписи/цвета берём из конфига.
 */
export function mergePublicSettings(data?: PublicSettings | null): PublicSettings {
  const byCode = new Map((data?.statuses ?? []).map((status) => [status.code, status] as const));
  return {
    app_name: data?.app_name?.trim() || FALLBACK_PUBLIC_SETTINGS.app_name,
    statuses: TICKET_STATUS_ORDER.map((code) => {
      const fallback = FALLBACK_PUBLIC_SETTINGS.statuses.find((item) => item.code === code)!;
      const stored = byCode.get(code);
      return {
        code,
        label: stored?.label?.trim() || fallback.label,
        color: /^#[0-9a-fA-F]{6}$/.test(stored?.color?.trim() ?? '')
          ? (stored!.color as string)
          : fallback.color,
      };
    }),
  };
}

export interface UsePublicSettingsResult {
  /** Всегда готовые настройки: серверные значения с фолбэком на конфиг. */
  settings: PublicSettings;
  /** `app_name` для шапки/подвала и заголовков публичных страниц. */
  appName: string;
  /** Подписи и цвета статусов по коду (для колонок дашборда и тегов). */
  statusMeta: Record<TicketStatus, PublicStatusMeta>;
  /** Первая загрузка (показываем скелетон, если нужно). */
  isLoading: boolean;
  /** true — API недоступен или вернул неполные данные, показан фолбэк. */
  isFallback: boolean;
}

export function usePublicSettings(): UsePublicSettingsResult {
  const query = useQuery({
    queryKey: queryKeys.publicSettings,
    queryFn: fetchPublicSettings,
    // Публичные настройки меняются редко; при ошибке не ретраим — фолбэк.
    staleTime: 5 * 60_000,
    gcTime: 30 * 60_000,
    retry: false,
  });

  return useMemo(() => {
    const settings = mergePublicSettings(query.data);
    const statusMeta = settings.statuses.reduce<Record<TicketStatus, PublicStatusMeta>>(
      (acc, status) => {
        acc[status.code] = { label: status.label, color: status.color };
        return acc;
      },
      {} as Record<TicketStatus, PublicStatusMeta>,
    );
    return {
      settings,
      appName: settings.app_name,
      statusMeta,
      isLoading: query.isLoading,
      isFallback: query.isError || !query.data,
    };
  }, [query.data, query.isError, query.isLoading]);
}
