/**
 * Ошибки API и их отображение на русском (§3.13).
 *
 * Формат ответа сервера: `{code, message, details, request_id}`.
 * Задача клиента — показать понятный русский текст и, для 5xx, `request_id`,
 * чтобы поддержка могла найти запрос в логах (NFR-4).
 */
import type { ApiErrorResponse } from './types';

/** Коды ошибок из PLAN.md (доменные) + общие HTTP-коды. */
export type ApiErrorCode =
  | 'INVALID_CREDENTIALS'
  | 'PASSWORD_CHANGE_REQUIRED'
  | 'BUILDING_REQUIRED_FOR_USER'
  | 'BUILDING_INACTIVE'
  | 'BUILDING_NOT_ASSIGNED'
  | 'BUILDING_IN_USE'
  | 'WAREHOUSE_NOT_EMPTY'
  | 'CANNOT_MODIFY_SELF'
  | 'LAST_ADMIN'
  | 'VALIDATION_ERROR'
  | 'CYCLE_DETECTED'
  | 'EXECUTOR_WRONG_BUILDING'
  | 'ASSIGN_OTHER_USER'
  | 'NOT_EXECUTOR'
  | 'INVALID_MEDIA_REF'
  | 'SAME_WAREHOUSE'
  | 'DUPLICATE_REGISTRATION_NUMBER'
  | 'TOO_MANY_REQUESTS'
  | 'UNAUTHORIZED'
  | 'FORBIDDEN'
  | 'NOT_FOUND'
  | 'CONFLICT'
  | 'PAYLOAD_TOO_LARGE'
  | 'UNSUPPORTED_MEDIA_TYPE'
  | 'INTERNAL_ERROR'
  | 'NETWORK_ERROR'
  | 'TIMEOUT'
  | 'UNKNOWN';

/**
 * Маппинг `code` → русский текст. Сообщение сервера показываем только как
 * дополнение (`details`/`message`) — пользователям нужен стабильный текст,
 * который не зависит от формулировок бэкенда.
 */
export const ERROR_MESSAGES: Record<string, string> = {
  INVALID_CREDENTIALS: 'Неверный логин или пароль',
  PASSWORD_CHANGE_REQUIRED: 'Сначала смените временный пароль',
  BUILDING_REQUIRED_FOR_USER: 'Для сотрудника обязательно выберите здание',
  BUILDING_INACTIVE: 'Здание деактивировано: действие недоступно',
  BUILDING_NOT_ASSIGNED: 'Здание не назначено, обратитесь к администратору',
  BUILDING_IN_USE: 'Здание используется: деактивируйте его вместо удаления',
  WAREHOUSE_NOT_EMPTY: 'В помещении есть оборудование — переместите его',
  CANNOT_MODIFY_SELF: 'Нельзя изменить собственную учётную запись',
  LAST_ADMIN: 'Нельзя понизить или заблокировать последнего администратора',
  VALIDATION_ERROR: 'Проверьте заполнение полей',
  CYCLE_DETECTED: 'Нельзя вложить раздел в самого себя',
  EXECUTOR_WRONG_BUILDING: 'Исполнитель должен быть из здания заявки',
  ASSIGN_OTHER_USER: 'Назначить можно только себя',
  NOT_EXECUTOR: 'Править заявку может только её исполнитель',
  INVALID_MEDIA_REF: 'Файл не найден или недоступен для этого раздела',
  SAME_WAREHOUSE: 'Оборудование уже находится в этом помещении',
  DUPLICATE_REGISTRATION_NUMBER: 'Такой регистрационный номер уже используется',
  TOO_MANY_REQUESTS: 'Слишком много запросов, попробуйте позже',
  UNAUTHORIZED: 'Требуется вход в систему',
  FORBIDDEN: 'Недостаточно прав для этого действия',
  NOT_FOUND: 'Запись не найдена',
  CONFLICT: 'Данные уже изменились, обновите страницу',
  PAYLOAD_TOO_LARGE: 'Файл слишком большой',
  UNSUPPORTED_MEDIA_TYPE: 'Недопустимый тип файла',
  INTERNAL_ERROR: 'Ошибка сервера, попробуйте позже',
  NETWORK_ERROR: 'Нет связи с сервером',
  TIMEOUT: 'Сервер не ответил вовремя',
  UNKNOWN: 'Неизвестная ошибка',
};

