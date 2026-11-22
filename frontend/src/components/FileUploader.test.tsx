/**
 * FileUploader: лимиты 10 МБ фото / 200 МБ видео, drag&drop, прогресс (Б3, TICKET-14).
 * Загрузчик подменяется, чтобы тест не ходил в сеть.
 */
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import type { FileMeta } from '@/api/types';
import { formatBytes, validateUploadFile } from '@/api/upload';
import { FileUploader, type FileUploaderProps, type UploadHandler } from './FileUploader';

function makeFile(name: string, type: string, size = 1024): File {
  const file = new File(['x'], name, { type });
  Object.defineProperty(file, 'size', { value: size, configurable: true });
  return file;
}

function makeMeta(overrides: Partial<FileMeta> = {}): FileMeta {
  return {
    id: 42,
    filename: 'photo.jpg',
    content_type: 'image/jpeg',
    size_bytes: 1024,
    media_kind: 'image',
    url: '/api/files/42',
    created_at: '2026-03-05T10:00:00Z',
    ...overrides,
  };
}

/** Контролируемый родитель: после загрузки список обновляется через onChange. */
function ControlledUploader(props: Partial<FileUploaderProps> = {}) {
  const [files, setFiles] = useState<FileMeta[]>(props.value ?? []);
  return (
    <FileUploader
      scope="ticket"
      {...props}
      value={files}
      onChange={(next) => {
        setFiles(next);
        props.onChange?.(next);
      }}
    />
  );
}

describe('validateUploadFile', () => {
  it('принимает фото до 10 МБ и видео до 200 МБ', () => {
    expect(validateUploadFile(makeFile('a.png', 'image/png', 9 * 1024 * 1024)).ok).toBe(true);
    expect(validateUploadFile(makeFile('b.mp4', 'video/mp4', 150 * 1024 * 1024)).ok).toBe(true);
  });

  it('отклоняет фото больше 10 МБ и видео больше 200 МБ', () => {
    expect(validateUploadFile(makeFile('big.png', 'image/png', 11 * 1024 * 1024))).toMatchObject({
      ok: false,
      kind: 'image',
      error: expect.stringContaining('10 МБ'),
    });
    expect(validateUploadFile(makeFile('big.mp4', 'video/mp4', 201 * 1024 * 1024))).toMatchObject({
      ok: false,
      kind: 'video',
    });
  });

  it('отклоняет посторонние типы и пустые файлы', () => {
    expect(validateUploadFile(makeFile('virus.exe', 'application/x-msdownload'))).toMatchObject({
      ok: false,
      error: 'Можно загружать только фото и видео',
    });
    expect(validateUploadFile(makeFile('empty.png', 'image/png', 0)).ok).toBe(false);
  });

  it('форматирует размер для списка файлов', () => {
    expect(formatBytes(900)).toBe('900 Б');
    expect(formatBytes(2048)).toBe('2.0 КБ');
    expect(formatBytes(5 * 1024 * 1024)).toBe('5.0 МБ');
  });
});

describe('FileUploader', () => {
  it('не отправляет файл недопустимого типа и объясняет причину', async () => {
    const upload = vi.fn<UploadHandler>();
    render(<FileUploader scope="ticket" upload={upload} />);

    fireEvent.change(screen.getByTestId('file-uploader-input'), {
      target: { files: [makeFile('setup.exe', 'application/x-msdownload')] },
    });

    expect(await screen.findByTestId('file-uploader-error')).toHaveTextContent(
      'Можно загружать только фото и видео',
    );
    expect(upload).not.toHaveBeenCalled();
  });

  it('не отправляет слишком большое фото (10 МБ)', async () => {
    const upload = vi.fn<UploadHandler>();
    render(<FileUploader scope="ticket" upload={upload} />);

    fireEvent.change(screen.getByTestId('file-uploader-input'), {
      target: { files: [makeFile('huge.png', 'image/png', 11 * 1024 * 1024)] },
    });

    expect(await screen.findByTestId('file-uploader-error')).toHaveTextContent('10 МБ');
    expect(upload).not.toHaveBeenCalled();
  });

  it('показывает прогресс и отдаёт загруженный файл через onChange', async () => {
    const meta = makeMeta();
    let resolveUpload!: (value: FileMeta) => void;
    const upload = vi.fn<UploadHandler>(
      (_file, options) =>
        new Promise<FileMeta>((resolve) => {
          options.onProgress?.(40);
          resolveUpload = resolve;
        }),
    );
    const onChange = vi.fn();

    render(<ControlledUploader upload={upload} onChange={onChange} />);
    fireEvent.change(screen.getByTestId('file-uploader-input'), {
      target: { files: [makeFile('photo.jpg', 'image/jpeg')] },
    });

    await waitFor(() => expect(screen.getByTestId('file-uploader-progress')).toBeInTheDocument());
    expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '40');
    expect(upload).toHaveBeenCalledWith(
      expect.objectContaining({ name: 'photo.jpg' }),
      expect.objectContaining({ scope: 'ticket' }),
    );

    await act(async () => {
      resolveUpload(meta);
    });

    await waitFor(() => expect(onChange).toHaveBeenCalledWith([meta]));
    expect(await screen.findByTestId('file-uploader-item-42')).toHaveTextContent('photo.jpg');
  });

  it('показывает русскую ошибку, если сервер отклонил файл', async () => {
    const upload = vi.fn<UploadHandler>(async () => {
      const { ApiError } = await import('@/api/errors');
      throw new ApiError({ code: 'PAYLOAD_TOO_LARGE', status: 413 });
    });

    render(<FileUploader scope="ticket" upload={upload} />);
    fireEvent.change(screen.getByTestId('file-uploader-input'), {
      target: { files: [makeFile('photo.jpg', 'image/jpeg')] },
    });

    await waitFor(() => expect(screen.getByText('Файл слишком большой')).toBeInTheDocument());
  });

  it('ограничивает число вложений maxCount', async () => {
    const onChange = vi.fn();
    const upload = vi.fn<UploadHandler>(async () => makeMeta());
    render(
      <FileUploader
        scope="ticket"
        upload={upload}
        maxCount={1}
        value={[makeMeta({ id: 1 })]}
        onChange={onChange}
      />,
    );

    fireEvent.change(screen.getByTestId('file-uploader-input'), {
      target: { files: [makeFile('second.jpg', 'image/jpeg')] },
    });

    expect(await screen.findByTestId('file-uploader-error')).toHaveTextContent(
      'не больше 1 файлов',
    );
    expect(upload).not.toHaveBeenCalled();
  });

  it('удаляет вложение из списка', async () => {
    const onChange = vi.fn();
    render(
      <FileUploader
        scope="ticket"
        upload={vi.fn<UploadHandler>()}
        value={[makeMeta({ id: 7 })]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByTestId('file-uploader-remove-7'));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it('принимает файлы drag&drop', async () => {
    const upload = vi.fn<UploadHandler>(async () => makeMeta());
    render(<FileUploader scope="ticket" upload={upload} onChange={vi.fn()} />);

    fireEvent.drop(screen.getByTestId('file-uploader-dropzone'), {
      dataTransfer: { files: [makeFile('drop.png', 'image/png')] },
    });

    await waitFor(() => expect(upload).toHaveBeenCalledTimes(1));
  });
});
