/** Сборка дерева маршрутов приложения. */
import type { RouteObject } from 'react-router-dom';
import { ForbiddenPage } from '@/pages/ForbiddenPage';
import { LoginPage } from '@/pages/auth/LoginPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { appRoutes } from './appRoutes';
import { publicRoutes } from './publicRoutes';

export const routes: RouteObject[] = [
  appRoutes,
  ...publicRoutes,
  { path: '/login', element: <LoginPage /> },
  { path: '/403', element: <ForbiddenPage /> },
  { path: '*', element: <NotFoundPage /> },
];

export { publicRoutes, appRoutes };
