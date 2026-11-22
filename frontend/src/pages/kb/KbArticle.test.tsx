/** Тесты статьи БЗ: безопасный рендер (KB-4) и права USER/ADMIN (§3.1, KB-7). */
import { describe, expect, it, vi } from 'vitest';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { KbNode, KbTreeNode } from '@/api/types';
import { makeAuthUser, makeAuthValue, renderWithProviders } from '@/test/utils';
import { KbArticle } from './components/KbArticle';

const root: KbTreeNode = {
  id: 1,
  title: 'Инструкции',
  parent_id: null,
  sort_order: 10,
  is_hidden_for_anon: false,
  is_hidden_effective: false,
  has_children: true,
  children: [],
};

const child: KbTreeNode = {
  id: 3,
  title: 'Как создать заявку',
  parent_id: 1,
  sort_order: 10,
  is_hidden_for_anon: false,
  is_hidden_effective: true,
  has_children: false,
  children: [],
};

function makeNode(overrides: Partial<KbNode> = {}): KbNode {
  return {
    id: 3,
    title: 'Как создать заявку',
    content_html: '<h2>Создание заявки</h2><p>Выберите кабинет и вид проблемы.</p>',
    parent_id: 1,
    sort_order: 10,
    is_hidden_for_anon: false,
    is_hidden_effective: false,
    created_at: '2026-01-10T09:00:00Z',
    updated_at: '2026-02-11T12:30:00Z',
    updated_by_name: 'Администратор Системы',
    media: [],
    ...overrides,
  };
}

const path = [root, child];

describe('KbArticle', () => {
  it('рендерит статью и вырезает <script> из content_html (KB-4)', () => {
    const onEdit = vi.fn();
    const { container } = renderWithProviders(
      <KbArticle
        node={makeNode({
          content_html:
            '<h2>Поля формы</h2><p>Кабинет обязателен.</p><script>window.__kbXss = true;</script>' +
            '<img src="/api/files/1" onerror="alert(1)" alt="форма">',
        })}
        path={path}
        isAdmin={false}
        onEdit={onEdit}
        onDelete={vi.fn()}
        onVisibilityChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId('kb-article')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Поля формы' })).toBeInTheDocument();
    expect(screen.getByTestId('kb-article-content')).toHaveTextContent('Кабинет обязателен.');
    // Скрипт не попал в DOM и не выполнился.
    expect(container.querySelector('script')).toBeNull();
    expect((window as unknown as { __kbXss?: boolean }).__kbXss).toBeUndefined();
    expect(container.querySelector('img')?.getAttribute('onerror')).toBeNull();
    expect(
      screen.getByText(/Обновлено .* · Администратор Системы/),
    ).toBeInTheDocument();
  });

  it('показывает ADMIN удаление и тумблер видимости корневого раздела', async () => {
    const onVisibilityChange = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(
      <KbArticle
        node={makeNode({ id: 1, title: 'Инструкции', parent_id: null })}
        path={[root]}
        isAdmin
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onVisibilityChange={onVisibilityChange}
      />,
      { auth: makeAuthValue({ user: makeAuthUser({ role: 'ADMIN', building_id: null }) }) },
    );

    expect(screen.getByTestId('kb-delete')).toBeInTheDocument();
    const toggle = screen.getByTestId('kb-visibility-switch');
    expect(toggle).toBeInTheDocument();
    await user.click(toggle);
    expect(onVisibilityChange).toHaveBeenCalledWith(true);
  });

  it('не показывает USER удаление и тумблер видимости (KB-3, KB-7)', () => {
    renderWithProviders(
      <KbArticle
        node={makeNode()}
        path={path}
        isAdmin={false}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onVisibilityChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId('kb-edit')).toBeInTheDocument();
    expect(screen.queryByTestId('kb-delete')).toBeNull();
    expect(screen.queryByTestId('kb-visibility-control')).toBeNull();
    expect(screen.queryByTestId('kb-visibility-switch')).toBeNull();
  });

  it('у подраздела видимость наследуется от корня (KB-1)', () => {
    renderWithProviders(
      <KbArticle
        node={makeNode({ is_hidden_effective: true })}
        path={path}
        isAdmin
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onVisibilityChange={vi.fn()}
      />,
      { auth: makeAuthValue({ user: makeAuthUser({ role: 'ADMIN', building_id: null }) }) },
    );

    expect(screen.getByTestId('kb-visibility-inherited')).toHaveTextContent('Инструкции');
    expect(screen.queryByTestId('kb-visibility-switch')).toBeNull();
    expect(screen.getByTestId('kb-article-hidden-badge')).toBeInTheDocument();
  });
});
