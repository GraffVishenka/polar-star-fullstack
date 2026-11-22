/**
 * Корзина (TRASH-1..TRASH-5): подписи, срок автоочистки и тексты ошибок.
 *
 * Клиент не придумывает сроки: 90 дней — правило TRASH-4, дата удаления приходит
 * с сервера, а автоочистка вычисляется только для подсказки администратору.
 */
import dayjs from 'dayjs';
import { ApiError } from '@/api/errors';
import type { TrashType } from '@/api/types';

/** TRASH-4: корзина хранится 90 дней. */
export const TRASH_RETENTION_DAYS = 90;

/** Слово-подтверждение для необратимого удаления (TRASH-3). */
export const TRASH_PURGE_CONFIRMATION = 'УДАЛИТЬ';

export const TRASH_TYPE_LABELS: Record<TrashType, string> = {
  ticket: 'Заявки',
  kb: 'База знаний',
  equipment: 'Оборудование',
};

export const TRASH_TYPE_EMPTY: Record<TrashType, string> = {
  ticket: 'Удалённых заявок нет',
  kb: 'Удалённых разделов нет',
  equipment: 'Удалённого оборудования нет',
};

/** `YYYY-MM-DD HH:mm` из ISO-строки сервера. */
export function formatDeletedAt(value: string): string {
  const parsed = dayjs(value);
  return parsed.isValid() ? parsed.format('DD.MM.YYYY HH:mm') : value;
}

/** Дата автоочистки: дата удаления + 90 дней (TRASH-4). */
export function autoCleanupDate(deletedAt: string): string {
  const parsed = dayjs(deletedAt);
  return parsed.isValid() ? parsed.add(TRASH_RETENTION_DAYS, 'day').format('DD.MM.YYYY') : '—';
}

/** Сколько дней осталось до автоочистки (может быть отрицательным). */
export function daysUntilCleanup(deletedAt: string): number {
  const parsed = dayjs(deletedAt);
  if (!parsed.isValid()) return 0;
  return parsed.add(TRASH_RETENTION_DAYS, 'day').diff(dayjs(), 'day');
}

/** Текст ошибки восстановления (TRASH-2). */
export function describeTrashError(cause: unknown): string {
  if (!(cause instanceof ApiError)) return 'Не удалось выполнить действие, попробуйте позже';
  switch (cause.code) {
    case 'DUPLICATE_REGISTRATION_NUMBER':
      return 'Регистрационный номер уже занят другой единицей — восстановление невозможно. Освободите номер или удалите активную единицу.';
    case 'CONFLICT':
      return cause.message;
    case 'NOT_FOUND':
      return 'Элемент уже удалён из корзины — обновите список';
    default:
      return cause.userMessage;
  }
}
