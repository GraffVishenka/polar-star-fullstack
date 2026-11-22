/**
 * Общие помощники админ-панели Ф6: роли, рабочие дни, пароли, IANA-пояса
 * и маппинг серверных 422 (`details`) на поля формы.
 */
import type { FormInstance } from 'antd';
import { ApiError } from '@/api/errors';
import type { Role, Weekday } from '@/api/types';

export const ROLE_LABELS: Record<Role, string> = {
  USER: 'Сотрудник',
  ADMIN: 'Администратор',
};

/** Порядок дней недели для маски CAL-1 (Пн — первый). */
export const WEEKDAY_OPTIONS: Array<{ value: Weekday; label: string; short: string }> = [
  { value: 'mon', label: 'Понедельник', short: 'Пн' },
  { value: 'tue', label: 'Вторник', short: 'Вт' },
  { value: 'wed', label: 'Среда', short: 'Ср' },
  { value: 'thu', label: 'Четверг', short: 'Чт' },
  { value: 'fri', label: 'Пятница', short: 'Пт' },
  { value: 'sat', label: 'Суббота', short: 'Сб' },
  { value: 'sun', label: 'Воскресенье', short: 'Вс' },
];

export const WEEKDAY_LABELS: Record<Weekday, string> = {
  mon: 'Пн',
  tue: 'Вт',
  wed: 'Ср',
  thu: 'Чт',
  fri: 'Пт',
  sat: 'Сб',
  sun: 'Вс',
};

/** Подсказки для IANA-пояса: можно ввести любой валидный идентификатор. */
export const TIMEZONE_SUGGESTIONS = [
  'Asia/Yekaterinburg',
  'Asia/Moscow',
  'Asia/Novosibirsk',
  'Asia/Krasnoyarsk',
  'Asia/Vladivostok',
  'Europe/Kaliningrad',
  'Europe/Moscow',
  'Europe/Berlin',
  'UTC',
];

/**
 * Временный пароль ≥ 8 символов (AUTH-4): буквы обоих регистров и цифры,
 * без похожих символов (0/O, 1/l/I). Криптостойкий источник, если доступен.
 */
export function generateTemporaryPassword(length = 12): string {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789';
  const bytes = new Uint8Array(length);
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    crypto.getRandomValues(bytes);
  } else {
    for (let index = 0; index < length; index += 1) {
      bytes[index] = Math.floor(Math.random() * 256);
    }
  }
  let password = Array.from(bytes, (byte) => chars[byte % chars.length]).join('');
  // Гарантируем требования сложности, даже если случайность подвела.
  if (!/[A-Z]/.test(password)) password = `A${password.slice(1)}`;
  if (!/[a-z]/.test(password)) password = `${password.slice(0, 1)}a${password.slice(2)}`;
  if (!/[0-9]/.test(password)) password = `${password.slice(0, -1)}7`;
  return password;
}

/** Проверка IANA-идентификатора (CAL-2) через Intl. */
export function isKnownTimeZone(timezone: string): boolean {
  if (!timezone.trim()) return false;
  try {
    new Intl.DateTimeFormat('ru-RU', { timeZone: timezone });
    return true;
  } catch {
    return false;
  }
}

/** Текущие дата и время в поясе `timeZone` (для подсказки под полем). */
export function formatTimeInZone(date: Date, timeZone: string): string {
  try {
    return new Intl.DateTimeFormat('ru-RU', {
      timeZone,
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).format(date);
  } catch {
    return '—';
  }
}

/** Русский текст ошибки API без привязки к домену. */
export function adminErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) return error.message || error.userMessage;
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

/**
 * Переносит `details` из 422-ответа в ошибки полей antd-формы.
 * Возвращает список полей, к которым ошибка применилась; если он пуст —
 * вызывающий показывает общий Alert (например, 409 LAST_ADMIN).
 */
export function applyApiFieldErrors<Values extends object>(
  form: FormInstance<Values>,
  error: unknown,
  allowedFields: readonly string[],
): string[] {
  if (!(error instanceof ApiError) || !error.details) return [];
  const applied: string[] = [];
  const message = error.message || error.userMessage;
  for (const key of Object.keys(error.details)) {
    if (!allowedFields.includes(key)) continue;
    form.setFields([{ name: [key] as never, errors: [message] }]);
    applied.push(key);
  }
  return applied;
}

/** Маска `HH:MM` (CAL-3) — клиентская подсказка до запроса на сервер. */
export function isValidReminderTime(value: string): boolean {
  return /^([01]\d|2[0-3]):[0-5]\d$/.test(value.trim());
}

/** `HH:MM` → `HH:MM`, невалидное значение возвращается как есть (для инпута). */
export function normalizeReminderTime(value: string): string {
  return value.trim();
}
