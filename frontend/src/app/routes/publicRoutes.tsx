/**
 * Публичные маршруты (§3.11): `/`, `/b/:buildingId`, `/t/:token`, публичная БЗ.
 * Только здесь разрешён анонимный доступ; данные приходят из `/api/public/*`.
 *
 * Страницы грузятся лениво: анонимный посетитель не скачивает код кабинета
 * (TipTap, dnd-kit, тяжёлые таблицы), пока не откроет нужный раздел.
 */
import { lazy } from 'react';
import type { RouteObject } from 'react-router-dom';
import { RequireAnonOnly } from '@/auth/guards';
import { PublicLayout } from '@/layouts/PublicLayout';

const BuildingSelectPage = lazy(() =>
  import('@/pages/public/BuildingSelectPage').then((module) => ({
    default: module.BuildingSelectPage,
  })),
);
const BuildingDashboardPage = lazy(() =>
  import('@/pages/public/BuildingDashboardPage').then((module) => ({
    default: module.BuildingDashboardPage,
  })),
);
const PublicTicketPage = lazy(() =>
  import('@/pages/public/PublicTicketPage').then((module) => ({
    default: module.PublicTicketPage,
  })),
);
const PublicKbPage = lazy(() =>
  import('@/pages/public/PublicKbPage').then((module) => ({ default: module.PublicKbPage })),
);

export const publicRoutes: RouteObject[] = [
  {
    path: '/',
    element: <PublicLayout />,
    children: [
      {
        index: true,
        element: (
          <RequireAnonOnly>
            <BuildingSelectPage />
          </RequireAnonOnly>
        ),
      },
      { path: 'b/:buildingId', element: <BuildingDashboardPage /> },
      { path: 't/:token', element: <PublicTicketPage /> },
      { path: 'public/kb', element: <PublicKbPage /> },
    ],
  },
];
