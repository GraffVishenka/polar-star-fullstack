/**
 * Интеграционные тесты `/kb` (Ф3): дерево + статья, права, создание
 * подраздела, понятные ошибки 400 и защита несохранённых изменений.
 *
 * Страница использует `useBlocker`, поэтому рендерим её в data-роутере
 * (`createMemoryRouter`) — как в приложении (`createBrowserRouter`).
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App as AntApp, ConfigProvider } from 'antd';
import ruRU from 'antd/locale/ru_RU';
import { QueryClientProvider } from '@tanstack/react-query';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { render } from '@testing-library/react';
import { AuthContext } from '@/auth/AuthContext';
import { server } from '@/mocks/server';
import { authenticateAs, makeAuthUser, makeAuthValue, createTestQueryClient } from '@/test/utils';
import { HttpResponse, http } from 'msw';
import { KbPage } from './KbPage';

/**
 * В jsdom `AbortSignal` создаётся jsdom, а `Request` — нативный (undici),
 * поэтому `new Request(url, { signal })` из react-router падает с
 * «Expected signal to be an instance of AbortSignal». Для тестов убираем
 * signal при создании Request: навигация проверяется без реальных запросов.
 */
const NativeRequest = globalThis.Request;
class TestRequest extends NativeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    if (init && typeof init === 'object' && 'signal' in init) {
      const rest = { ...init };
      delete (rest as { signal?: AbortSignal }).signal;
      super(input, rest);
      return;
    }
    super(input, init);
  }
}
globalThis.Request = TestRequest as unknown as typeof Request;

function renderKbPage(route = '/kb/3', admin = true) {
  const client = createTestQueryClient();
  const auth = admin
    ? makeAuthValue({ user: makeAuthUser({ role: 'ADMIN', building_id: null }) })
    : makeAuthValue();
  const router = createMemoryRouter(
    [
      { path: '/kb', element: <KbPage /> },
      { path: '/kb/:nodeId', element: <KbPage /> },
      { path: '/', element: <div data-testid="app-root">главная</div> },
    ],
    { initialEntries: [route] },
  );

  const utils = render(
    <ConfigProvider locale={ruRU}>
      <AntApp>
        <QueryClientProvider client={client}>
          <AuthContext.Provider value={auth}>
            <RouterProvider router={router} />
          </AuthContext.Provider>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>,
  );

  return { ...utils, client, router };
}

beforeEach(() => {
  // Авторизованный ADMIN (id 1) по умолчанию; USER-тесты ставят токен сами.
  authenticateAs(1);
});

