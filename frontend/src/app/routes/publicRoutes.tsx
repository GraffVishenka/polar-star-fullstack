/**
 * Публичные маршруты (§3.11): `/`, `/b/:buildingId`, `/t/:token`, публичная БЗ.
 * Только здесь разрешён анонимный доступ; данные приходят из `/api/public/*`.
 */
import type { RouteObject } from 'react-router-dom';
import { RequireAnonOnly } from '@/auth/guards';
import { PublicLayout } from '@/layouts/PublicLayout';
import { PlaceholderPage } from '@/pages/placeholders/PlaceholderPage';
import { BuildingDashboardPage } from '@/pages/public/BuildingDashboardPage';
import { BuildingSelectPage } from '@/pages/public/BuildingSelectPage';
import { PublicTicketPage } from '@/pages/public/PublicTicketPage';

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
      {
        path: 'public/kb',
        element: (
          <PlaceholderPage
            title="База знаний"
            block="Ф3"
            owner="fe-2"
            description="Публичное дерево статей (PUB-7) доступно без входа"
          />
        ),
      },
    ],
  },
];
