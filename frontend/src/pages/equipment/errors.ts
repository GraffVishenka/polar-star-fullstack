/**
 * Тексты ошибок домена «Оборудование» (§3.6, §3.13).
 *
 * Ошибки не парсим по тексту: сервер присылает `code`, из него получаем
 * понятный русский текст. Для `BUILDING_INACTIVE` уточняем правило EQ-3 —
 * перемещение в неактивное здание доступно только ADMIN.
 */
import { ApiError } from '@/api/errors';

export type EquipmentErrorContext = 'move' | 'bulk' | 'save' | 'delete';

const INACTIVE_MESSAGE =
  'Нельзя переместить оборудование в неактивное здание: это доступно только администратору (EQ-3)';

export function describeEquipmentError(cause: unknown, context: EquipmentErrorContext): string {
  if (!(cause instanceof ApiError)) {
    return 'Не удалось выполнить действие, попробуйте позже';
  }

  switch (cause.code) {
    case 'BUILDING_INACTIVE':
      return INACTIVE_MESSAGE;
    case 'SAME_WAREHOUSE':
      return 'Оборудование уже находится в этом помещении';
    case 'DUPLICATE_REGISTRATION_NUMBER':
      return 'Такой регистрационный номер уже используется другой единицей';
    case 'WAREHOUSE_NOT_EMPTY':
      return 'В помещении есть оборудование — сначала переместите его';
    case 'NOT_FOUND':
      return context === 'bulk'
        ? 'Одна из единиц не найдена — перемещение отменено. Обновите список и повторите'
        : 'Запись не найдена: возможно, её уже удалили. Обновите список';
    case 'BUILDING_IN_USE':
      return 'Здание используется: деактивируйте его вместо удаления';
    case 'VALIDATION_ERROR':
      return context === 'bulk'
        ? 'Проверьте набор единиц: за операцию можно переместить от 1 до 200 единиц (EQ-4)'
        : 'Проверьте заполнение полей';
    default:
      return cause.userMessage;
  }
}
