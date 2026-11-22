/**
 * Комментарии заявки: список, добавление с файлом, оптимистичный откат (TICKET-14).
 */
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, delay, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import type * as UploadModule from '@/api/upload';
import { registerToastApi } from '@/app/toast';
import { db } from '@/mocks/db';
import { server } from '@/mocks/server';
import { USER_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { TicketComments } from '../components/TicketComments';

// В jsdom MSW не разбирает multipart/FormData, поэтому подменяем загрузчик:
// сам FileUploader покрыт отдельными тестами (лимиты/прогресс/drag&drop).
vi.mock('@/api/upload', async (importOriginal) => {
  const actual = await importOriginal<typeof UploadModule>();
  return {
    ...actual,
    uploadFile: async (file: File) => ({
      id: 3,
      filename: 'течь.png',
      content_type: file.type || 'image/png',
      size_bytes: file.size,
      media_kind: 'image' as const,
      url: '/api/files/3',
      created_at: new Date().toISOString(),
    }),
  };
});

const userAuth = makeAuthValue({ user: USER_AUTH });

function renderComments(ticketId = 2, canComment = true) {
  return renderWithProviders(<TicketComments ticketId={ticketId} canComment={canComment} />, {
    auth: userAuth,
  });
}

function makeImageFile(name = 'photo.png'): File {
  return new File(['image-bytes'], name, { type: 'image/png' });
}

describe('TicketComments', () => {
  it('показывает существующие комментарии заявки', async () => {
    authenticateAs(2);
    renderComments();

    expect(await screen.findByText(/Нужна замена сифона/)).toBeInTheDocument();
    expect(screen.getByText(/жду мастера после 15:00/)).toBeInTheDocument();
    expect(screen.getByText('Комментарии')).toBeInTheDocument();
  });

  it('добавляет комментарий с вложением и отправляет file_ids', async () => {
    authenticateAs(2);
    renderComments();

    await screen.findByText(/Нужна замена сифона/);

    fireEvent.change(screen.getByTestId('file-uploader-input'), {
      target: { files: [makeImageFile('течь.png')] },
    });
    // Ждём именно завершённую загрузку (появляется FileMeta в списке).
    await waitFor(() => expect(screen.getByTestId(/^file-uploader-item-/)).toBeInTheDocument());

    await userEvent.type(screen.getByTestId('comment-input'), 'Фото прилагаю');
    await userEvent.click(screen.getByTestId('comment-submit'));

    await waitFor(() => expect(screen.getByText('Фото прилагаю')).toBeInTheDocument());
    await waitFor(() =>
      expect(db.comments.filter((comment) => comment.ticket_id === 2)).toHaveLength(3),
    );

    const created = db.comments.filter((comment) => comment.ticket_id === 2).at(-1)!;
    expect(created.body).toBe('Фото прилагаю');
    expect(created.file_ids).toEqual([3]);
    // Сервер возвращает файл из своего хранилища (файл 3 сида) — превью с его именем.
    expect((await screen.findAllByAltText('protechka-foto.svg')).length).toBeGreaterThan(0);
  });

  it('откатывает оптимистичный комментарий при 403 и возвращает текст', async () => {
    const toast = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() };
    registerToastApi(toast);

    server.use(
      http.post('*/api/tickets/:id/comments', async () => {
        await delay(100);
        return HttpResponse.json(
          { code: 'NOT_EXECUTOR', message: 'forbidden', details: null, request_id: 'r-403' },
          { status: 403 },
        );
      }),
    );

    authenticateAs(2);
    renderComments();
    await screen.findByText(/Нужна замена сифона/);

    await userEvent.type(screen.getByTestId('comment-input'), 'Временный комментарий');
    await userEvent.click(screen.getByTestId('comment-submit'));

    // Оптимистично текст появился в списке.
    expect(await screen.findByText('Временный комментарий')).toBeInTheDocument();

    // Сервер отказал: комментарий исчез из списка, текст вернулся в поле ввода.
    await waitFor(() =>
      expect(
        within(screen.getByTestId('comments-list')).queryByText('Временный комментарий'),
      ).not.toBeInTheDocument(),
    );
    expect(screen.getByTestId('comment-input')).toHaveValue('Временный комментарий');
    expect(toast.error).toHaveBeenCalledWith('Править заявку может только её исполнитель');
    expect(db.comments.filter((comment) => comment.ticket_id === 2)).toHaveLength(2);
  });

  it('не показывает форму комментария без прав', async () => {
    authenticateAs(2);
    renderComments(2, false);

    expect(await screen.findByTestId('comment-forbidden')).toBeInTheDocument();
    expect(screen.queryByTestId('comment-form')).not.toBeInTheDocument();
  });
});
