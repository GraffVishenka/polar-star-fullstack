/**
 * Публичная карточка `/t/:token` (TICKET-13, PUB-6):
 * поля, дедлайн по серверному `is_overdue`, комментарии с пагинацией,
 * добавление комментария с файлом, Word-выгрузка, 404 для чужого токена.
 */
import { fireEvent, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type * as PublicModule from '@/api/public';
import { registerToastApi } from '@/app/toast';
import { db } from '@/mocks/db';
import { server } from '@/mocks/server';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { PublicTicketPage } from '../PublicTicketPage';

// В jsdom MSW не разбирает multipart/form-data — подменяем публичную загрузку
// на границе API (сам FileUploader покрыт отдельными тестами).
vi.mock('@/api/public', async (importOriginal) => {
  const actual = await importOriginal<typeof PublicModule>();
  return {
    ...actual,
    uploadPublicCommentFile: vi.fn(async () => ({
      id: 3,
      filename: 'течь.png',
      content_type: 'image/png',
      size_bytes: 5,
      media_kind: 'image' as const,
      url: '/api/public/files/3?token=demo-token-2',
      created_at: new Date().toISOString(),
    })),
  };
});

function renderToken(route = '/t/demo-token-1') {
  return renderWithProviders(
    <Routes>
      <Route path="/t/:token" element={<PublicTicketPage />} />
      <Route path="/" element={<div>home page</div>} />
    </Routes>,
    { route, auth: makeAnonymousAuthValue() },
  );
}

function makeComment(id: number, body: string) {
  return {
    id,
    ticket_id: 4,
    author_name: 'Гость',
    author_type: 'anonymous' as const,
    body,
    created_at: '2026-03-01T10:00:00Z',
    files: [],
  };
}

describe('PublicTicketPage', () => {
  beforeEach(() => {
    registerToastApi({ success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() });
  });

  it('показывает поля заявки и существующие комментарии по токену', async () => {
    renderToken();

    expect(await screen.findByTestId('public-ticket-page')).toBeInTheDocument();
    expect(screen.getByText('Заявка №4')).toBeInTheDocument();
    expect(screen.getByText('Центральный офис')).toBeInTheDocument();
    expect(screen.getByText('каб. 12')).toBeInTheDocument();
    expect(screen.getByText('Уборка')).toBeInTheDocument();
    expect(screen.getAllByText('Завершена').length).toBeGreaterThan(0);

    expect(await screen.findByText(/Уборка выполнена, заявка закрыта/)).toBeInTheDocument();
    expect(screen.getByTestId('public-comments-total')).toHaveTextContent('1');
  });

  it('чужой или битый токен отдаёт 404', async () => {
    renderToken('/t/unknown-token');

    expect(await screen.findByTestId('public-not-found')).toBeInTheDocument();
    expect(screen.getByText('Заявка не найдена')).toBeInTheDocument();
    expect(screen.queryByTestId('public-ticket-page')).not.toBeInTheDocument();
  });

  it('показывает просрочку только по серверному is_overdue', async () => {
    server.use(
      http.get('*/api/public/tickets/:token', () =>
        HttpResponse.json({
          id: 77,
          building_id: 1,
          building_name: 'Центральный офис',
          applicant_name: 'Гость',
          cabinet: 'каб. 1',
          problem_type_name: 'Прочее',
          status: 'new',
          status_label: 'Новая',
          status_color: '#1677ff',
          deadline: '2020-01-01',
          is_overdue: true,
          executor_name: null,
          description: 'Просроченная заявка',
          created_at: '2026-01-01T00:00:00Z',
          completed_at: null,
          comments: [],
          files: [],
        }),
      ),
    );

    renderToken('/t/any-token');

    expect(await screen.findByText('Просрочено')).toBeInTheDocument();
  });

  it('подгружает комментарии постранично (50 на страницу по контракту)', async () => {
    server.use(
      http.get('*/api/public/tickets/:token/comments', ({ request }) => {
        const page = Number(new URL(request.url).searchParams.get('page') ?? '1');
        return HttpResponse.json({
          items: [makeComment(page, `Комментарий со страницы ${page}`)],
          total: 2,
          page,
          size: 1,
        });
      }),
    );

    renderToken();

    expect(await screen.findByText('Комментарий со страницы 1')).toBeInTheDocument();
    const more = await screen.findByTestId('public-comments-more');
    expect(more).toHaveTextContent('Показать ещё (1 из 2)');

    await userEvent.click(more);

    expect(await screen.findByText('Комментарий со страницы 2')).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByTestId('public-comments-more')).not.toBeInTheDocument(),
    );
  });

  it('добавляет комментарий с вложением (до 5 файлов)', async () => {
    renderToken('/t/demo-token-2');

    expect(await screen.findByTestId('public-ticket-page')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('file-uploader-input'), {
      target: { files: [new File(['bytes'], 'течь.png', { type: 'image/png' })] },
    });
    await waitFor(() => expect(screen.getByTestId(/^file-uploader-item-/)).toBeInTheDocument());

    await userEvent.type(screen.getByTestId('public-comment-author'), 'Посетитель');
    await userEvent.type(screen.getByTestId('public-comment-input'), 'Прикладываю фото');
    await userEvent.click(screen.getByTestId('public-comment-submit'));

    expect(await screen.findByText('Прикладываю фото')).toBeInTheDocument();

    const created = db.comments.filter((comment) => comment.ticket_id === 6).at(-1)!;
    expect(created).toBeDefined();
    expect(created.author_name).toBe('Посетитель');
    expect(created.author_type).toBe('anonymous');
    expect(created.file_ids).toEqual([3]);
    // Файл вернулся с публичным URL и превью (antd Image даёт два узла с alt).
    expect((await screen.findAllByAltText('protechka-foto.svg')).length).toBeGreaterThan(0);
  });

  it('скачивает Word-версию заявки кнопкой', async () => {
    const toast = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() };
    registerToastApi(toast);
    Object.defineProperty(URL, 'createObjectURL', {
      writable: true,
      value: vi.fn(() => 'blob:mock-docx'),
    });
    Object.defineProperty(URL, 'revokeObjectURL', { writable: true, value: vi.fn() });
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

    renderToken();

    const button = await screen.findByTestId('export-button');
    await userEvent.click(button);

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Файл готов', 'ticket_4.docx'));
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1);
  });
});
