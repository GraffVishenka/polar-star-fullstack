/** Сборка дерева маршрутов приложения. */
import type { RouteObject } from 'react-router-dom';
import { LoginPage } from '@/pages/auth/LoginPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { appRoutes } from './appRoutes';
import { publicRoutes } from './publicRoutes';

export const routes: RouteObject[] = [
  appRoutes,
  ...publicRoutes,
  { path: '/login', element: <LoginPage /> },
  { path: '*', element: <NotFoundPage /> },
];

export { publicRoutes, appRoutes };
