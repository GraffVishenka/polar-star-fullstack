/**
 * `/login` — вход сотрудника (AUTH-1).
 *
 * Регрессия QA: форма на RHF `register` + antd `Input` отправлялась с пустыми
 * значениями, поэтому тест действительно печатает логин/пароль через
 * `userEvent.type` и проверяет, что `AuthContext.login` получил их.
 */
import { describe, expect, it, vi, type Mock } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { App as AntApp, ConfigProvider } from 'antd';
import ruRU from 'antd/locale/ru_RU';
import { QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { ApiError } from '@/api/errors';
import type { AuthUser } from '@/api/types';
import { AuthContext, type AuthContextValue } from '@/auth/AuthContext';
import { createTestQueryClient, makeAuthUser, makeAuthValue } from '@/test/utils';
import { LoginPage } from './LoginPage';

interface RenderOptions {
  login?: Mock;
  initialEntries?: Array<string | { pathname: string; state?: unknown }>;
}

function renderLoginPage(options: RenderOptions = {}) {
  const login = options.login ?? vi.fn(async () => makeAuthUser());
  const auth = makeAuthValue({
    status: 'anonymous',
    user: null,
    login: login as unknown as AuthContextValue['login'],
  });

  render(
    <ConfigProvider locale={ruRU}>
      <AntApp>
        <QueryClientProvider client={createTestQueryClient()}>
          <AuthContext.Provider value={auth}>
            <MemoryRouter initialEntries={options.initialEntries ?? ['/login']}>
              <Routes>
                <Route path="/login" element={<LoginPage />} />
                <Route path="/tickets" element={<div data-testid="tickets-page">Заявки</div>} />
                <Route
                  path="/change-password"
                  element={<div data-testid="change-password-page">Смена пароля</div>}
                />
                <Route path="/kb/7" element={<div data-testid="kb-page">Статья БЗ</div>} />
              </Routes>
            </MemoryRouter>
          </AuthContext.Provider>
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>,
  );

  return { login };
}

async function fillCredentials(login = 'admin', password = 'admin12345') {
  const user = userEvent.setup();
  await user.type(screen.getByTestId('login-input'), login);
  await user.type(screen.getByTestId('password-input'), password);
  return user;
}

describe('LoginPage', () => {
  it('отправляет введённые логин и пароль в AuthContext.login и ведёт на /tickets', async () => {
    const login = vi.fn(async () => makeAuthUser({ full_name: 'Администратор Системы' }));
    renderLoginPage({ login });

    const user = await fillCredentials('admin', 'admin12345');
    await user.click(screen.getByTestId('login-submit'));

    await waitFor(() =>
      expect(login).toHaveBeenCalledWith({ login: 'admin', password: 'admin12345' }),
    );
    expect(await screen.findByTestId('tickets-page')).toBeInTheDocument();
  });

  it('не отправляет пустую форму и показывает ошибки обязательных полей', async () => {
    const { login } = renderLoginPage();
    const user = userEvent.setup();

    await user.click(screen.getByTestId('login-submit'));

    await waitFor(() => {
      const messages = Array.from(document.querySelectorAll('.ant-form-item-explain-error')).map(
        (node) => node.textContent,
      );
      expect(messages).toContain('Введите логин');
      expect(messages).toContain('Введите пароль');
    });
    expect(login).not.toHaveBeenCalled();
  });

  it('показывает INVALID_CREDENTIALS без раскрытия существования логина', async () => {
    const login = vi.fn(async (): Promise<AuthUser> => {
      throw new ApiError({ code: 'INVALID_CREDENTIALS', status: 401 });
    });
    renderLoginPage({ login });

    const user = await fillCredentials('admin', 'wrong-pass');
    await user.click(screen.getByTestId('login-submit'));

    await waitFor(() =>
      expect(screen.getByTestId('login-error')).toHaveTextContent('Неверный логин или пароль'),
    );
    expect(screen.queryByTestId('tickets-page')).not.toBeInTheDocument();
  });

  it('показывает сообщение про лимит попыток при 429', async () => {
    const login = vi.fn(async (): Promise<AuthUser> => {
      throw new ApiError({ code: 'TOO_MANY_REQUESTS', status: 429 });
    });
    renderLoginPage({ login });

    const user = await fillCredentials('admin', 'admin12345');
    await user.click(screen.getByTestId('login-submit'));

    await waitFor(() =>
      expect(screen.getByTestId('login-error')).toHaveTextContent(
        'Слишком много попыток входа. Подождите 10 минут.',
      ),
    );
  });

  it('при must_change_password ведёт на /change-password', async () => {
    const login = vi.fn(async () => makeAuthUser({ must_change_password: true }));
    renderLoginPage({ login });

    const user = await fillCredentials('novichok', 'temp12345');
    await user.click(screen.getByTestId('login-submit'));

    expect(await screen.findByTestId('change-password-page')).toBeInTheDocument();
  });

  it('возвращает на state.from после успешного входа', async () => {
    const login = vi.fn(async () => makeAuthUser());
    renderLoginPage({
      login,
      initialEntries: [{ pathname: '/login', state: { from: '/kb/7' } }],
    });

    const user = await fillCredentials('admin', 'admin12345');
    await user.click(screen.getByTestId('login-submit'));

    expect(await screen.findByTestId('kb-page')).toBeInTheDocument();
  });

  it('показывает демо-подсказку в DEV-режиме', () => {
    renderLoginPage();
    expect(screen.getByTestId('login-demo-hint')).toBeInTheDocument();
  });
});
