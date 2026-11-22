/**
 * `/admin/stats` — статистика сотрудников (STATS-1, STATS-3).
 *
 * Доступ только ADMIN (маршрут закрыт `RequireRole`). Фильтры: сотрудник
 * (`/api/admin/users`, поиск по ФИО), активные здания, период. Метрики — как на
 * `/stats`; если API прислал `employees` (агрегатный режим без `user_id`),
 * показываем сводную таблицу по сотрудникам. Excel — `StatsExportButton`
 * с `user_id/building_id/from/to`.
 */
import { useMemo, useState } from 'react';
import { ReloadOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Flex, Skeleton } from 'antd';
import type { AdminStatsParams } from '@/api/types';
import { ApiError } from '@/api/errors';
import { fetchAdminStats } from '@/api/stats';
import { fetchAdminUsers } from '@/api/admin';
import { fetchBuildings } from '@/api/equipment';
import { queryKeys } from '@/app/queryClient';
import type { BuildingOption } from '@/components/BuildingSelect';
import { EmptyState } from '@/components/EmptyState';
import { PageHeader } from '@/components/PageHeader';
import { StatsExportButton } from '@/components/exports';
import { AdminStatsFilters } from './components/AdminStatsFilters';
import { EmployeeSummaryTable } from './components/EmployeeSummaryTable';
import { StatsBreakdown } from './components/StatsBreakdown';
import { StatsMetricCards } from './components/StatsMetricCards';
import {
  defaultStatsPeriod,
  formatPeriodRange,
  isEmptyStats,
  toStatsParams,
  type StatsPeriod,
} from './period';
import type { StatsWithEmployees } from './types';

function statsErrorText(error: unknown): string {
  return error instanceof ApiError ? error.userMessage : 'Не удалось загрузить статистику';
}

export function AdminStatsPage() {
  const [userId, setUserId] = useState<number | null>(null);
  const [buildingId, setBuildingId] = useState<number | null>(null);
  const [period, setPeriod] = useState<StatsPeriod>(() => defaultStatsPeriod());

  const params = useMemo<AdminStatsParams>(
    () => ({
      ...toStatsParams(period),
      user_id: userId ?? undefined,
      building_id: buildingId ?? undefined,
    }),
    [buildingId, period, userId],
  );

  const statsQuery = useQuery({
    queryKey: queryKeys.adminStats(params),
    queryFn: () => fetchAdminStats(params),
  });
  const usersQuery = useQuery({
    queryKey: queryKeys.adminUsers({ size: 100, is_active: true }),
    queryFn: () => fetchAdminUsers({ size: 100, is_active: true }),
  });
  const buildingsQuery = useQuery({
    queryKey: queryKeys.buildings,
    queryFn: () => fetchBuildings({ is_active: true }),
  });

  const stats = statsQuery.data as StatsWithEmployees | undefined;
  const users = usersQuery.data?.items ?? [];
  const buildings: BuildingOption[] = (buildingsQuery.data ?? []).map((building) => ({
    id: building.id,
    name: building.name,
  }));
  const selectedUser = users.find((user) => user.id === userId);
  const scopeLabel = selectedUser ? selectedUser.full_name : 'Все сотрудники';
  const periodLabel = formatPeriodRange(period.range);

  return (
    <div data-testid="admin-stats-page">
      <PageHeader
        title="Статистика сотрудников"
        description={`${scopeLabel} · период ${periodLabel}`}
        extra={
          <Flex gap={8} wrap align="center">
            <StatsExportButton params={params} label="Excel" disabled={statsQuery.isFetching} />
            <Button
              icon={<ReloadOutlined />}
              onClick={() => void statsQuery.refetch()}
              loading={statsQuery.isFetching}
            >
              Обновить
            </Button>
          </Flex>
        }
      />

      <Flex vertical gap={12}>
        <AdminStatsFilters
          users={users}
          buildings={buildings}
          userId={userId}
          buildingId={buildingId}
          period={period}
          onUserId={setUserId}
          onBuildingId={setBuildingId}
          onPeriod={setPeriod}
          usersLoading={usersQuery.isLoading}
          buildingsLoading={buildingsQuery.isLoading}
          disabled={statsQuery.isFetching}
        />

        {usersQuery.isError ? (
          <Alert
            type="warning"
            showIcon
            message="Не удалось загрузить сотрудников"
            description={statsErrorText(usersQuery.error)}
          />
        ) : null}
        {buildingsQuery.isError ? (
          <Alert
            type="warning"
            showIcon
            message="Не удалось загрузить здания"
            description={statsErrorText(buildingsQuery.error)}
          />
        ) : null}

        {statsQuery.isError ? (
          <Alert
            type="error"
            showIcon
            message="Не удалось загрузить статистику"
            description={statsErrorText(statsQuery.error)}
            action={
              <Button size="small" onClick={() => void statsQuery.refetch()}>
                Повторить
              </Button>
            }
            data-testid="admin-stats-error"
          />
        ) : null}

        {statsQuery.isLoading ? (
          <Skeleton active paragraph={{ rows: 6 }} data-testid="admin-stats-loading" />
        ) : null}

        {stats ? (
          <>
            <StatsMetricCards stats={stats} />
            {/* Сводка приходит только в агрегатном режиме (без выбранного сотрудника). */}
            {userId === null ? <EmployeeSummaryTable employees={stats.employees} /> : null}
            {isEmptyStats(stats) ? (
              <EmptyState
                title="Нет завершённых заявок"
                description={`За период ${periodLabel} завершённых заявок нет. Измените период или снимите фильтры.`}
              />
            ) : (
              <StatsBreakdown stats={stats} />
            )}
          </>
        ) : null}
      </Flex>
    </div>
  );
}
