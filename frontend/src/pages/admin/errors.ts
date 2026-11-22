/**
 * Ошибки инфраструктурных админ-разделов (Ф7): здания/склады (EQ-6),
 * модерация БЗ (KB-3/KB-7), корзина (TRASH-2).
 *
 * `409 BUILDING_IN_USE` содержит `details.reasons` (что мешает удалению) —
 * показываем их списком и подсказываем деактивацию. `409 WAREHOUSE_NOT_EMPTY`
 * объясняет, что данные не потеряны: оборудование нужно переместить.
 */
import { ApiError } from '@/api/errors';

/** Массив строк из `details` (например, `details.reasons`). */
export function readReasons(details: Record<string, unknown> | null | undefined): string[] {
  const raw = details?.reasons;
  if (!Array.isArray(raw)) return [];
  return raw.filter((item): item is string => typeof item === 'string' && item.length > 0);
}

export interface InfraErrorText {
  title: string;
  message: string;
  reasons: string[];
}

/** Человеческий текст ошибки удаления здания/склада или правки справочника. */
export function describeInfraError(cause: unknown): InfraErrorText {
  if (!(cause instanceof ApiError)) {
    return {
      title: 'Не удалось выполнить действие',
      message: 'Попробуйте ещё раз или проверьте соединение.',
      reasons: [],
    };
  }

  switch (cause.code) {
    case 'BUILDING_IN_USE': {
      const reasons = readReasons(cause.details);
      return {
        title: 'Здание нельзя удалить',
        message:
          'Здание используется, поэтому физическое удаление недоступно — деактивируйте его: история, заявки и оборудование сохранятся (EQ-6).',
        reasons,
      };
    }
    case 'WAREHOUSE_NOT_EMPTY': {
      const reasons = readReasons(cause.details);
      const count = Number(cause.details?.equipment_count ?? 0);
      return {
        title: 'Помещение нельзя удалить',
        message:
          'В помещении есть оборудование — переместите его в другое помещение. Данные не потеряны (EQ-6).',
        reasons: reasons.length ? reasons : count ? [`Единиц оборудования: ${count}`] : [],
      };
    }
    case 'BUILDING_INACTIVE':
      return {
        title: 'Здание деактивировано',
        message: 'Активные действия с объектами деактивированного здания ограничены (EQ-3).',
        reasons: [],
      };
    case 'CONFLICT':
      // Для справочников 409 означает занятое имя/логин — текст сервера уместнее общего.
      return { title: 'Конфликт данных', message: cause.message, reasons: [] };
    case 'VALIDATION_ERROR':
      return { title: 'Проверьте данные', message: cause.message, reasons: [] };
    default:
      return { title: 'Не удалось выполнить действие', message: cause.userMessage, reasons: [] };
  }
}

/** Короткий текст для инлайн-ошибки в форме (например, занятое имя склада). */
export function describeInfraFieldError(cause: unknown): string | null {
  if (!(cause instanceof ApiError)) return 'Не удалось выполнить действие';
  if (cause.code === 'CONFLICT') return cause.message;
  return cause.userMessage;
}

/** Одна строка ошибки для тостов и инлайн-алертов. */
export function describeInfraErrorMessage(cause: unknown): string {
  return describeInfraError(cause).message;
}
