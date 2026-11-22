/**
 * `/change-password` (AUTH-4/AUTH-5): валидация и успешная смена пароля.
 *
 * Форма на antd Form с правилами — регрессия на баг «RHF `register` +
 * antd `Input.Password` не передаёт значения»: тест вводит пароли и проверяет,
 * что запрос ушёл с введёнными значениями.
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it } from 'vitest';
import { db } from '@/mocks/db';
import { USER_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { ChangePasswordPage } from './ChangePasswordPage';

function renderPage(mustChangePassword = false) {
  authenticateAs(USER_AUTH.id);
  return renderWithProviders(
    <Routes>
      <Route path="/change-password" element={<ChangePasswordPage />} />
      <Route path="/tickets" element={<div>tickets page</div>} />
    </Routes>,
    {
      route: '/change-password',
      auth: makeAuthValue({ user: { ...USER_AUTH, must_change_password: mustChangePassword } }),
    },
  );
}

describe('ChangePasswordPage', () => {
  beforeEach(() => authenticateAs(USER_AUTH.id));

  it('показывает ошибки обязательных полей', async () => {
    renderPage();

    await userEvent.click(await screen.findByTestId('change-password-submit'));

    // antd Form валидирует поля асинхронно — даём запас по времени под нагрузкой CI.
    await waitFor(
      () => {
        expect(screen.getByText('Введите текущий пароль')).toBeInTheDocument();
        expect(screen.getByText('Введите новый пароль')).toBeInTheDocument();
        expect(screen.getByText('Подтвердите новый пароль')).toBeInTheDocument();
      },
      { timeout: 3000 },
    );
    // До сервера запрос не дошёл.
    expect(db.users.find((user) => user.id === USER_AUTH.id)?.password).toBe('user12345');
  });

  it('валидирует длину, совпадение и отличие от текущего пароля', async () => {
    renderPage();

    await userEvent.type(await screen.findByTestId('change-password-current'), 'user12345');
    await userEvent.type(screen.getByTestId('change-password-new'), 'short');
    await userEvent.type(screen.getByTestId('change-password-confirm'), 'other');
    await userEvent.click(screen.getByTestId('change-password-submit'));

    expect(await screen.findByText('Новый пароль — не короче 8 символов')).toBeInTheDocument();
    expect(screen.getByText('Пароли не совпадают')).toBeInTheDocument();

    // Тот же пароль, что и текущий, тоже отклоняется.
    await userEvent.clear(screen.getByTestId('change-password-new'));
    await userEvent.type(screen.getByTestId('change-password-new'), 'user12345');
    await userEvent.clear(screen.getByTestId('change-password-confirm'));
    await userEvent.type(screen.getByTestId('change-password-confirm'), 'user12345');
    await userEvent.click(screen.getByTestId('change-password-submit'));

    await waitFor(
      () =>
        expect(screen.getByText('Новый пароль должен отличаться от текущего')).toBeInTheDocument(),
      { timeout: 3000 },
    );
  });

  it('меняет пароль и ведёт в заявки после успеха', async () => {
    renderPage();

    await userEvent.type(await screen.findByTestId('change-password-current'), 'user12345');
    await userEvent.type(screen.getByTestId('change-password-new'), 'newpass123');
    await userEvent.type(screen.getByTestId('change-password-confirm'), 'newpass123');
    await userEvent.click(screen.getByTestId('change-password-submit'));

    expect(await screen.findByText('tickets page')).toBeInTheDocument();
    expect(db.users.find((user) => user.id === USER_AUTH.id)?.password).toBe('newpass123');
  });

  it('показывает серверную ошибку при неверном текущем пароле', async () => {
    renderPage();

    await userEvent.type(await screen.findByTestId('change-password-current'), 'wrong-password');
    await userEvent.type(screen.getByTestId('change-password-new'), 'newpass123');
    await userEvent.type(screen.getByTestId('change-password-confirm'), 'newpass123');
    await userEvent.click(screen.getByTestId('change-password-submit'));

    expect(await screen.findByTestId('change-password-error')).toBeInTheDocument();
    // Пароль не изменился, экран остался на месте.
    expect(db.users.find((user) => user.id === USER_AUTH.id)?.password).toBe('user12345');
    await waitFor(() => expect(screen.queryByText('tickets page')).not.toBeInTheDocument());
  });

  it('при обязательной смене пароля не показывает «Отмену»', async () => {
    renderPage(true);

    expect(await screen.findByTestId('change-password-page')).toBeInTheDocument();
    expect(screen.queryByTestId('change-password-cancel')).not.toBeInTheDocument();
    expect(screen.getByText(/временным паролем/)).toBeInTheDocument();
  });
});
