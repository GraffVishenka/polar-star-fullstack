/**
 * Публичная база знаний `/public/kb` (PUB-7, KB-4, KB-7):
 * дерево без скрытых разделов, чтение статьи, санитизация HTML, 404.
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { server } from '@/mocks/server';
import { makeAnonymousAuthValue, renderWithProviders } from '@/test/utils';
import { PublicKbPage, sanitizeKbHtml, toPublicMediaHtml } from '../PublicKbPage';

function renderKb(route = '/public/kb') {
  return renderWithProviders(
    <Routes>
      <Route path="/public/kb" element={<PublicKbPage />} />
    </Routes>,
    { route, auth: makeAnonymousAuthValue() },
  );
}

describe('sanitizeKbHtml (KB-4)', () => {
  it('вырезает script, обработчики и iframe', () => {
    const dirty =
      '<script>alert(1)</script><p onclick="alert(2)">Текст</p><iframe src="https://evil"></iframe>';
    const clean = sanitizeKbHtml(dirty);

    expect(clean).not.toContain('<script');
    expect(clean).not.toContain('onclick');
    expect(clean).not.toContain('<iframe');
    expect(clean).toContain('Текст');
  });

  it('переписывает медиа БЗ на публичный эндпоинт файлов', () => {
    expect(toPublicMediaHtml('<img src="/api/files/1" alt="схема">')).toContain(
      'src="/api/public/files/1"',
    );

    const clean = sanitizeKbHtml(
      '<img src="/api/files/1" alt="схема"><video src="/api/files/2"></video>',
    );
    expect(clean).toContain('/api/public/files/1');
    expect(clean).toContain('/api/public/files/2');
  });
});

describe('PublicKbPage', () => {
  it('показывает только публичные разделы (скрытые отсутствуют)', async () => {
    renderKb();

    expect(await screen.findByTestId('public-kb-page')).toBeInTheDocument();
    expect(screen.getByText('Инструкции')).toBeInTheDocument();
    expect(screen.getByText('Оборудование')).toBeInTheDocument();
    // KB-7: корневой раздел, скрытый для анонимов, не должен попадать в дерево.
    expect(screen.queryByText('Администрирование')).not.toBeInTheDocument();
  });

  it('открывает статью и показывает содержимое', async () => {
    renderKb('/public/kb?node=3');

    expect(await screen.findByTestId('public-kb-article')).toBeInTheDocument();
    expect(screen.getByText('Как создать заявку')).toBeInTheDocument();
    expect(screen.getByText('Создание заявки')).toBeInTheDocument();
    // Медиа внутри статьи отдаётся с публичного эндпоинта.
    const image = document.querySelector('.kb-article img');
    expect(image).not.toBeNull();
    expect(image?.getAttribute('src')).toContain('/api/public/files/');
  });

  it('скрытый раздел по прямой ссылке отдаёт 404 (KB-7)', async () => {
    renderKb('/public/kb?node=10');

    expect(await screen.findByTestId('public-not-found')).toBeInTheDocument();
    expect(screen.getByText('Раздел не найден')).toBeInTheDocument();
  });

  it('экранирует опасный HTML из статьи', async () => {
    server.use(
      http.get('*/api/public/kb/nodes/:id', () =>
        HttpResponse.json({
          id: 3,
          title: 'Опасная статья',
          content_html:
            '<p>Безопасный текст</p><script>window.__pwned = true</script><img src="/api/files/1" onerror="alert(1)">',
          parent_id: 2,
          sort_order: 10,
          is_hidden_for_anon: false,
          is_hidden_effective: false,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z',
          updated_by_name: null,
          media: [],
        }),
      ),
    );

    renderKb('/public/kb?node=3');

    expect(await screen.findByText('Безопасный текст')).toBeInTheDocument();
    await waitFor(() => expect(document.querySelector('.kb-article script')).toBeNull());
    expect(document.querySelector('.kb-article [onerror]')).toBeNull();
    expect(document.querySelector('.kb-article img')?.getAttribute('src')).toContain(
      '/api/public/files/1',
    );
  });

  it('позволяет выбрать раздел из дерева', async () => {
    renderKb();

    await screen.findByTestId('public-kb-page');
    await userEvent.click(screen.getByText('Развернуть всё'));
    await userEvent.click(screen.getByText('Как создать заявку'));

    expect(await screen.findByTestId('public-kb-article')).toBeInTheDocument();
    expect(screen.getByText('Создание заявки')).toBeInTheDocument();
  });
});
