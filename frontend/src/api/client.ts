/**
 * HTTP-клиент API (fetch, без axios).
 *
 * Требования Ф1 / AUTH-2:
 *  - baseURL `/api`, Bearer-токен только в памяти (никаких localStorage);
 *  - при 401 — single-flight refresh через httpOnly refresh-cookie и ОДИН повтор
 *    исходного запроса новым токеном;
 *  - типизированные ошибки `{code,message,details,request_id}`, маппинг кодов
 *    в русские тексты, тост с `request_id` для 5xx.
 */
import {
  ApiError,
  messageForCode,
  notifyApiError,
  parseApiErrorPayload,
  setErrorNotifier,
} from './errors';
import type { ApiErrorResponse, AuthUser, RefreshResponse } from './types';

/** Базовый URL API: `/api` в бою (nginx/vite proxy), абсолютный — в тестах. */
export const API_BASE_URL: string = (import.meta.env.VITE_API_BASE_URL?.trim() || '/api').replace(
  /\/+$/,
  '',
);

/** Пути, для которых refresh/повтор не выполняется (иначе цикл). */
const NO_REFRESH_PATHS = ['/auth/login', '/auth/refresh', '/auth/logout'];

export type QueryValue = string | number | boolean | null | undefined | Array<string | number>;

/** Любой объект параметров домена (у интерфейсов нет index signature). */
export type QueryRecord = object;

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE';
  /** JSON-тело; объект сериализуется, `FormData` отправляется как есть. */
  body?: unknown;
  query?: QueryRecord;
  headers?: Record<string, string>;
  signal?: AbortSignal;
  /** Публичные эндпоинты: без Bearer и без refresh при 401. */
  skipAuth?: boolean;
  /** Запрет refresh (например, сам `/auth/login`). */
  skipRefresh?: boolean;
  timeoutMs?: number;
  /** `blob` — для скачивания документов, `void` — 204/пустой ответ. */
  responseType?: 'json' | 'blob' | 'void';
}

// ---------------------------------------------------------------------------
// Хранилище access-токена (только память)
// ---------------------------------------------------------------------------

let accessToken: string | null = null;
let refreshedUser: AuthUser | null = null;
const tokenListeners = new Set<(token: string | null) => void>();
const authLostListeners = new Set<() => void>();

export function getAccessToken(): string | null {
  return accessToken;
}

export function setAccessToken(token: string | null): void {
  if (accessToken === token) return;
  accessToken = token;
  tokenListeners.forEach((listener) => listener(token));
}

export function onAccessTokenChange(listener: (token: string | null) => void): () => void {
  tokenListeners.add(listener);
  return () => tokenListeners.delete(listener);
}

/** Подписка на потерю сессии (refresh-cookie истёк/отозван). */
export function onAuthLost(listener: () => void): () => void {
  authLostListeners.add(listener);
  return () => authLostListeners.delete(listener);
}

/** Пользователь, пришедший вместе с refresh-ответом (если сервер его отдал). */
export function getRefreshedUser(): AuthUser | null {
  return refreshedUser;
}

/** Полный сброс клиентской сессии (logout, неудачный refresh). */
export function clearSession(): void {
  setAccessToken(null);
  refreshedUser = null;
  refreshPromise = null;
}

// ---------------------------------------------------------------------------
// Single-flight refresh
// ---------------------------------------------------------------------------

let refreshPromise: Promise<string | null> | null = null;