/** Тексты-заглушки по HTTP-статусу, если сервер не прислал `code`. */
const STATUS_MESSAGES: Record<number, string> = {
  400: 'Некорректный запрос',
  401: 'Требуется вход в систему',
  403: 'Недостаточно прав для этого действия',
  404: 'Запись не найдена',
  409: 'Конфликт данных',
  413: 'Файл слишком большой',
  415: 'Недопустимый тип файла',
  422: 'Проверьте заполнение полей',
  428: 'Сначала смените временный пароль',
  429: 'Слишком много запросов, попробуйте позже',
  500: 'Ошибка сервера, попробуйте позже',
  502: 'Сервис временно недоступен',
  503: 'Сервис временно недоступен',
  504: 'Сервер не ответил вовремя',
};

/** Русский текст для кода ошибки; при неизвестном коде — по статусу. */
export function messageForCode(code?: string, status?: number): string {
  if (code && ERROR_MESSAGES[code]) return ERROR_MESSAGES[code]!;
  if (status !== undefined && STATUS_MESSAGES[status]) return STATUS_MESSAGES[status]!;
  return ERROR_MESSAGES.UNKNOWN!;
}

/** Ошибка уровня API: доменный код, HTTP-статус, `details` и `request_id`. */
export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly details: Record<string, unknown> | null;
  readonly requestId: string | null;
  /** true, если запрос не дошёл до сервера. */
  readonly isNetworkError: boolean;

  constructor(params: {
    code?: string;
    message?: string;
    status?: number;
    details?: Record<string, unknown> | null;
    requestId?: string | null;
    isNetworkError?: boolean;
    cause?: unknown;
  }) {
    const status = params.status ?? 0;
    const code = params.code ?? (params.isNetworkError ? 'NETWORK_ERROR' : 'UNKNOWN');
    super(params.message?.trim() || messageForCode(code, status));
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.details = params.details ?? null;
    this.requestId = params.requestId ?? null;
    this.isNetworkError = params.isNetworkError ?? false;
    if (params.cause !== undefined) this.cause = params.cause;
  }

  /** Понятный пользователю текст (русский), независимый от формулировок сервера. */
  get userMessage(): string {
    return messageForCode(this.code, this.status);
  }

  /** 4xx — повторять запрос бессмысленно. */
  get isClientError(): boolean {
    return this.status >= 400 && this.status < 500;
  }

  get isServerError(): boolean {
    return this.status >= 500;
  }

  /** 5xx и сетевые сбои показываем автоматически, с `request_id`. */
  get shouldNotify(): boolean {
    return this.isServerError || this.isNetworkError || this.status === 429;
  }
}

/** Разбор тела ошибки; терпим к не-JSON и к отсутствию полей. */
export function parseApiErrorPayload(raw: unknown): Partial<ApiErrorResponse> {
  if (raw && typeof raw === 'object') {
    const value = raw as Record<string, unknown>;
    return {
      code: typeof value.code === 'string' ? value.code : undefined,
      message: typeof value.message === 'string' ? value.message : undefined,
      details:
        value.details && typeof value.details === 'object'
          ? (value.details as Record<string, unknown>)
          : null,
      request_id: typeof value.request_id === 'string' ? value.request_id : undefined,
    };
  }
  return {};
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

/** Единая точка показа серверных ошибок (регистрируется в провайдерах). */
type ErrorNotifier = (error: ApiError) => void;

let errorNotifier: ErrorNotifier | null = null;

export function setErrorNotifier(notifier: ErrorNotifier | null): void {
  errorNotifier = notifier;
}

export function notifyApiError(error: ApiError): void {
  if (!error.shouldNotify) return;
  errorNotifier?.(error);
}

/** Подпись для тоста: текст + request_id для 5xx (NFR-4). */
export function describeApiError(error: ApiError): { title: string; description?: string } {
  if (error.requestId && error.shouldNotify) {
    return { title: error.userMessage, description: `Код запроса: ${error.requestId}` };
  }
  return { title: error.userMessage };
}
