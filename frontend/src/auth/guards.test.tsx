/** Тесты guards: редиректы по роли, анонимности и must_change_password. */
import { screen } from '@testing-library/react';
import { Route, Routes } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { RequireAnonOnly, RequireAuth, RequireRole, PASSWORD_CHANGE_PATH } from './guards';
import {
  makeAnonymousAuthValue,
  makeAuthUser,
  makeAuthValue,
  renderWithProviders,
} from '@/test/utils';
import type { AuthContextValue } from './AuthContext';

function renderGuarded(auth: AuthContextValue, route = '/protected') {
  return renderWithProviders(
    <Routes>
      <Route path="/login" element={<div>login page</div>} />
      <Route path="/tickets" element={<div>tickets page</div>} />
      <Route path={PASSWORD_CHANGE_PATH} element={<div>change password page</div>} />
      <Route
        path="/protected"
        element={
          <RequireAuth>
            <div>protected content</div>
          </RequireAuth>
        }
      />
      <Route
        path="/admin"
        element={
          <RequireRole role="ADMIN">
            <div>admin page</div>
          </RequireRole>
        }
      />
      <Route
        path="/"
        element={
          <RequireAnonOnly>
            <div>public root</div>
          </RequireAnonOnly>
        }
      />
    </Routes>,
    { route, auth },
  );
}

describe('RequireAuth', () => {
  it('анонима отправляет на /login', () => {
    renderGuarded(makeAnonymousAuthValue());
    expect(screen.getByText('login page')).toBeInTheDocument();
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  it('показывает загрузку, пока идёт bootstrap сессии', () => {
    renderGuarded(makeAuthValue({ status: 'loading', user: null }));
    expect(screen.getByTestId('full-page-loader')).toBeInTheDocument();
    expect(screen.queryByText('login page')).not.toBeInTheDocument();
  });

  it('пускает авторизованного сотрудника', () => {
    renderGuarded(makeAuthValue());
    expect(screen.getByText('protected content')).toBeInTheDocument();
  });

  it('при must_change_password ведёт на смену пароля', () => {
    renderGuarded(
      makeAuthValue({ user: makeAuthUser({ must_change_password: true }) }),
      '/protected',
    );
    expect(screen.getByText('change password page')).toBeInTheDocument();
    expect(screen.queryByText('protected content')).not.toBeInTheDocument();
  });

  it('не зацикливает редирект на самой странице смены пароля', () => {
    renderGuarded(
      makeAuthValue({ user: makeAuthUser({ must_change_password: true }) }),
      PASSWORD_CHANGE_PATH,
    );
    expect(screen.getByText('change password page')).toBeInTheDocument();
  });
});

describe('RequireRole', () => {
  it('USER на админском маршруте получает редирект на /tickets', () => {
    renderGuarded(makeAuthValue(), '/admin');
    expect(screen.getByText('tickets page')).toBeInTheDocument();
    expect(screen.queryByText('admin page')).not.toBeInTheDocument();
  });

  it('ADMIN видит админский маршрут', () => {
    renderGuarded(
      makeAuthValue({
        user: makeAuthUser({ role: 'ADMIN', building_id: null, building_name: null }),
      }),
      '/admin',
    );
    expect(screen.getByText('admin page')).toBeInTheDocument();
  });

  it('анонима на админском маршруте ведёт на /login', () => {
    renderGuarded(makeAnonymousAuthValue(), '/admin');
    expect(screen.getByText('login page')).toBeInTheDocument();
  });
});

describe('RequireAnonOnly', () => {
  it('аноним видит публичный корень', () => {
    renderGuarded(makeAnonymousAuthValue(), '/');
    expect(screen.getByText('public root')).toBeInTheDocument();
  });

  it('авторизованного уводит с публичного корня на /tickets', () => {
    renderGuarded(makeAuthValue(), '/');
    expect(screen.getByText('tickets page')).toBeInTheDocument();
    expect(screen.queryByText('public root')).not.toBeInTheDocument();
  });
});
