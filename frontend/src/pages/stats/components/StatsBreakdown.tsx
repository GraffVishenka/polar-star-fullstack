/**
 * Разбивки статистики: по видам проблем и по месяцам (STATS-2).
 * Если сервер не прислал данные — пустое состояние, а не пустая таблица.
 */
import { Card, Col, Empty, Flex, Progress, Row, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { MonthStat, ProblemTypeStat, StatsResponse } from '@/api/types';
import { formatCount, formatMonthLabel } from '../period';

interface ProblemRow extends ProblemTypeStat {
  key: number;
}

interface MonthRow extends MonthStat {
  key: string;
}

const problemColumns: ColumnsType<ProblemRow> = [
  {
    title: 'Вид проблемы',
    dataIndex: 'problem_type_name',
    key: 'name',
    ellipsis: true,
  },
  {
    title: 'Всего',
    dataIndex: 'total',
    key: 'total',
    width: 90,
    align: 'right',
    render: (value: number) => formatCount(value),
  },
  {
    title: 'Завершено',
    dataIndex: 'done',
    key: 'done',
    width: 110,
    align: 'right',
    render: (value: number) => formatCount(value),
  },
  {
    title: 'Доля',
    key: 'share',
    width: 160,
    render: (_, row) => {
      const percent = row.total > 0 ? Math.round((row.done / row.total) * 100) : 0;
      return (
        <Flex align="center" gap={8}>
          <Progress
            percent={percent}
            size="small"
            showInfo={false}
            style={{ width: 80, marginBottom: 0 }}
          />
          <Typography.Text type="secondary">{percent}%</Typography.Text>
        </Flex>
      );
    },
  },
];

const monthColumns = (maxDone: number): ColumnsType<MonthRow> => [
  {
    title: 'Месяц',
    dataIndex: 'month',
    key: 'month',
    render: (value: string) => formatMonthLabel(value),
  },
  {
    title: 'Завершено',
    dataIndex: 'done',
    key: 'done',
    width: 200,
    render: (value: number) => (
      <Flex align="center" gap={8}>
        <Progress
          percent={maxDone > 0 ? Math.round((value / maxDone) * 100) : 0}
          size="small"
          showInfo={false}
          style={{ width: 120, marginBottom: 0 }}
        />
        <Typography.Text>{formatCount(value)}</Typography.Text>
      </Flex>
    ),
  },
];

export interface StatsBreakdownProps {
  stats: StatsResponse;
}

export function StatsBreakdown({ stats }: StatsBreakdownProps) {
  const problemRows: ProblemRow[] = stats.by_problem_type.map((row) => ({
    ...row,
    key: row.problem_type_id,
  }));
  const monthRows: MonthRow[] = stats.by_month.map((row) => ({ ...row, key: row.month }));
  const maxDone = monthRows.reduce((max, row) => Math.max(max, row.done), 0);

  return (
    <Row gutter={[12, 12]}>
      <Col xs={24} xl={12}>
        <Card size="small" title="По видам проблем" data-testid="breakdown-problem-types">
          {problemRows.length ? (
            <Table<ProblemRow>
              size="small"
              pagination={false}
              columns={problemColumns}
              dataSource={problemRows}
              scroll={{ x: 480 }}
            />
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Нет данных за период" />
          )}
        </Card>
      </Col>
      <Col xs={24} xl={12}>
        <Card size="small" title="По месяцам" data-testid="breakdown-months">
          {monthRows.length ? (
            <Table<MonthRow>
              size="small"
              pagination={false}
              columns={monthColumns(maxDone)}
              dataSource={monthRows}
              scroll={{ x: 360 }}
            />
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Нет данных за период" />
          )}
        </Card>
      </Col>
    </Row>
  );
}
