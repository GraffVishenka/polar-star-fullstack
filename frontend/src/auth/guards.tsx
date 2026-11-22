/**
 * Guards маршрутов (§3.1, §3.2 AUTH-4).
 *
 * - `RequireAuth` — только для авторизованных; анонима отправляет на `/login`
 *   с запоминанием адреса возврата; при `must_change_password` — на смену пароля.
 * - `RequireRole('ADMIN')` — USER получает редирект на `/tickets` (без утечки
 *   существования админских данных).
 * - `RequireAnonOnly` — публичный `/` для анонима; авторизованного ведёт на `/tickets`.
 */
import type { ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from './AuthContext';
import { FullPageLoader } from '@/components/FullPageLoader';
import type { Role } from '@/api/types';

export const PASSWORD_CHANGE_PATH = '/change-password';

export interface RequireAuthProps {
  children: ReactNode;
  /** Куда вести анонима. */
  redirectTo?: string;
}

export function RequireAuth({ children, redirectTo = '/login' }: RequireAuthProps) {
  const { status, user } = useAuth();
  const location = useLocation();

  if (status === 'loading') return <FullPageLoader text="Проверяем сессию…" />;
  if (status === 'anonymous' || !user) {
    return (
      <Navigate
        to={redirectTo}
        replace
        state={{ from: `${location.pathname}${location.search}` }}
      />
    );
  }
  if (user.must_change_password && location.pathname !== PASSWORD_CHANGE_PATH) {
    return <Navigate to={PASSWORD_CHANGE_PATH} replace />;
  }
  return <>{children}</>;
}

export interface RequireRoleProps {
  role: Role | Role[];
  children: ReactNode;
  redirectTo?: string;
}

export function RequireRole({ role, children, redirectTo = '/tickets' }: RequireRoleProps) {
  const { status, user } = useAuth();
  const location = useLocation();
  const roles = Array.isArray(role) ? role : [role];

  if (status === 'loading') return <FullPageLoader text="Проверяем сессию…" />;
  if (status === 'anonymous' || !user) {
    return (
      <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}` }} />
    );
  }
  if (!roles.includes(user.role)) return <Navigate to={redirectTo} replace />;
  return <>{children}</>;
}

export interface RequireAnonOnlyProps {
  children: ReactNode;
  redirectTo?: string;
}

export function RequireAnonOnly({ children, redirectTo = '/tickets' }: RequireAnonOnlyProps) {
  const { status } = useAuth();
  if (status === 'loading') return <FullPageLoader text="Загружаем…" />;
  if (status === 'authenticated') return <Navigate to={redirectTo} replace />;
  return <>{children}</>;
}
