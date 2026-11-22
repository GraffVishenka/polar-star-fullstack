/**
 * ExportButton — кнопка скачивания документов (Word/Excel) через API (DOC-1..DOC-3, Ф8).
 *
 * Поведение (см. `useFileDownload`):
 *  - состояния загрузки/ошибки; повторное нажатие во время запроса игнорируется;
 *  - имя файла берётся из `Content-Disposition`, иначе — из `filename`/пути;
 *  - пустой/битый ответ не создаёт «пустой» файл (attachment, DOC-3);
 *  - 422/404/403/500 и таймаут объясняются по-русски, для 5xx — тост с `request_id`.
 */
import type { ReactNode } from 'react';
import { Button, Tooltip } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';
import type { ApiError } from '@/api/errors';
import { DEFAULT_DOWNLOAD_TIMEOUT_MS, filenameFromPath, saveBlob } from './exports/download';
import { useFileDownload, type DownloadErrorMapper } from './exports/useFileDownload';

/** Переэкспорт утилит для обратной совместимости (Ф2/Ф5 используют их из этого модуля). */
export { filenameFromPath, saveBlob, DEFAULT_DOWNLOAD_TIMEOUT_MS };

export interface ExportButtonProps {
  /** Путь API, например `/tickets/12/export.docx`. */
  path: string;
  /** Имя файла, если сервер не прислал `Content-Disposition`. */
  filename?: string;
  label?: string;
  icon?: ReactNode;
  type?: 'primary' | 'default' | 'dashed' | 'text' | 'link';
  size?: 'small' | 'middle' | 'large';
  block?: boolean;
  disabled?: boolean;
  /** Публичная карточка `/t/{token}`: скачивание без Bearer. */
  skipAuth?: boolean;
  /** Пояснение при ошибке (tooltip). */
  errorHint?: ReactNode;
  /** Таймаут запроса, мс (по умолчанию 120 с). */
  timeoutMs?: number;
  /** Свой маппинг ошибок (например, ведомость/статистика). */
  errorMessage?: DownloadErrorMapper;
  /** Текст успешного тоста (второй аргумент — имя файла). */
  successMessage?: string;
  onError?: (error: ApiError) => void;
  onSuccess?: (filename: string) => void;
  className?: string;
}

export function ExportButton({
  path,
  filename,
  label = 'Скачать',
  icon = <DownloadOutlined />,
  type = 'default',
  size = 'middle',
  block = false,
  disabled = false,
  skipAuth = false,
  errorHint,
  timeoutMs = DEFAULT_DOWNLOAD_TIMEOUT_MS,
  errorMessage,
  successMessage,
  onError,
  onSuccess,
  className,
}: ExportButtonProps) {
  const { download, loading, canDownload } = useFileDownload({
    path,
    filename,
    skipAuth,
    timeoutMs,
    errorMessage,
    successMessage,
    onSuccess,
    onError,
  });

  const button = (
    <Button
      className={className}
      type={type}
      size={size}
      block={block}
      icon={icon}
      loading={loading}
      disabled={disabled || !canDownload}
      onClick={() => void download()}
      data-testid="export-button"
    >
      {label}
    </Button>
  );

  return errorHint ? <Tooltip title={errorHint}>{button}</Tooltip> : button;
}
