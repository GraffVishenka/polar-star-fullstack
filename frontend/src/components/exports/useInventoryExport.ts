/**
 * `useInventoryExport` — ведомость помещения в Excel (EQ-8, DOC-2).
 *
 * Отдаёт всё для кнопки/меню: путь `GET /api/warehouses/{id}/inventory.xlsx`,
 * состояние загрузки/ошибки, имя файла из `Content-Disposition` (fallback
 * `inventory_{id}.xlsx`), понятные тексты для 422 (>1000 позиций) и 404.
 *
 * Страницу `/equipment` не трогаем: fe-3 подключает `InventoryExportButton`
 * (или сам хук) на странице помещения.
 */
import { warehouseInventoryPath } from '@/api/equipment';
import type { ApiError } from '@/api/errors';
import { useFileDownload, type UseFileDownloadResult } from './useFileDownload';

/** EQ-8: максимум позиций в одной ведомости. */
export const INVENTORY_POSITIONS_LIMIT = 1000;

export function inventoryErrorMessage(error: ApiError): string {
  switch (error.status) {
    case 422:
      return `В помещении больше ${INVENTORY_POSITIONS_LIMIT} позиций — ведомость не формируется, выгрузите по частям`;
    case 404:
      return 'Помещение не найдено';
    case 403:
      return 'Нет прав на выгрузку ведомости помещения';
    default:
      return error.userMessage;
  }
}

export interface UseInventoryExportOptions {
  onSuccess?: (filename: string) => void;
  onError?: (error: ApiError) => void;
  /** Отключить тосты (например, в тестах или при своей нотификации). */
  notify?: boolean;
  timeoutMs?: number;
}

export interface UseInventoryExportResult extends UseFileDownloadResult {
  /** Валидный id помещения; null — выгрузка недоступна. */
  warehouseId: number | null;
}

function isValidWarehouseId(value: number | null | undefined): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value > 0;
}

export function useInventoryExport(
  warehouseId: number | null | undefined,
  options: UseInventoryExportOptions = {},
): UseInventoryExportResult {
  const valid = isValidWarehouseId(warehouseId);
  const filename = valid ? `inventory_${warehouseId}.xlsx` : undefined;

  const download = useFileDownload({
    path: valid ? warehouseInventoryPath(warehouseId) : '',
    filename,
    errorMessage: inventoryErrorMessage,
    successMessage: 'Ведомость готова',
    onSuccess: options.onSuccess,
    onError: options.onError,
    notify: options.notify,
    timeoutMs: options.timeoutMs,
  });

  return {
    ...download,
    // Без валидного id помещения выгрузка недоступна (кнопка disabled).
    canDownload: valid && download.canDownload,
    warehouseId: valid ? warehouseId : null,
  };
}
