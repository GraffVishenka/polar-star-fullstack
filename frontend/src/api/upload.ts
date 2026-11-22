/**
 * Загрузка файлов с прогрессом (Б3).
 *
 * Прогресс отдачи тела доступен только в XHR, поэтому здесь XHR, а не fetch.
 * Авторизация — тот же Bearer из памяти + single-flight refresh при 401.
 *
 * `uploadFileToEndpoint` — общий транспорт: используется и защищённым
 * `POST /api/files` (Б3), и публичной загрузкой вложения анонима
 * (`uploadPublicationFile` в `api/public.ts`).
 */
import { buildUrl, getAccessToken, refreshSession } from './client';
import { ApiError, messageForCode, notifyApiError, parseApiErrorPayload } from './errors';
import type { FileMeta, FileScope, FileUploadResponse } from './types';

/** Настройки транспорта: произвольный эндпоинт, поля формы, авторизация. */
export interface EndpointUploadOptions {
  /** Дополнительные поля multipart (например, `scope`). */
  fields?: Record<string, string>;
  /** Прогресс 0..100. */
  onProgress?: (percent: number) => void;
  signal?: AbortSignal;
  /** `false` — публичный эндпоинт, без Bearer и refresh (вложения анонима). */
  withAuth?: boolean;
}

export interface UploadFileOptions {
  /** `ticket` — вложения комментариев, `kb` — медиа статей (KB-5). */
  scope: FileScope;
  /** Прогресс 0..100. */
  onProgress?: (percent: number) => void;
  signal?: AbortSignal;
}

function parseJson(text: string): unknown {
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return null;
  }
}

function toUploadError(status: number, text: string): ApiError {
  const payload = parseApiErrorPayload(parseJson(text));
  const code =
    payload.code ??
    (status === 413 ? 'PAYLOAD_TOO_LARGE' : status === 415 ? 'UNSUPPORTED_MEDIA_TYPE' : 'UNKNOWN');
  return new ApiError({
    code,
    message: payload.message ?? messageForCode(code, status),
    status,
    details: payload.details ?? null,
    requestId: payload.request_id ?? null,
  });
}

function xhrUploadToUrl(
  url: string,
  file: File,
  options: EndpointUploadOptions,
  token: string | null,
): Promise<FileMeta> {
  return new Promise<FileMeta>((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open('POST', url);
    request.responseType = 'text';
    request.withCredentials = true;
    request.setRequestHeader('Accept', 'application/json');
    if (token) request.setRequestHeader('Authorization', `Bearer ${token}`);

    if (options.signal) {
      if (options.signal.aborted) {
        reject(new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true }));
        return;
      }
      options.signal.addEventListener(
        'abort',
        () => {
          request.abort();
          reject(new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true }));
        },
        { once: true },
      );
    }

    request.upload.onprogress = (event) => {
      if (event.lengthComputable && event.total > 0 && options.onProgress) {
        options.onProgress(Math.round((event.loaded / event.total) * 100));
      }
    };

    request.onerror = () =>
      reject(new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true }));
    request.ontimeout = () =>
      reject(new ApiError({ code: 'TIMEOUT', status: 0, isNetworkError: true }));
    request.onload = () => {
      const status = request.status;
      const text = typeof request.response === 'string' ? request.response : request.responseText;
      if (status >= 200 && status < 300) {
        const payload = parseJson(text) as FileUploadResponse | null;
        if (payload?.file) resolve(payload.file);
        else
          reject(
            new ApiError({ code: 'INTERNAL_ERROR', status, message: 'Некорректный ответ сервера' }),
          );
        return;
      }
      reject(toUploadError(status, text));
    };

    const form = new FormData();
    form.append('file', file);
    Object.entries(options.fields ?? {}).forEach(([key, value]) => form.append(key, value));
    request.send(form);
  });
}

/**
 * Загружает файл на произвольный multipart-эндпоинт.
 * При 401 (если `withAuth`) делает refresh и повторяет попытку один раз.
 */
export async function uploadFileToEndpoint(
  url: string,
  file: File,
  options: EndpointUploadOptions = {},
): Promise<FileMeta> {
  const withAuth = options.withAuth !== false;
  try {
    return await xhrUploadToUrl(url, file, options, withAuth ? getAccessToken() : null);
  } catch (error) {
    if (withAuth && error instanceof ApiError && error.status === 401) {
      const token = await refreshSession();
      if (token) return xhrUploadToUrl(url, file, options, token);
    }
    if (error instanceof ApiError) notifyApiError(error);
    throw error;
  }
}

/** Загружает файл в `POST /api/files` (USER/ADMIN, Б3). */
export function uploadFile(file: File, options: UploadFileOptions): Promise<FileMeta> {
  return uploadFileToEndpoint(buildUrl('/files'), file, {
    fields: { scope: options.scope },
    onProgress: options.onProgress,
    signal: options.signal,
  });
}

// --- Клиентская валидация (лимиты Б3: 10 МБ фото, 200 МБ видео) -----------

export const MAX_IMAGE_BYTES = 10 * 1024 * 1024;
export const MAX_VIDEO_BYTES = 200 * 1024 * 1024;

export type UploadKind = 'image' | 'video';

export interface FileValidationResult {
  ok: boolean;
  kind: UploadKind | null;
  /** Русский текст ошибки для показа рядом с полем. */
  error?: string;
}

/** MIME → вид медиа. Расширение не решает: сервер тоже проверяет magic bytes (Б3). */
export function detectUploadKind(file: File): UploadKind | null {
  if (file.type.startsWith('image/')) return 'image';
  if (file.type.startsWith('video/')) return 'video';
  return null;
}

/** Проверка типа и размера файла ДО загрузки (10 МБ / 200 МБ). */
export function validateUploadFile(file: File): FileValidationResult {
  const kind = detectUploadKind(file);
  if (!kind) {
    return { ok: false, kind: null, error: 'Можно загружать только фото и видео' };
  }
  const limit = kind === 'image' ? MAX_IMAGE_BYTES : MAX_VIDEO_BYTES;
  if (file.size > limit) {
    return {
      ok: false,
      kind,
      error:
        kind === 'image'
          ? 'Фото больше 10 МБ — уменьшите размер файла'
          : 'Видео больше 200 МБ — уменьшите размер файла',
    };
  }
  if (file.size === 0) {
    return { ok: false, kind, error: 'Файл пустой' };
  }
  return { ok: true, kind };
}

/** Человекочитаемый размер для списка файлов. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} Б`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
}