async function performRefresh(): Promise<string | null> {
  try {
    const response = await fetch(buildUrl('/auth/refresh'), {
      method: 'POST',
      credentials: 'include',
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) {
      setAccessToken(null);
      refreshedUser = null;
      return null;
    }
    const payload = (await response.json()) as RefreshResponse;
    if (!payload?.access_token) {
      setAccessToken(null);
      refreshedUser = null;
      return null;
    }
    refreshedUser = payload.user ?? null;
    setAccessToken(payload.access_token);
    return payload.access_token;
  } catch {
    setAccessToken(null);
    refreshedUser = null;
    return null;
  }
}

/**
 * Обновляет access-токен. Параллельные вызовы (в т.ч. из разных запросов)
 * используют одно и то же обещание — single-flight, без «шторма» refresh.
 */
export function refreshSession(): Promise<string | null> {
  if (!refreshPromise) {
    refreshPromise = performRefresh().finally(() => {
      refreshPromise = null;
    });
  }
  return refreshPromise;
}

/** Bootstrap при старте приложения: восстановление сессии по refresh-cookie. */
export async function bootstrapSession(): Promise<{
  accessToken: string;
  user: AuthUser | null;
} | null> {
  const token = await refreshSession();
  if (!token) return null;
  return { accessToken: token, user: refreshedUser };
}

/** Для тестов: сбрасывает состояние single-flight и токен. */
export function __resetApiClientForTests(): void {
  clearSession();
  tokenListeners.clear();
  authLostListeners.clear();
  setErrorNotifier(null);
}

// ---------------------------------------------------------------------------
// URL и заголовки
// ---------------------------------------------------------------------------

function normalizePath(path: string): string {
  return path.startsWith('/') ? path : `/${path}`;
}

export function buildUrl(path: string, query?: QueryRecord): string {
  const base = `${API_BASE_URL}${normalizePath(path)}`;
  const origin =
    typeof globalThis.location !== 'undefined' && globalThis.location.origin
      ? globalThis.location.origin
      : 'http://localhost';
  const url = new URL(base, origin);
  if (query) {
    for (const [key, rawValue] of Object.entries(query)) {
      const value = rawValue as QueryValue;
      if (value === undefined || value === null || value === '') continue;
      if (Array.isArray(value)) {
        // Массивы (например, статусы) уходят повторяющимся параметром.
        value.forEach((item) => url.searchParams.append(key, String(item)));
      } else {
        url.searchParams.set(key, String(value));
      }
    }
  }
  return url.toString();
}

function defaultCodeForStatus(status: number): string {
  switch (status) {
    case 400:
      return 'VALIDATION_ERROR';
    case 401:
      return 'UNAUTHORIZED';
    case 403:
      return 'FORBIDDEN';
    case 404:
      return 'NOT_FOUND';
    case 409:
      return 'CONFLICT';
    case 413:
      return 'PAYLOAD_TOO_LARGE';
    case 415:
      return 'UNSUPPORTED_MEDIA_TYPE';
    case 422:
      return 'VALIDATION_ERROR';
    case 428:
      return 'PASSWORD_CHANGE_REQUIRED';
    case 429:
      return 'TOO_MANY_REQUESTS';
    default:
      return status >= 500 ? 'INTERNAL_ERROR' : 'UNKNOWN';
  }
}

async function toApiError(response: Response): Promise<ApiError> {
  let payload: Partial<ApiErrorResponse> = {};
  try {
    const text = await response.text();
    if (text) payload = parseApiErrorPayload(JSON.parse(text));
  } catch {
    // не-JSON тело (HTML от прокси) — используем статус
  }
  const code = payload.code ?? defaultCodeForStatus(response.status);
  return new ApiError({
    code,
    message: payload.message ?? messageForCode(code, response.status),
    status: response.status,
    details: payload.details ?? null,
    requestId: payload.request_id ?? response.headers.get('X-Request-Id'),
  });
}

// ---------------------------------------------------------------------------
// Транспорт
// ---------------------------------------------------------------------------

function composeSignal(signal?: AbortSignal, timeoutMs?: number): AbortSignal | undefined {
  if (!timeoutMs && !signal) return undefined;
  const controller = new AbortController();
  const abort = (reason: unknown) => {
    if (!controller.signal.aborted) controller.abort(reason);
  };
  if (signal) {
    if (signal.aborted) abort(signal.reason);
    else signal.addEventListener('abort', () => abort(signal.reason), { once: true });
  }
  if (timeoutMs) {
    const timer = setTimeout(() => abort(new Error('timeout')), timeoutMs);
    controller.signal.addEventListener('abort', () => clearTimeout(timer), { once: true });
  }
  return controller.signal;
}

async function performFetch(
  url: string,
  options: RequestOptions,
  token: string | null,
): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json', ...options.headers };
  const isFormData = typeof FormData !== 'undefined' && options.body instanceof FormData;
  if (options.body !== undefined && !isFormData) {
    headers['Content-Type'] = headers['Content-Type'] ?? 'application/json';
  }
  if (token) headers.Authorization = `Bearer ${token}`;

  return fetch(url, {
    method: options.method ?? 'GET',
    headers,
    body:
      options.body === undefined
        ? undefined
        : isFormData
          ? (options.body as FormData)
          : JSON.stringify(options.body),
    credentials: 'same-origin',
    signal: composeSignal(options.signal, options.timeoutMs),
  });
}

