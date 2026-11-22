/** Тестовые утилиты: провайдеры (antd + Query + Auth + Router) и фабрики данных. */
import type { ReactElement, ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App as AntApp, ConfigProvider } from 'antd';
import ruRU from 'antd/locale/ru_RU';
import { render, type RenderOptions } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import type { AuthUser } from '@/api/types';
import { AuthContext, type AuthContextValue } from '@/auth/AuthContext';

export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
      mutations: { retry: false },
    },
  });
}

export function makeAuthUser(overrides: Partial<AuthUser> = {}): AuthUser {
  return {
    id: 2,
    login: 'user',
    full_name: 'Иванов Иван Иванович',
    role: 'USER',
    building_id: 1,
    building_name: 'Центральный офис',
    must_change_password: false,
    is_active: true,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

export function makeAuthValue(overrides: Partial<AuthContextValue> = {}): AuthContextValue {
  const user = overrides.user ?? makeAuthUser();
  const status = overrides.status ?? (user ? 'authenticated' : 'anonymous');
  return {
    status,
    user,
    isAuthenticated: status === 'authenticated' && user !== null,
    isAdmin: user?.role === 'ADMIN',
    mustChangePassword: user?.must_change_password ?? false,
    login: async () => user ?? makeAuthUser(),
    logout: async () => {},
    refreshUser: async () => user,
    ...overrides,
  };
}

export function makeAnonymousAuthValue(): AuthContextValue {
  return makeAuthValue({ status: 'anonymous', user: null });
}

export interface ProviderOptions {
  route?: string;
  auth?: AuthContextValue;
  client?: QueryClient;
}

export function createWrapper(options: ProviderOptions = {}) {
  const client = options.client ?? createTestQueryClient();
  const auth = options.auth ?? makeAuthValue();
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ConfigProvider locale={ruRU}>
        <AntApp>
          <QueryClientProvider client={client}>
            <AuthContext.Provider value={auth}>
              <MemoryRouter initialEntries={[options.route ?? '/']}>{children}</MemoryRouter>
            </AuthContext.Provider>
          </QueryClientProvider>
        </AntApp>
      </ConfigProvider>
    );
  };
}

export function renderWithProviders(
  ui: ReactElement,
  options: ProviderOptions & Omit<RenderOptions, 'wrapper'> = {},
) {
  const { route, auth, client, ...renderOptions } = options;
  return render(ui, {
    wrapper: createWrapper({ route, auth, client }),
    ...renderOptions,
  });
}
