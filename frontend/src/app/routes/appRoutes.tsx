/**
 * Маршруты авторизованных сотрудников (AppLayout) и админ-панели.
 *
 * Регистрация по доменам: сейчас заглушки ведут на `PlaceholderPage`, чтобы
 * владельцы разделов (fe-2, fe-3) заменили только свою строку маршрута и не
 * конфликтовали с общими файлами.
 */
import type { RouteObject } from 'react-router-dom';
import { Navigate } from 'react-router-dom';
import { RequireAuth, RequireRole } from '@/auth/guards';
import { AdminLayout } from '@/layouts/AdminLayout';
import { AppLayout } from '@/layouts/AppLayout';
import { ChangePasswordPage } from '@/pages/auth/ChangePasswordPage';
import { PlaceholderPage } from '@/pages/placeholders/PlaceholderPage';
import { TicketsPage } from '@/pages/tickets/TicketsPage';

export const adminRoutes: RouteObject = {
  path: 'admin',
  element: (
    <RequireRole role="ADMIN">
      <AdminLayout />
    </RequireRole>
  ),
  children: [
    { index: true, element: <Navigate to="users" replace /> },
    {
      path: 'users',
      element: <PlaceholderPage title="Пользователи" block="Ф6" owner="fe-2" />,
    },
    {
      path: 'dictionaries',
      element: <PlaceholderPage title="Справочники" block="Ф6" owner="fe-2" />,
    },
    {
      path: 'settings',
      element: <PlaceholderPage title="Настройки" block="Ф6" owner="fe-2" />,
    },
    {
      path: 'buildings',
      element: <PlaceholderPage title="Здания и склады" block="Ф7" owner="fe-3" />,
    },
    {
      path: 'stats',
      element: <PlaceholderPage title="Статистика сотрудников" block="Ф10" owner="fe-3" />,
    },
    {
      path: 'trash',
      element: <PlaceholderPage title="Корзина" block="Ф7" owner="fe-3" />,
    },
  ],
};

export const appRoutes: RouteObject = {
  element: (
    <RequireAuth>
      <AppLayout />
    </RequireAuth>
  ),
  children: [
    { path: 'tickets', element: <TicketsPage /> },
    { path: 'change-password', element: <ChangePasswordPage /> },
    {
      path: 'kb',
      element: <PlaceholderPage title="База знаний" block="Ф3" owner="fe-2" />,
    },
    {
      path: 'equipment',
      element: <PlaceholderPage title="Оборудование" block="Ф4" owner="fe-3" />,
    },
    {
      path: 'stats',
      element: <PlaceholderPage title="Моя статистика" block="Ф10" owner="fe-3" />,
    },
    {
      path: 'profile',
      element: <PlaceholderPage title="Профиль" block="Ф10" owner="fe-3" />,
    },
    adminRoutes,
  ],
};
