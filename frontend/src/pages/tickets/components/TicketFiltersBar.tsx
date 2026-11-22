/**
 * Панель фильтров доски (TICKET-15, TICKET-7).
 *
 * Все значения живут в URL (`useTicketFilters`), поэтому фильтры переживают
 * перезагрузку и пересылаются ссылкой. У USER здание заблокировано на своём
 * (`BuildingSelect locked`), у ADMIN — «Все здания» по умолчанию.
 */
import { ClearOutlined, SearchOutlined } from '@ant-design/icons';
import { Button, Card, DatePicker, Flex, Input, Select, Space, Tooltip, Typography } from 'antd';
import dayjs, { type Dayjs } from 'dayjs';
import type { AssignableUser, ProblemType, TicketStatusSetting } from '@/api/types';
import { BuildingSelect } from '@/components/BuildingSelect';
import type { TicketFilterState, TicketSortBy, TicketSortDir } from '../hooks/useTicketFilters';
import { useEffect, useState } from 'react';

const { RangePicker } = DatePicker;

export interface TicketFiltersBarProps {
  filters: TicketFilterState;
  statuses: TicketStatusSetting[];
  problemTypes: ProblemType[];
  executors: AssignableUser[];
  buildings: Array<{ id: number; name: string }>;
  isAdmin: boolean;
  buildingsLoading?: boolean;
  executorsLoading?: boolean;
  hasActiveFilters: boolean;
  onChange: (patch: Partial<TicketFilterState>) => void;
  onReset: () => void;
  /** Полное имя сотрудника для подписи «Только мои». */
  userFullName?: string;
}

const SORT_OPTIONS: Array<{
  value: string;
  label: string;
  sortBy: TicketSortBy;
  sortDir: TicketSortDir;
}> = [
  { value: 'created_at:desc', label: 'Сначала новые', sortBy: 'created_at', sortDir: 'desc' },
  { value: 'created_at:asc', label: 'Сначала старые', sortBy: 'created_at', sortDir: 'asc' },
  { value: 'deadline:asc', label: 'По дедлайну', sortBy: 'deadline', sortDir: 'asc' },
  { value: 'deadline:desc', label: 'Дедлайн: дальние', sortBy: 'deadline', sortDir: 'desc' },
];

export function TicketFiltersBar({
  filters,
  statuses,
  problemTypes,
  executors,
  buildings,
  isAdmin,
  buildingsLoading,
  executorsLoading,
  hasActiveFilters,
  onChange,
  onReset,
  userFullName,
}: TicketFiltersBarProps) {
  const [searchDraft, setSearchDraft] = useState(filters.search);

  useEffect(() => {
    setSearchDraft(filters.search);
  }, [filters.search]);

  const executorValue =
    filters.executor === 'all' || filters.executor === 'none' || filters.executor === 'me'
      ? filters.executor
      : String(filters.executor);

  const executorOptions = [
    { value: 'all', label: 'Все исполнители' },
    { value: 'none', label: 'Без исполнителя' },
    {
      value: 'me',
      label: userFullName ? `Только мои (${userFullName})` : 'Только мои',
    },
    ...(isAdmin
      ? executors.map((user) => ({
          value: String(user.id),
          label: `${user.full_name} · ${user.role === 'ADMIN' ? 'администратор' : (user.building_name ?? 'без здания')}`,
        }))
      : []),
  ];

  const sortValue = `${filters.sortBy}:${filters.sortDir}`;

  const dateRange: [Dayjs | null, Dayjs | null] | null =
    filters.dateFrom || filters.dateTo
      ? [
          filters.dateFrom ? dayjs(filters.dateFrom) : null,
          filters.dateTo ? dayjs(filters.dateTo) : null,
        ]
      : null;

  return (
    <Card size="small" data-testid="ticket-filters">
      <Flex vertical gap={10}>
        <Flex gap={8} wrap align="center">
          <Input.Search
            allowClear
            value={searchDraft}
            onChange={(event) => setSearchDraft(event.target.value)}
            onSearch={(value) => onChange({ search: value.trim() })}
            placeholder="Заявитель или кабинет"
            prefix={<SearchOutlined />}
            style={{ width: 260 }}
            data-testid="filters-search"
            aria-label="Поиск по заявителю или кабинету"
          />

          <Select
            mode="multiple"
            allowClear
            placeholder="Все статусы"
            value={filters.statuses}
            options={statuses.map((status) => ({
              value: status.code,
              label: status.label,
            }))}
            onChange={(value) => onChange({ statuses: value })}
            style={{ minWidth: 200 }}
            maxTagCount="responsive"
            data-testid="filters-status"
            aria-label="Фильтр по статусам"
          />

          <Tooltip title={isAdmin ? undefined : 'Доступно только ваше здание'}>
            <span style={{ display: 'inline-flex', minWidth: 200 }}>
              <BuildingSelect
                buildings={buildings}
                value={filters.buildingId === 'all' ? null : filters.buildingId}
                onChange={(id) => onChange({ buildingId: id ?? 'all' })}
                locked={!isAdmin}
                loading={buildingsLoading}
                allowAll={isAdmin}
                size="middle"
                className="ticket-filters__building"
              />
            </span>
          </Tooltip>

          <Select
            value={executorValue}
            options={executorOptions}
            onChange={(value) => {
              if (value === 'all' || value === 'none' || value === 'me') {
                onChange({ executor: value });
                return;
              }
              onChange({ executor: Number(value) });
            }}
            loading={executorsLoading}
            style={{ minWidth: 200 }}
            data-testid="filters-executor"
            aria-label="Фильтр по исполнителю"
          />

          <Select
            allowClear
            placeholder="Вид проблемы"
            value={filters.problemTypeId ?? undefined}
            options={problemTypes.map((type) => ({ value: type.id, label: type.name }))}
            onChange={(value) => onChange({ problemTypeId: value ?? null })}
            style={{ minWidth: 190 }}
            data-testid="filters-problem-type"
            aria-label="Фильтр по виду проблемы"
          />

          <RangePicker
            value={dateRange}
            onChange={(values) =>
              onChange({
                dateFrom: values?.[0]?.isValid() ? values[0]!.format('YYYY-MM-DD') : null,
                dateTo: values?.[1]?.isValid() ? values[1]!.format('YYYY-MM-DD') : null,
              })
            }
            format="DD.MM.YYYY"
            placeholder={['Создана с', 'по']}
            style={{ width: 260 }}
            data-testid="filters-dates"
            aria-label="Диапазон дат создания"
          />

          <Select
            value={sortValue}
            options={SORT_OPTIONS.map(({ value, label }) => ({ value, label }))}
            onChange={(value) => {
              const option = SORT_OPTIONS.find((item) => item.value === value);
              if (option) onChange({ sortBy: option.sortBy, sortDir: option.sortDir });
            }}
            style={{ width: 190 }}
            data-testid="filters-sort"
            aria-label="Сортировка"
          />

          <Space size={4}>
            <Button
              icon={<ClearOutlined />}
              onClick={onReset}
              disabled={!hasActiveFilters}
              data-testid="filters-reset"
            >
              Сбросить
            </Button>
          </Space>
        </Flex>

        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {isAdmin
            ? 'ADMIN видит все здания; фильтр здания по умолчанию «Все здания».'
            : 'USER видит только заявки своего здания — ограничение проверяет сервер (TICKET-7).'}
        </Typography.Text>
      </Flex>
    </Card>
  );
}
