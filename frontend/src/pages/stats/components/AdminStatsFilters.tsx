/**
 * Фильтры `/admin/stats` (STATS-1): сотрудник (поиск по ФИО), здание, период.
 * USER сюда не попадает — маршрут закрыт `RequireRole('ADMIN')`.
 */
import { Card, Flex, Select } from 'antd';
import type { AdminUser } from '@/api/types';
import { BuildingSelect, type BuildingOption } from '@/components/BuildingSelect';
import type { StatsPeriod } from '../period';
import { StatsPeriodFilter } from './StatsPeriodFilter';

export interface AdminStatsFiltersProps {
  users: AdminUser[];
  buildings: BuildingOption[];
  userId: number | null;
  buildingId: number | null;
  period: StatsPeriod;
  onUserId: (userId: number | null) => void;
  onBuildingId: (buildingId: number | null) => void;
  onPeriod: (period: StatsPeriod) => void;
  usersLoading?: boolean;
  buildingsLoading?: boolean;
  /** Блокирует фильтры, пока грузится статистика. */
  disabled?: boolean;
}

function userOptionLabel(user: AdminUser): string {
  const building = user.building_name ? ` · ${user.building_name}` : '';
  const role = user.role === 'ADMIN' ? ' · администратор' : '';
  return `${user.full_name}${building}${role}`;
}

export function AdminStatsFilters({
  users,
  buildings,
  userId,
  buildingId,
  period,
  onUserId,
  onBuildingId,
  onPeriod,
  usersLoading = false,
  buildingsLoading = false,
  disabled = false,
}: AdminStatsFiltersProps) {
  return (
    <Card size="small" data-testid="admin-stats-filters">
      <Flex gap={12} wrap align="center">
        <Select<number>
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder="Все сотрудники"
          style={{ minWidth: 280, flex: '1 1 280px', maxWidth: 420 }}
          loading={usersLoading}
          disabled={disabled}
          value={userId ?? undefined}
          onChange={(value) => onUserId(value ?? null)}
          options={users.map((user) => ({
            value: user.id,
            label: userOptionLabel(user),
          }))}
          data-testid="admin-stats-user"
          aria-label="Сотрудник"
        />
        <BuildingSelect
          buildings={buildings}
          value={buildingId}
          onChange={onBuildingId}
          allowAll
          loading={buildingsLoading}
          disabled={disabled}
          placeholder="Все здания"
          id="admin-stats-building"
        />
        <StatsPeriodFilter
          period={period}
          onChange={onPeriod}
          loading={disabled}
          testIdPrefix="admin-stats-period"
        />
      </Flex>
    </Card>
  );
}
