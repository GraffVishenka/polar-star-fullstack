/**
 * `/stats` — личная статистика сотрудника (STATS-1, STATS-2, STATS-4).
 *
 * Метрики считает сервер: `total_done`, `done_in_period`, `avg/median` часов,
 * `on_time_percent`, `overdue_count`; `null` показываем как «—». Период — пресеты
 * (месяц/квартал/год) или произвольный диапазон, по умолчанию текущий месяц.
 * Пустое состояние — «нет завершённых заявок»; Excel — `StatsExportButton`.
 */
import { useMemo, useState } from 'react';
import { ReloadOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Flex, Skeleton } from 'antd';
import { ApiError } from '@/api/errors';
import { fetchMyStats } from '@/api/stats';
import { useAuth } from '@/auth';
import { queryKeys } from '@/app/queryClient';
import { EmptyState } from '@/components/EmptyState';
import { PageHeader } from '@/components/PageHeader';
import { StatsExportButton } from '@/components/exports';
import { StatsBreakdown } from './components/StatsBreakdown';
import { StatsMetricCards } from './components/StatsMetricCards';
import { StatsPeriodFilter } from './components/StatsPeriodFilter';
import {
  defaultStatsPeriod,
  formatPeriodRange,
  isEmptyStats,
  toStatsParams,
  type StatsPeriod,
} from './period';

function statsErrorText(error: unknown): string {
  return error instanceof ApiError ? error.userMessage : 'Не удалось загрузить статистику';
}

export function StatsPage() {
  const { user } = useAuth();
  const [period, setPeriod] = useState<StatsPeriod>(() => defaultStatsPeriod());
  const params = useMemo(() => toStatsParams(period), [period]);

  const statsQuery = useQuery({
    queryKey: queryKeys.myStats(params),
    queryFn: () => fetchMyStats(params),
  });

  const stats = statsQuery.data;
  const periodLabel = formatPeriodRange(period.range);
  const subtitle = `${stats?.user_name ?? user?.full_name ?? 'Сотрудник'} · период ${periodLabel}`;

  return (
    <div data-testid="stats-page">
      <PageHeader
        title="Моя статистика"
        description={subtitle}
        extra={
          <Flex gap={8} wrap align="center">
            <StatsPeriodFilter
              period={period}
              onChange={setPeriod}
              loading={statsQuery.isFetching}
            />
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
          style={{ marginBottom: 12 }}
          data-testid="stats-error"
        />
      ) : null}

      {statsQuery.isLoading ? (
        <Skeleton active paragraph={{ rows: 6 }} data-testid="stats-loading" />
      ) : null}

      {stats ? (
        <Flex vertical gap={12}>
          <StatsMetricCards stats={stats} />
          {isEmptyStats(stats) ? (
            <EmptyState
              title="Нет завершённых заявок"
              description={`За период ${periodLabel} завершённых заявок нет. Измените период или дождитесь закрытия заявок.`}
            />
          ) : (
            <StatsBreakdown stats={stats} />
          )}
        </Flex>
      ) : null}
    </div>
  );
}
