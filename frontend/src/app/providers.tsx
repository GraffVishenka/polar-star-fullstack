/**
 * Провайдеры приложения: тема AntD + русская локаль, antd App (message/notification),
 * ErrorBoundary, TanStack Query и auth-контекст.
 *
 * AuthProvider находится вне RouterProvider: guards читают контекст и работают
 * с `useLocation`, поэтому контекст должен быть доступен всем маршрутам.
 */
import type { ReactNode } from 'react';
import { App as AntApp, ConfigProvider } from 'antd';
import ruRU from 'antd/locale/ru_RU';
import { QueryClientProvider } from '@tanstack/react-query';
import dayjs from 'dayjs';
import 'dayjs/locale/ru';
import { AuthProvider } from '@/auth/AuthContext';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { queryClient } from './queryClient';
import { themeConfig } from './theme';
import { ToastBridge } from './ToastBridge';

dayjs.locale('ru');

export interface ProvidersProps {
  children: ReactNode;
  /** Свой QueryClient (тесты). */
  client?: typeof queryClient;
}

export function Providers({ children, client = queryClient }: ProvidersProps) {
  return (
    <ConfigProvider locale={ruRU} theme={themeConfig}>
      <AntApp>
        <ErrorBoundary>
          <QueryClientProvider client={client}>
            <AuthProvider>{children}</AuthProvider>
          </QueryClientProvider>
        </ErrorBoundary>
        <ToastBridge />
      </AntApp>
    </ConfigProvider>
  );
}
