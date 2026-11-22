/**
 * Общие утилиты скачивания документов (Word/Excel): имя файла, сохранение blob,
 * таймаут по умолчанию. Вынесено из `ExportButton`, чтобы хуки экспорта
 * (`useFileDownload`, `useInventoryExport`, `useStatsExport`) не зависели от React-компонента.
 */

/** Таймаут скачивания: документы формируются на сервере (NFR-1 — до 2 с на 1000 строк). */
export const DEFAULT_DOWNLOAD_TIMEOUT_MS = 120_000;

/** Имя файла из пути: `/tickets/12/export.docx` → `export.docx`. */
export function filenameFromPath(path: string): string {
  const clean = path.split('?')[0]?.replace(/\/+$/, '') ?? '';
  const last = clean.split('/').pop();
  return last && last.includes('.') ? last : 'download';
}

/** Сохраняет blob как файл средствами браузера (attachment, DOC-3). */
export function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.style.display = 'none';
  document.body.appendChild(link);
  link.click();
  link.remove();
  // Даём браузеру начать скачивание, затем освобождаем память.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Текст для 422 по умолчанию: чаще всего это «слишком много позиций». */
export function defaultDownloadErrorMessage(error: {
  status: number;
  userMessage: string;
}): string {
  if (error.status === 422) return 'Слишком много позиций для выгрузки — сократите выборку';
  return error.userMessage;
}
