/** Константы приложения. */
import type { TicketStatus } from '@/api/types';

/**
 * Имя приложения по умолчанию. В продакшене переопределяется настройкой
 * `app_name` из Admin-панели (Ф6) — публичный эндпоинт настроек появится позже.
 */
export const APP_NAME: string = import.meta.env.VITE_APP_NAME?.trim() || 'Сервисный центр';

/** Заголовок вкладки. */
export const APP_DESCRIPTION = 'Заявки · База знаний · Оборудование';

/** Порядок колонок дашборда и фильтров (§3.3 TICKET-1). */
export const TICKET_STATUS_ORDER: TicketStatus[] = ['new', 'in_progress', 'deferred', 'done'];

/** Русские названия по умолчанию (переопределяются `status_label` с сервера). */
export const TICKET_STATUS_LABELS: Record<TicketStatus, string> = {
  new: 'Новая',
  in_progress: 'В работе',
  deferred: 'Отложена',
  done: 'Завершена',
};

/** Цвета по умолчанию (переопределяются `status_color` из настроек). */
export const TICKET_STATUS_COLORS: Record<TicketStatus, string> = {
  new: '#1677ff',
  in_progress: '#fa8c16',
  deferred: '#8c8c8c',
  done: '#52c41a',
};

/** Публичный дашборд здания показывает 3 статуса: без `done` (TICKET-5). */
export const PUBLIC_STATUS_ORDER: TicketStatus[] = ['new', 'in_progress', 'deferred'];

export const PAGE_SIZE_DEFAULT = 20;
export const PAGE_SIZE_MAX = 100;
