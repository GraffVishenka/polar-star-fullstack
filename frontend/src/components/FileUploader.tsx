/**
 * FileUploader — загрузка фото/видео с drag&drop и прогрессом (TICKET-14, Б3).
 *
 * Правила:
 *  - только фото ≤ 10 МБ и видео ≤ 200 МБ; проверка размера/типа ДО запроса,
 *    сервер повторно проверяет magic bytes (Б3);
 *  - максимальное число вложений ограничено (`maxCount`, TICKET-14: до 5);
 *  - прогресс отдаётся XHR (fetch не умеет upload progress);
 *  - в тестах `upload` можно подменить, чтобы не ходить в сеть.
 */
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
} from 'react';
import { Alert, Button, Flex, Progress, Space, Typography } from 'antd';
import {
  CloudUploadOutlined,
  DeleteOutlined,
  FileImageOutlined,
  VideoCameraOutlined,
} from '@ant-design/icons';
import { ApiError } from '@/api/errors';
import type { FileMeta, FileScope } from '@/api/types';
import {
  formatBytes,
  uploadFile as defaultUploadFile,
  validateUploadFile,
  type UploadKind,
} from '@/api/upload';

export type UploadHandler = (
  file: File,
  options: {
    scope: FileScope;
    onProgress?: (percent: number) => void;
    signal?: AbortSignal;
  },
) => Promise<FileMeta>;

export interface FileUploaderProps {
  /** Область загрузки: вложения заявки или медиа статьи БЗ. */
  scope: FileScope;
  /** Уже загруженные файлы (контролируемый список). */
  value?: FileMeta[];
  onChange?: (files: FileMeta[]) => void;
  /** Сколько вложений допустимо всего (TICKET-14: до 5). */
  maxCount?: number;
  accept?: UploadKind | 'any';
  disabled?: boolean;
  /** Подмена загрузчика в тестах. */
  upload?: UploadHandler;
  hint?: string;
  className?: string;
}

interface PendingItem {
  uid: string;
  name: string;
  size: number;
  kind: UploadKind;
  progress: number;
  status: 'uploading' | 'error';
  error?: string;
}

function matchesAccept(kind: UploadKind, accept: UploadKind | 'any'): boolean {
  return accept === 'any' || accept === kind;
}

