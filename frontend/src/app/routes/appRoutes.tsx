/**
 * Маршруты авторизованных сотрудников (AppLayout) и админ-панели.
 *
 * Страницы разделов загружаются лениво (`React.lazy`): начальный бандл не тянет
 * TipTap, dnd-kit и тяжёлые таблицы, пока пользователь не открыл раздел (NFR-8).
 * Suspense-фолбэк — в layout'ах (`AppLayout`/`AdminLayout`/`PublicLayout`).
 */
import { lazy } from 'react';
import type { RouteObject } from 'react-router-dom';
import { Navigate } from 'react-router-dom';
import { RequireAuth, RequireRole } from '@/auth/guards';
import { AdminLayout } from '@/layouts/AdminLayout';
import { AppLayout } from '@/layouts/AppLayout';
import { ChangePasswordPage } from '@/pages/auth/ChangePasswordPage';

const TicketsPage = lazy(() =>
  import('@/pages/tickets/TicketsPage').then((module) => ({ default: module.TicketsPage })),
);
const KbPage = lazy(() =>
  import('@/pages/kb/KbPage').then((module) => ({ default: module.KbPage })),
);
const EquipmentPage = lazy(() =>
  import('@/pages/equipment/EquipmentPage').then((module) => ({
    default: module.EquipmentPage,
  })),
);
const StatsPage = lazy(() =>
  import('@/pages/stats/StatsPage').then((module) => ({ default: module.StatsPage })),
);
const AdminStatsPage = lazy(() =>
  import('@/pages/stats/AdminStatsPage').then((module) => ({
    default: module.AdminStatsPage,
  })),
);
const ProfilePage = lazy(() =>
  import('@/pages/profile/ProfilePage').then((module) => ({ default: module.ProfilePage })),
);
const AdminBuildingsPage = lazy(() =>
  import('@/pages/admin/buildings/AdminBuildingsPage').then((module) => ({
    default: module.AdminBuildingsPage,
  })),
);
const AdminKbPage = lazy(() =>
  import('@/pages/admin/kb/AdminKbPage').then((module) => ({ default: module.AdminKbPage })),
);
const AdminTrashPage = lazy(() =>
  import('@/pages/admin/trash/AdminTrashPage').then((module) => ({
    default: module.AdminTrashPage,
  })),
);
const DictionariesPage = lazy(() =>
  import('@/pages/admin/dictionaries/DictionariesPage').then((module) => ({
    default: module.DictionariesPage,
  })),
);
const SettingsPage = lazy(() =>
  import('@/pages/admin/settings/SettingsPage').then((module) => ({
    default: module.SettingsPage,
  })),
);
const UsersPage = lazy(() =>
  import('@/pages/admin/users/UsersPage').then((module) => ({ default: module.UsersPage })),
);

export const adminRoutes: RouteObject = {
  path: 'admin',
  element: (
    <RequireRole role="ADMIN">
      <AdminLayout />
    </RequireRole>
  ),
  children: [
    { index: true, element: <Navigate to="users" replace /> },
    { path: 'users', element: <UsersPage /> },
    { path: 'dictionaries', element: <DictionariesPage /> },
    { path: 'settings', element: <SettingsPage /> },
    { path: 'kb', element: <AdminKbPage /> },
    { path: 'buildings', element: <AdminBuildingsPage /> },
    { path: 'stats', element: <AdminStatsPage /> },
    { path: 'trash', element: <AdminTrashPage /> },
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
    { path: 'kb', element: <KbPage /> },
    { path: 'kb/:nodeId', element: <KbPage /> },
    { path: 'equipment', element: <EquipmentPage /> },
    { path: 'stats', element: <StatsPage /> },
    { path: 'profile', element: <ProfilePage /> },
    adminRoutes,
  ],
};
