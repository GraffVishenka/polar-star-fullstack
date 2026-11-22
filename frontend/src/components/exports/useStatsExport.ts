/**
 * `useStatsExport` — Excel-выгрузка статистики (STATS-3, DOC-2).
 *
 * Параметры: `user_id` (свой — всем, чужой — только ADMIN), `building_id`
 * (только ADMIN), период `from`/`to` (по `completed_at`, §3.8). Имя файла —
 * `stat_{login}_{from}_{to}.xlsx` из `Content-Disposition`; 403/422 объясняются.
 *
 * Страницы статистики делает fe-2 (Ф10): подключает `StatsExportButton`
 * или сам хук, страницу не создаём.
 */
import { useMemo } from 'react';
import { statsExportPath } from '@/api/stats';
import type { AdminStatsParams } from '@/api/types';
import type { ApiError } from '@/api/errors';
import { useFileDownload, type UseFileDownloadResult } from './useFileDownload';

export function statsErrorMessage(error: ApiError): string {
  switch (error.status) {
    case 403:
      return 'Чужой отчёт доступен только администратору';
    case 422:
      return 'Слишком много данных для выгрузки — сократите период или выборку';
    default:
      return error.userMessage;
  }
}

export interface UseStatsExportOptions {
  onSuccess?: (filename: string) => void;
  onError?: (error: ApiError) => void;
  notify?: boolean;
  timeoutMs?: number;
}

export interface UseStatsExportResult extends UseFileDownloadResult {
  /** Параметры, с которыми сформирован путь (удобно для отладки/тостов). */
  params: AdminStatsParams;
}

export function useStatsExport(
  params: AdminStatsParams = {},
  options: UseStatsExportOptions = {},
): UseStatsExportResult {
  const path = useMemo(() => statsExportPath(params), [params]);

  const download = useFileDownload({
    path,
    // Имя приходит в Content-Disposition (stat_{login}_{from}_{to}.xlsx).
    filename: undefined,
    errorMessage: statsErrorMessage,
    successMessage: 'Файл статистики готов',
    onSuccess: options.onSuccess,
    onError: options.onError,
    notify: options.notify,
    timeoutMs: options.timeoutMs,
  });

  return { ...download, params };
}
