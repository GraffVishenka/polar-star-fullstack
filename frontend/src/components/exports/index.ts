/**
 * Экспорт документов (Ф8): общий механизм скачивания, хуки и готовые кнопки.
 *
 * `fe-2` (статистика/профиль) и `fe-3` (оборудование/склады) подключают эти
 * модули, не дублируя обработку blob/имен/ошибок.
 */
export {
  DEFAULT_DOWNLOAD_TIMEOUT_MS,
  defaultDownloadErrorMessage,
  filenameFromPath,
  saveBlob,
} from './download';
export { useFileDownload } from './useFileDownload';
export type {
  DownloadErrorMapper,
  UseFileDownloadOptions,
  UseFileDownloadResult,
} from './useFileDownload';
export {
  INVENTORY_POSITIONS_LIMIT,
  inventoryErrorMessage,
  useInventoryExport,
} from './useInventoryExport';
export type { UseInventoryExportOptions, UseInventoryExportResult } from './useInventoryExport';
export { InventoryExportButton } from './InventoryExportButton';
export type { InventoryExportButtonProps } from './InventoryExportButton';
export { statsErrorMessage, useStatsExport } from './useStatsExport';
export type { UseStatsExportOptions, UseStatsExportResult } from './useStatsExport';
export { StatsExportButton } from './StatsExportButton';
export type { StatsExportButtonProps } from './StatsExportButton';