export function FileUploader({
  scope,
  value,
  onChange,
  maxCount = 5,
  accept = 'any',
  disabled = false,
  upload = defaultUploadFile,
  hint,
  className,
}: FileUploaderProps) {
  const files = useMemo(() => value ?? [], [value]);
  const inputId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const [pending, setPending] = useState<PendingItem[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [isDragging, setDragging] = useState(false);

  // Актуальный список нужен в async-колбэке загрузки (иначе теряются файлы,
  // добавленные в параллель).
  const filesRef = useRef<FileMeta[]>(files);
  useEffect(() => {
    filesRef.current = files;
  }, [files]);

  const total = files.length + pending.length;
  const limitReached = total >= maxCount;

  const startUpload = useCallback(
    async (file: File, uid: string) => {
      try {
        const meta = await upload(file, {
          scope,
          onProgress: (percent) => {
            setPending((current) =>
              current.map((item) => (item.uid === uid ? { ...item, progress: percent } : item)),
            );
          },
        });
        setPending((current) => current.filter((item) => item.uid !== uid));
        const next = [...filesRef.current, meta];
        filesRef.current = next;
        onChange?.(next);
      } catch (cause) {
        const message = cause instanceof ApiError ? cause.userMessage : 'Не удалось загрузить файл';
        setPending((current) =>
          current.map((item) =>
            item.uid === uid ? { ...item, status: 'error', error: message, progress: 0 } : item,
          ),
        );
      }
    },
    [onChange, scope, upload],
  );

  const addFiles = useCallback(
    (incoming: FileList | File[]) => {
      setError(null);
      const list = Array.from(incoming);
      const available = Math.max(0, maxCount - filesRef.current.length - pending.length);
      if (available === 0) {
        setError(`Можно прикрепить не больше ${maxCount} файлов`);
        return;
      }

      const accepted: Array<{ file: File; kind: UploadKind }> = [];
      for (const file of list.slice(0, available)) {
        const validation = validateUploadFile(file);
        if (!validation.ok || !validation.kind) {
          setError(validation.error ?? 'Файл не подходит');
          continue;
        }
        if (!matchesAccept(validation.kind, accept)) {
          setError(
            accept === 'image' ? 'Можно загружать только фото' : 'Можно загружать только видео',
          );
          continue;
        }
        accepted.push({ file, kind: validation.kind });
      }

      if (list.length > available) {
        setError(`Можно прикрепить не больше ${maxCount} файлов`);
      }

      accepted.forEach(({ file, kind }, index) => {
        const uid = `${Date.now()}-${index}-${file.name}`;
        setPending((current) => [
          ...current,
          {
            uid,
            name: file.name,
            size: file.size,
            kind,
            progress: 0,
            status: 'uploading',
          },
        ]);
        void startUpload(file, uid);
      });
    },
    [accept, maxCount, pending.length, startUpload],
  );

  const handleInputChange = (event: ChangeEvent<HTMLInputElement>) => {
    if (event.target.files?.length) addFiles(event.target.files);
    event.target.value = '';
  };

  const handleDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragging(false);
    if (disabled) return;
    if (event.dataTransfer.files?.length) addFiles(event.dataTransfer.files);
  };

  const handleRemove = (id: number) => {
    const next = filesRef.current.filter((file) => file.id !== id);
    filesRef.current = next;
    onChange?.(next);
  };

  return (
    <div className={className} data-testid="file-uploader">
      <div
        className={`file-uploader__dropzone${isDragging ? ' file-uploader__dropzone--active' : ''}${
          disabled ? ' file-uploader__dropzone--disabled' : ''
        }`}
        data-testid="file-uploader-dropzone"
        onDragOver={(event) => {
          event.preventDefault();
          if (!disabled) setDragging(true);
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={handleDrop}
        onClick={() => {
          if (!disabled && !limitReached) inputRef.current?.click();
        }}
        role="button"
        tabIndex={disabled ? -1 : 0}
        aria-label="Загрузить фото или видео"
        onKeyDown={(event) => {
          if ((event.key === 'Enter' || event.key === ' ') && !disabled && !limitReached) {
            event.preventDefault();
            inputRef.current?.click();
          }
        }}
      >
        <CloudUploadOutlined style={{ fontSize: 24, color: '#1677ff' }} />
        <Typography.Text>
          Перетащите файлы сюда или <Typography.Link>выберите на устройстве</Typography.Link>
        </Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Фото до 10 МБ, видео до 200 МБ. Файлов: {total} из {maxCount}
        </Typography.Text>
        <input
          ref={inputRef}
          id={inputId}
          data-testid="file-uploader-input"
          type="file"
          multiple
          accept={
            accept === 'image' ? 'image/*' : accept === 'video' ? 'video/*' : 'image/*,video/*'
          }
          style={{ display: 'none' }}
          disabled={disabled || limitReached}
          onChange={handleInputChange}
        />
      </div>

      {hint ? (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {hint}
        </Typography.Text>
      ) : null}

      {error ? (
        <Alert
          type="error"
          showIcon
          message={error}
          data-testid="file-uploader-error"
          closable
          onClose={() => setError(null)}
          style={{ marginTop: 8 }}
        />
      ) : null}

      {pending.length ? (
        <Flex vertical gap={6} style={{ marginTop: 8 }}>
          {pending.map((item) => (
            <div key={item.uid} data-testid="file-uploader-pending">
              <Space size={6}>
                {item.kind === 'image' ? <FileImageOutlined /> : <VideoCameraOutlined />}
                <Typography.Text ellipsis style={{ maxWidth: 220 }}>
                  {item.name}
                </Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {formatBytes(item.size)}
                </Typography.Text>
              </Space>
              {item.status === 'uploading' ? (
                <Progress
                  percent={item.progress}
                  size="small"
                  data-testid="file-uploader-progress"
                  aria-label={`Загрузка ${item.name}`}
                  status="active"
                />
              ) : (
                <Typography.Text type="danger" style={{ fontSize: 12 }}>
                  {item.error}
                </Typography.Text>
              )}
            </div>
          ))}
        </Flex>
      ) : null}

      {files.length ? (
        <Flex vertical gap={6} style={{ marginTop: 8 }}>
          {files.map((file) => (
            <Flex
              key={file.id}
              align="center"
              justify="space-between"
              gap={8}
              data-testid={`file-uploader-item-${file.id}`}
              style={{ border: '1px solid #f0f0f0', borderRadius: 8, padding: '6px 8px' }}
            >
              <Space size={6} style={{ minWidth: 0 }}>
                {file.media_kind === 'image' ? <FileImageOutlined /> : <VideoCameraOutlined />}
                <Typography.Text ellipsis style={{ maxWidth: 240, fontSize: 13 }}>
                  {file.filename}
                </Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {formatBytes(file.size_bytes)}
                </Typography.Text>
              </Space>
              <Button
                type="text"
                danger
                size="small"
                icon={<DeleteOutlined />}
                aria-label={`Удалить ${file.filename}`}
                data-testid={`file-uploader-remove-${file.id}`}
                disabled={disabled}
                onClick={(event) => {
                  event.stopPropagation();
                  handleRemove(file.id);
                }}
              />
            </Flex>
          ))}
        </Flex>
      ) : null}
    </div>
  );
}
