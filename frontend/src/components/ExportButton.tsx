/**
 * ExportButton — скачивание документов (Word/Excel) через API (DOC-1..DOC-3, Ф8).
 *
 * Требования:
 *  - состояния загрузки/ошибки; повторное нажатие во время запроса игнорируется,
 *    чтобы не плодить дубли;
 *  - имя файла берётся из `Content-Disposition`, иначе — из пути;
 *  - пустой/битый ответ не создаёт «пустой» файл;
 *  - 422 (слишком много позиций) объясняется человеческим текстом.
 */
import { useCallback, useState, type ReactNode } from 'react';
import { Button, Tooltip } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';
import { apiDownload } from '@/api/client';
import { ApiError } from '@/api/errors';
import { toast } from '@/app/toast';

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
  /** Пояснение при ошибке. */
  errorHint?: ReactNode;
  onError?: (error: ApiError) => void;
  onSuccess?: (filename: string) => void;
  className?: string;
}

/** Имя файла из пути: `/tickets/12/export.docx` → `export.docx`. */
export function filenameFromPath(path: string): string {
  const clean = path.split('?')[0]?.replace(/\/+$/, '') ?? '';
  const last = clean.split('/').pop();
  return last && last.includes('.') ? last : 'download';
}

/** Сохраняет blob как файл средствами браузера. */
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

function exportErrorMessage(error: ApiError): string {
  if (error.code === 'VALIDATION_ERROR' && error.status === 422) {
    return 'Слишком много позиций для выгрузки — сократите выборку';
  }
  return error.userMessage;
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
  onError,
  onSuccess,
  className,
}: ExportButtonProps) {
  const [loading, setLoading] = useState(false);

  const handleClick = useCallback(async () => {
    if (loading) return;
    setLoading(true);
    try {
      const result = await apiDownload(path, { skipAuth });
      if (!result.blob.size)
        throw new ApiError({ code: 'INTERNAL_ERROR', status: 500, message: 'Пустой файл' });
      const name = filename ?? result.filename ?? filenameFromPath(path);
      saveBlob(result.blob, name);
      toast.success('Файл готов', name);
      onSuccess?.(name);
    } catch (cause) {
      const error =
        cause instanceof ApiError
          ? cause
          : new ApiError({ code: 'NETWORK_ERROR', status: 0, isNetworkError: true, cause });
      toast.error(exportErrorMessage(error));
      onError?.(error);
    } finally {
      setLoading(false);
    }
  }, [filename, loading, onError, onSuccess, path, skipAuth]);

  const button = (
    <Button
      className={className}
      type={type}
      size={size}
      block={block}
      icon={icon}
      loading={loading}
      disabled={disabled}
      onClick={() => void handleClick()}
      data-testid="export-button"
    >
      {label}
    </Button>
  );

  return errorHint ? <Tooltip title={errorHint}>{button}</Tooltip> : button;
}
