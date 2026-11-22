/**
 * Сводная таблица по сотрудникам для `/admin/stats` (STATS-1).
 *
 * Рендерится только если API прислал `employees` (аддитивное расширение,
 * согласуется с lead/be; в MSW уже смоделировано). Если поля нет — блок скрыт,
 * страница работает на одиночной статистике выбранного сотрудника.
 */
import { Card, Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { formatCount, formatHours, formatPercent } from '../period';
import type { EmployeeStats } from '../types';

export interface EmployeeSummaryTableProps {
  employees?: EmployeeStats[];
}

const columns: ColumnsType<EmployeeStats> = [
  {
    title: 'Сотрудник',
    dataIndex: 'user_name',
    key: 'user_name',
    ellipsis: true,
  },
  {
    title: 'Завершено',
    dataIndex: 'total_done',
    key: 'total_done',
    align: 'right',
    render: (value: number) => formatCount(value),
  },
  {
    title: 'За период',
    dataIndex: 'done_in_period',
    key: 'done_in_period',
    align: 'right',
    render: (value: number) => formatCount(value),
  },
  {
    title: 'В срок',
    dataIndex: 'on_time_percent',
    key: 'on_time_percent',
    align: 'right',
    render: (value: number | null) => formatPercent(value),
  },
  {
    title: 'Среднее',
    dataIndex: 'avg_resolution_hours',
    key: 'avg_resolution_hours',
    align: 'right',
    render: (value: number | null) => formatHours(value),
  },
  {
    title: 'Медиана',
    dataIndex: 'median_resolution_hours',
    key: 'median_resolution_hours',
    align: 'right',
    render: (value: number | null) => formatHours(value),
  },
  {
    title: 'Просрочено',
    dataIndex: 'overdue_count',
    key: 'overdue_count',
    align: 'right',
    render: (value: number) => formatCount(value),
  },
];

export function EmployeeSummaryTable({ employees }: EmployeeSummaryTableProps) {
  if (!employees?.length) return null;

  return (
    <Card size="small" title="Сводка по сотрудникам" data-testid="employee-summary">
      <Table<EmployeeStats>
        size="small"
        rowKey="user_id"
        pagination={false}
        columns={columns}
        dataSource={employees}
        scroll={{ x: 720 }}
      />
    </Card>
  );
}
