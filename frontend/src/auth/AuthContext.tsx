/**
 * Auth-контекст (§3.2).
 *
 * Access-токен хранится только в памяти API-клиента; в React-состоянии — `me`.
 * При старте приложение восстанавливает сессию по refresh-cookie (AUTH-2),
 * поэтому перезагрузка страницы не разлогинивает.
 */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { fetchMe, login as apiLogin, logout as apiLogout, restoreSession } from '@/api/auth';
import { clearSession, onAuthLost } from '@/api/client';
import type { AuthUser, LoginRequest } from '@/api/types';

export type AuthStatus = 'loading' | 'authenticated' | 'anonymous';

export interface AuthContextValue {
  status: AuthStatus;
  user: AuthUser | null;
  isAuthenticated: boolean;
  isAdmin: boolean;
  /** AUTH-4: пока true, разрешены только смена пароля и выход. */
  mustChangePassword: boolean;
  login: (input: LoginRequest) => Promise<AuthUser>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<AuthUser | null>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

export interface AuthProviderProps {
  children: ReactNode;
  /**
   * Готовый пользователь (тесты/SSR): пропускает bootstrap по refresh-cookie.
   * `null` — сразу аноним.
   */
  initialUser?: AuthUser | null;
  /** Отключить bootstrap (используется в unit-тестах провайдера). */
  disableBootstrap?: boolean;
}

export function AuthProvider({
  children,
  initialUser,
  disableBootstrap = initialUser !== undefined,
}: AuthProviderProps) {
  const [user, setUser] = useState<AuthUser | null>(initialUser ?? null);
  const [status, setStatus] = useState<AuthStatus>(
    disableBootstrap ? (initialUser ? 'authenticated' : 'anonymous') : 'loading',
  );

  useEffect(() => {
    if (disableBootstrap) return;
    let cancelled = false;

    void (async () => {
      try {
        const restored = await restoreSession();
        if (cancelled) return;
        setUser(restored);
        setStatus(restored ? 'authenticated' : 'anonymous');
      } catch {
        if (cancelled) return;
        setUser(null);
        setStatus('anonymous');
      }
    })();

    // Refresh-cookie отозвана/истекла — сессия закончилась (AUTH-2, AUTH-6).
    const unsubscribe = onAuthLost(() => {
      if (cancelled) return;
      setUser(null);
      setStatus('anonymous');
    });

    return () => {
      cancelled = true;
      unsubscribe();
    };
  }, [disableBootstrap]);

  const login = useCallback(async (input: LoginRequest) => {
    const response = await apiLogin(input);
    setUser(response.user);
    setStatus('authenticated');
    return response.user;
  }, []);

  const logout = useCallback(async () => {
    try {
      await apiLogout();
    } catch {
      // Даже если сервер недоступен, локальную сессию закрываем.
    } finally {
      clearSession();
      setUser(null);
      setStatus('anonymous');
    }
  }, []);

  const refreshUser = useCallback(async () => {
    try {
      const fresh = await fetchMe();
      setUser(fresh);
      setStatus('authenticated');
      return fresh;
    } catch {
      return null;
    }
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      isAuthenticated: status === 'authenticated' && user !== null,
      isAdmin: user?.role === 'ADMIN',
      mustChangePassword: user?.must_change_password ?? false,
      login,
      logout,
      refreshUser,
    }),
    [login, logout, refreshUser, status, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) throw new Error('useAuth должен использоваться внутри <AuthProvider>');
  return context;
}