describe('KbPage', () => {
  it('открывает выбранный раздел: дерево слева, статья справа, действия ADMIN', async () => {
    renderKbPage('/kb/3');

    await waitFor(() =>
      expect(screen.getByTestId('kb-article')).toHaveTextContent('Как создать заявку'),
    );
    expect(screen.getByTestId('kb-tree')).toBeInTheDocument();
    // Дерево раскрыто до выбранного узла (он на 3-м уровне в сиде).
    expect(screen.getByTestId('kb-node-3')).toHaveAttribute('data-depth', '3');
    // Замок у скрытого корневого раздела 10 (KB-7).
    expect(screen.getByTestId('kb-lock-10')).toBeInTheDocument();
    // ADMIN видит удаление, USER — нет (§3.1).
    expect(screen.getByTestId('kb-delete')).toBeInTheDocument();
    expect(screen.getByTestId('kb-edit')).toBeInTheDocument();
  });

  it('не показывает USER удаление и тумблер видимости (KB-3, KB-7)', async () => {
    authenticateAs(2);
    renderKbPage('/kb/11', false);

    await waitFor(() =>
      expect(screen.getByTestId('kb-article')).toHaveTextContent('Пользователи и роли'),
    );
    expect(screen.queryByTestId('kb-delete')).toBeNull();
    expect(screen.queryByTestId('kb-visibility-switch')).toBeNull();
    expect(screen.getByTestId('kb-edit')).toBeInTheDocument();
  });

  it('создаёт подраздел через модалку и открывает его (KB-1, KB-2)', async () => {
    authenticateAs(2);
    const user = userEvent.setup();
    const { router } = renderKbPage('/kb/2', false);

    await waitFor(() => expect(screen.getByTestId('kb-article')).toHaveTextContent('Заявки'));

    await user.click(screen.getByTestId('kb-add-subsection'));
    const titleInput = await screen.findByTestId('kb-node-title-input');
    await user.type(titleInput, 'Подраздел с проверкой прав');

    await user.click(screen.getByRole('button', { name: 'Создать' }));

    await waitFor(() =>
      expect(screen.getByTestId('kb-article')).toHaveTextContent('Подраздел с проверкой прав'),
    );
    expect(router.state.location.pathname).toMatch(/^\/kb\/\d+$/);
    expect(router.state.location.pathname).not.toBe('/kb/2');

    // Новый узел появился в дереве как потомок выбранного раздела.
    await waitFor(() => {
      const sidebar = within(screen.getByTestId('kb-sidebar'));
      const created = sidebar.getByText('Подраздел с проверкой прав');
      expect(created.closest('li')).toHaveAttribute('data-depth', '3');
    });
  });

  it('показывает понятную ошибку 400 при создании раздела', async () => {
    authenticateAs(2);
    const user = userEvent.setup();
    server.use(
      http.post('*/api/kb/nodes', () =>
        HttpResponse.json(
          {
            code: 'VALIDATION_ERROR',
            message: 'Родительский раздел не найден',
            details: { parent_id: 'not found' },
            request_id: 'mock-400',
          },
          { status: 400 },
        ),
      ),
    );

    renderKbPage('/kb/2', false);
    await waitFor(() => expect(screen.getByTestId('kb-article')).toHaveTextContent('Заявки'));

    await user.click(screen.getByTestId('kb-add-root'));
    await user.type(await screen.findByTestId('kb-node-title-input'), 'Новый раздел');
    await user.click(screen.getByRole('button', { name: 'Создать' }));

    await waitFor(() =>
      expect(screen.getByTestId('kb-node-form-error')).toHaveTextContent(
        'Проверьте заполнение полей',
      ),
    );
    // Модалка остаётся открытой — данные пользователя не потеряны.
    expect(screen.getByTestId('kb-node-title-input')).toHaveValue('Новый раздел');
  });

  it('защищает от потери несохранённых изменений: beforeunload и подтверждение перехода', async () => {
    const user = userEvent.setup();
    const { router } = renderKbPage('/kb/3');

    await waitFor(() => expect(screen.getByTestId('kb-edit')).toBeInTheDocument());
    await user.click(screen.getByTestId('kb-edit'));

    const titleInput = await screen.findByTestId('kb-editor-title');
    await user.clear(titleInput);
    await user.type(titleInput, 'Черновик без сохранения');
    await waitFor(() => expect(screen.getByTestId('kb-editor-dirty')).toBeInTheDocument());

    // beforeunload: браузер спросит подтверждение при закрытии вкладки.
    const unload = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(unload);
    expect(unload.defaultPrevented).toBe(true);

    // Переход по дереву блокируется, пока правки не сохранены.
    await user.click(screen.getByTestId('kb-node-row-7'));
    await waitFor(() =>
      expect(screen.getByTestId('confirm-dialog-body')).toHaveTextContent(
        'Если уйти со страницы, правки статьи будут потеряны.',
      ),
    );
    await user.click(screen.getByRole('button', { name: 'Остаться' }));

    // Навигация отменена: остались в редакторе той же статьи.
    await waitFor(() => expect(router.state.location.pathname).toBe('/kb/3'));
    expect(screen.getByTestId('kb-editor')).toBeInTheDocument();
    expect(screen.queryByTestId('kb-article')).toBeNull();
  });

  it('удаляет раздел только после подтверждения и возвращает к родителю (KB-3)', async () => {
    const user = userEvent.setup();
    renderKbPage('/kb/12');

    await waitFor(() =>
      expect(screen.getByTestId('kb-article')).toHaveTextContent('Заявки на проверку'),
    );

    await user.click(screen.getByTestId('kb-delete'));
    await waitFor(() => expect(screen.getByText('Удалить раздел?')).toBeInTheDocument());
    await user.click(screen.getByTestId('confirm-dialog-ok'));

    await waitFor(() =>
      expect(screen.getByTestId('kb-article')).toHaveTextContent('Администрирование'),
    );
    await waitFor(() => expect(screen.queryByTestId('kb-node-12')).toBeNull());
  });
});
