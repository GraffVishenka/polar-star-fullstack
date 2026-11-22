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
import { AdminBuildingsPage } from '@/pages/admin/buildings/AdminBuildingsPage';
import { AdminKbPage } from '@/pages/admin/kb/AdminKbPage';
import { AdminTrashPage } from '@/pages/admin/trash/AdminTrashPage';
import { DictionariesPage } from '@/pages/admin/dictionaries/DictionariesPage';
import { SettingsPage } from '@/pages/admin/settings/SettingsPage';
import { UsersPage } from '@/pages/admin/users/UsersPage';
import { EquipmentPage } from '@/pages/equipment/EquipmentPage';
import { KbPage } from '@/pages/kb/KbPage';
import { PlaceholderPage } from '@/pages/placeholders/PlaceholderPage';
import { ProfilePage } from '@/pages/profile/ProfilePage';
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
    // Ф6 (fe-2): пользователи, справочники и настройки/время.
    { path: 'users', element: <UsersPage /> },
    { path: 'dictionaries', element: <DictionariesPage /> },
    { path: 'settings', element: <SettingsPage /> },
    {
      path: 'kb',
      element: <AdminKbPage />,
    },
    {
      path: 'buildings',
      element: <AdminBuildingsPage />,
    },
    {
      path: 'stats',
      element: <PlaceholderPage title="Статистика сотрудников" block="Ф10" owner="fe-3" />,
    },
    {
      path: 'trash',
      element: <AdminTrashPage />,
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
    // Ф3 (fe-2): дерево + статья; выбранный раздел — отдельный адрес.
    { path: 'kb', element: <KbPage /> },
    { path: 'kb/:nodeId', element: <KbPage /> },
    { path: 'equipment', element: <EquipmentPage /> },
    {
      path: 'stats',
      element: <PlaceholderPage title="Моя статистика" block="Ф10" owner="fe-3" />,
    },
    { path: 'profile', element: <ProfilePage /> },
    adminRoutes,
  ],
};
