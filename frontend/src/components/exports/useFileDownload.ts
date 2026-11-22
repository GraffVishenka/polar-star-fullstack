/**
 * `useFileDownload` — единый механизм скачивания документов (DOC-3, Ф8).
 *
 * Требования:
 *  - состояние загрузки/ошибки; повторный вызов во время запроса игнорируется;
 *  - имя файла из `Content-Disposition`, иначе — из пути или переданное явно;
 *  - пустой ответ не создаёт «пустой» файл;
 *  - тост с `request_id` для 5xx (NFR-4), понятный текст для 403/404/422/таймаута;
 *  - ошибка сети не оставляет файл.
 */
import { useCallback, useRef, useState } from 'react';
import { apiDownload } from '@/api/client';
import { ApiError } from '@/api/errors';
import { toast } from '@/app/toast';
import {
  DEFAULT_DOWNLOAD_TIMEOUT_MS,
  defaultDownloadErrorMessage,
  filenameFromPath,
  saveBlob,
} from './download';

export type DownloadErrorMapper = (error: ApiError) => string;

export interface UseFileDownloadOptions {
  /** Путь API (относительно `/api`), например `/tickets/12/export.docx`. */
  path: string;
  /** Имя файла, если сервер не прислал `Content-Disposition`. */
  filename?: string;
  /** Публичный эндпоинт: без Bearer и refresh (карточка `/t/{token}`). */
  skipAuth?: boolean;
  timeoutMs?: number;
  /** Заголовок успешного тоста (второй аргумент — имя файла). */
  successMessage?: string;
  /** Маппинг ошибки в русский текст; по умолчанию 422 → «слишком много позиций». */
  errorMessage?: DownloadErrorMapper;
  /** Отключить тосты (композиция/тесты). */
  notify?: boolean;
  onSuccess?: (filename: string) => void;
  onError?: (error: ApiError) => void;
}

export interface UseFileDownloadResult {
  download: () => Promise<void>;
  loading: boolean;
  error: ApiError | null;
  /** Имя последнего успешно скачанного файла. */
  filename: string | null;
  /** Есть ли что скачивать (пустой путь = недоступно). */
  canDownload: boolean;
}

function toApiError(cause: unknown): ApiError {
  if (cause instanceof ApiError) return cause;
  return new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true, cause });
}

export function useFileDownload(options: UseFileDownloadOptions): UseFileDownloadResult {
  const {
    path,
    filename: preferredFilename,
    skipAuth = false,
    timeoutMs = DEFAULT_DOWNLOAD_TIMEOUT_MS,
    successMessage = 'Файл готов',
    errorMessage = defaultDownloadErrorMessage,
    notify = true,
    onSuccess,
    onError,
  } = options;

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiError | null>(null);
  const [filename, setFilename] = useState<string | null>(null);
  // Ref защищает от двойного клика до перерисовки (Ф8: без дублей запросов).
  const inFlight = useRef(false);

  const download = useCallback(async () => {
    if (!path || inFlight.current) return;
    inFlight.current = true;
    setLoading(true);
    setError(null);
    try {
      const result = await apiDownload(path, { skipAuth, timeoutMs });
      if (!result.blob.size) {
        throw new ApiError({ code: 'INTERNAL_ERROR', status: 500, message: 'Пустой файл' });
      }
      const name = preferredFilename ?? result.filename ?? filenameFromPath(path);
      saveBlob(result.blob, name);
      setFilename(name);
      if (notify) toast.success(successMessage, name);
      onSuccess?.(name);
    } catch (cause) {
      const apiError = toApiError(cause);
      setError(apiError);
      if (notify) {
        const message = errorMessage(apiError);
        // 5xx и сетевые сбои — с request_id, чтобы поддержка нашла запрос в логах.
        if (apiError.requestId && (apiError.isServerError || apiError.isNetworkError)) {
          toast.error(message, `Код запроса: ${apiError.requestId}`);
        } else {
          toast.error(message);
        }
      }
      onError?.(apiError);
    } finally {
      inFlight.current = false;
      setLoading(false);
    }
  }, [
    errorMessage,
    notify,
    onError,
    onSuccess,
    path,
    preferredFilename,
    skipAuth,
    successMessage,
    timeoutMs,
  ]);

  return {
    download,
    loading,
    error,
    filename,
    canDownload: Boolean(path),
  };
}