/** Запрос с авторизацией, single-flight refresh и одним повтором при 401. */
export async function apiRequest<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const url = buildUrl(path, options.query);
  const canRefresh =
    !options.skipAuth &&
    !options.skipRefresh &&
    !NO_REFRESH_PATHS.some((blocked) => normalizePath(path).startsWith(blocked));

  let response: Response;
  try {
    response = await performFetch(url, options, options.skipAuth ? null : getAccessToken());
  } catch (cause) {
    const isAbort = cause instanceof DOMException && cause.name === 'AbortError';
    throw new ApiError({
      code: isAbort ? 'TIMEOUT' : 'NETWORK_ERROR',
      status: 0,
      isNetworkError: true,
      cause,
    });
  }

  if (response.status === 401 && canRefresh) {
    const token = await refreshSession();
    if (!token) {
      authLostListeners.forEach((listener) => listener());
      throw await toApiError(response);
    }
    try {
      response = await performFetch(url, options, token);
    } catch (cause) {
      throw new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true, cause });
    }
    if (response.status === 401) {
      authLostListeners.forEach((listener) => listener());
    }
  }

  if (!response.ok) {
    const error = await toApiError(response);
    notifyApiError(error);
    throw error;
  }

  if (options.responseType === 'blob') return (await response.blob()) as T;
  if (options.responseType === 'void' || response.status === 204) return undefined as T;
  if (response.status === 205) return undefined as T;

  const text = await response.text();
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch (cause) {
    throw new ApiError({
      code: 'INTERNAL_ERROR',
      status: response.status,
      message: 'Некорректный ответ сервера',
      cause,
    });
  }
}

/** Скачивание файла (Word/Excel) с разбором имени из Content-Disposition. */
export interface DownloadResult {
  blob: Blob;
  filename: string | null;
}

export async function apiDownload(
  path: string,
  options: Omit<RequestOptions, 'responseType'> = {},
): Promise<DownloadResult> {
  const url = buildUrl(path, options.query);
  const canRefresh = !options.skipAuth && !options.skipRefresh;

  let response: Response;
  try {
    response = await performFetch(url, options, options.skipAuth ? null : getAccessToken());
  } catch (cause) {
    throw new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true, cause });
  }

  if (response.status === 401 && canRefresh) {
    const token = await refreshSession();
    if (!token) {
      authLostListeners.forEach((listener) => listener());
      throw await toApiError(response);
    }
    response = await performFetch(url, options, token);
  }

  if (!response.ok) {
    const error = await toApiError(response);
    notifyApiError(error);
    throw error;
  }

  return {
    blob: await response.blob(),
    filename: parseContentDispositionFilename(response.headers.get('Content-Disposition')),
  };
}

/** `attachment; filename="ticket_12.docx"; filename*=UTF-8''%D0%B7...` → имя файла. */
export function parseContentDispositionFilename(header: string | null): string | null {
  if (!header) return null;
  const utf8 = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1].trim());
    } catch {
      return utf8[1].trim();
    }
  }
  const plain = /filename="([^"]+)"/i.exec(header) ?? /filename=([^;]+)/i.exec(header);
  return plain?.[1]?.trim() ?? null;
}

export { ApiError, setErrorNotifier };
