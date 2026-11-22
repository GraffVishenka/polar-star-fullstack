/**
 * Загрузка файлов с прогрессом (Б3).
 *
 * Прогресс отдачи тела доступен только в XHR, поэтому здесь XHR, а не fetch.
 * Авторизация — тот же Bearer из памяти + single-flight refresh при 401.
 */
import { getAccessToken, refreshSession } from './client';
import { ApiError, messageForCode, notifyApiError, parseApiErrorPayload } from './errors';
import type { FileMeta, FileScope, FileUploadResponse } from './types';

export interface UploadFileOptions {
  /** `ticket` — вложения комментариев, `kb` — медиа статей (KB-5). */
  scope: FileScope;
  /** Прогресс 0..100. */
  onProgress?: (percent: number) => void;
  signal?: AbortSignal;
}

function buildUploadUrl(): string {
  const base = (import.meta.env.VITE_API_BASE_URL?.trim() || '/api').replace(/\/+$/, '');
  const origin =
    typeof globalThis.location !== 'undefined' && globalThis.location.origin
      ? globalThis.location.origin
      : 'http://localhost';
  return new URL(`${base}/files`, origin).toString();
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

function xhrUpload(
  file: File,
  options: UploadFileOptions,
  token: string | null,
): Promise<FileMeta> {
  return new Promise<FileMeta>((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open('POST', buildUploadUrl());
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
    form.append('scope', options.scope);
    request.send(form);
  });
}

/** Загружает файл, при 401 делает refresh и повторяет попытку один раз. */
export async function uploadFile(file: File, options: UploadFileOptions): Promise<FileMeta> {
  try {
    return await xhrUpload(file, options, getAccessToken());
  } catch (error) {
    if (error instanceof ApiError && error.status === 401) {
      const token = await refreshSession();
      if (token) return xhrUpload(file, options, token);
    }
    if (error instanceof ApiError) notifyApiError(error);
    throw error;
  }
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
