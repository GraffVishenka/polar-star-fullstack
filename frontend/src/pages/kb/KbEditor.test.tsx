/** Тесты TipTap-редактора БЗ: вставка медиа (KB-5), сохранение и dirty-флаг. */
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { FileMeta } from '@/api/types';
import { renderWithProviders } from '@/test/utils';
import { KbEditor } from './components/KbEditor';

function makeFileMeta(overrides: Partial<FileMeta> = {}): FileMeta {
  return {
    id: 42,
    filename: 'photo.png',
    content_type: 'image/png',
    size_bytes: 1024,
    media_kind: 'image',
    url: '/api/files/42',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('KbEditor', () => {
  it('вставляет изображение между абзацами через загрузку scope=kb и сохраняет ссылку /api/files/{id}', async () => {
    const upload = vi.fn(async () => makeFileMeta());
    const onSave = vi.fn();
    const user = userEvent.setup();

    renderWithProviders(
      <KbEditor
        initialTitle="Как создать заявку"
        initialHtml="<p>Первый абзац</p><p>Второй абзац</p>"
        upload={upload}
        onSave={onSave}
        onCancel={() => {}}
      />,
    );

    await waitFor(() => expect(screen.getByTestId('kb-editor-body')).toBeInTheDocument());

    const input = screen.getByTestId('kb-image-input') as HTMLInputElement;
    const file = new File(['fake-image-bytes'], 'photo.png', { type: 'image/png' });
    await user.upload(input, file);

    await waitFor(() =>
      expect(upload).toHaveBeenCalledWith(file, expect.objectContaining({ scope: 'kb' })),
    );
    // Узел изображения попал в документ TipTap.
    await waitFor(() => {
      const image = document.querySelector('.ProseMirror img');
      expect(image?.getAttribute('src')).toBe('/api/files/42');
    });

    await user.click(screen.getByTestId('kb-editor-save'));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const payload = onSave.mock.calls[0]![0] as { title: string; content_html: string };
    expect(payload.title).toBe('Как создать заявку');
    expect(payload.content_html).toContain('<img src="/api/files/42"');
    // Медиа вставлено отдельным блоком — текст статьи сохранён.
    expect(payload.content_html).toContain('Первый абзац');
    expect(payload.content_html).toContain('Второй абзац');
  });

  it('вставляет видео как <video><source> и предупреждает о неверном типе файла', async () => {
    const upload = vi.fn(async () =>
      makeFileMeta({
        id: 7,
        filename: 'instrukciya.mp4',
        content_type: 'video/mp4',
        media_kind: 'video',
        url: '/api/files/7',
      }),
    );
    const onSave = vi.fn();
    const user = userEvent.setup();

    renderWithProviders(
      <KbEditor
        initialTitle="Видео"
        initialHtml="<p>Текст до видео</p>"
        upload={upload}
        onSave={onSave}
        onCancel={() => {}}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('kb-editor-body')).toBeInTheDocument());

    // Картинка через «Вставить видео» — локальная ошибка без запроса на сервер.
    fireEvent.change(screen.getByTestId('kb-video-input'), {
      target: { files: [new File(['x'], 'photo.png', { type: 'image/png' })] },
    });
    await waitFor(() =>
      expect(screen.getByTestId('kb-editor-local-error')).toHaveTextContent('Выберите видео'),
    );
    expect(upload).not.toHaveBeenCalled();

    fireEvent.change(screen.getByTestId('kb-video-input'), {
      target: { files: [new File(['fake-video-bytes'], 'instrukciya.mp4', { type: 'video/mp4' })] },
    });
    await waitFor(() =>
      expect(upload).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'instrukciya.mp4' }),
        expect.objectContaining({ scope: 'kb' }),
      ),
    );
    await waitFor(() =>
      expect(document.querySelector('.ProseMirror video source')?.getAttribute('src')).toBe(
        '/api/files/7',
      ),
    );

    await user.click(screen.getByTestId('kb-editor-save'));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const payload = onSave.mock.calls[0]![0] as { content_html: string };
    expect(payload.content_html).toContain('<video');
    expect(payload.content_html).toContain('<source src="/api/files/7"');
  });

  it('сообщает о несохранённых изменениях (для beforeunload и блокировки перехода)', async () => {
    const onDirtyChange = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(
      <KbEditor
        initialTitle="Статья"
        initialHtml="<p>Текст</p>"
        onDirtyChange={onDirtyChange}
        onSave={() => {}}
        onCancel={() => {}}
      />,
    );

    expect(screen.queryByTestId('kb-editor-dirty')).toBeNull();
    await waitFor(() => expect(screen.getByTestId('kb-editor-title')).toBeInTheDocument());
    const titleInput = screen.getByTestId('kb-editor-title');
    await user.clear(titleInput);
    await user.type(titleInput, 'Новый заголовок');

    await waitFor(() => expect(screen.getByTestId('kb-editor-dirty')).toBeInTheDocument());
    expect(onDirtyChange).toHaveBeenLastCalledWith(true);
  });

  it('сохраняет уже вставленные в статью фото и видео при повторном открытии', async () => {
    const onSave = vi.fn();
    const user = userEvent.setup();
    const initialHtml =
      '<p>Первый абзац</p><img src="/api/files/5" alt="схема">' +
      '<video controls preload="metadata" title="ролик"><source src="/api/files/6" type="video/mp4"></video>' +
      '<p>Последний абзац</p>';

    renderWithProviders(
      <KbEditor
        initialTitle="С медиа"
        initialHtml={initialHtml}
        onSave={onSave}
        onCancel={() => {}}
      />,
    );

    await waitFor(() => expect(document.querySelector('.ProseMirror img')).not.toBeNull());
    expect(document.querySelector('.ProseMirror video source')?.getAttribute('src')).toBe(
      '/api/files/6',
    );

    await user.click(screen.getByTestId('kb-editor-save'));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    const payload = onSave.mock.calls[0]![0] as { content_html: string };
    expect(payload.content_html).toContain('<img src="/api/files/5"');
    expect(payload.content_html).toContain('<source src="/api/files/6"');
    expect(payload.content_html).toContain('Последний абзац');
  });

  it('показывает ошибку сохранения с сервера', () => {
    renderWithProviders(
      <KbEditor
        initialTitle="Статья"
        initialHtml="<p>Текст</p>"
        error={new Error('Нельзя вложить раздел')}
        onSave={() => {}}
        onCancel={() => {}}
      />,
    );
    expect(screen.getByTestId('kb-editor-server-error')).toHaveTextContent('Нельзя вложить раздел');
  });
});
