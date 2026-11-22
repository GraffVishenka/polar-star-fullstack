/** Карточки метрик статистики (STATS-1/STATS-4): null показываем как «—». */
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  FieldTimeOutlined,
  PercentageOutlined,
  WarningOutlined,
  RiseOutlined,
} from '@ant-design/icons';
import { Card, Col, Flex, Row, Typography } from 'antd';
import type { ReactNode } from 'react';
import type { StatsResponse } from '@/api/types';
import { formatCount, formatHours, formatPercent } from '../period';

export interface StatsMetricCardsProps {
  stats: StatsResponse;
}

interface MetricCard {
  key: string;
  title: string;
  value: string;
  hint: string;
  icon: ReactNode;
  danger?: boolean;
}

export function buildMetricCards(stats: StatsResponse): MetricCard[] {
  return [
    {
      key: 'total_done',
      title: 'Завершено всего',
      value: formatCount(stats.total_done),
      hint: 'за всё время',
      icon: <CheckCircleOutlined />,
    },
    {
      key: 'done_in_period',
      title: 'Завершено за период',
      value: formatCount(stats.done_in_period),
      hint: 'по выбранному периоду',
      icon: <RiseOutlined />,
    },
    {
      key: 'avg_resolution_hours',
      title: 'Среднее время',
      value: formatHours(stats.avg_resolution_hours),
      hint: 'от создания до завершения',
      icon: <ClockCircleOutlined />,
    },
    {
      key: 'median_resolution_hours',
      title: 'Медиана',
      value: formatHours(stats.median_resolution_hours),
      hint: 'устойчиво к выбросам',
      icon: <FieldTimeOutlined />,
    },
    {
      key: 'on_time_percent',
      title: 'В срок',
      value: formatPercent(stats.on_time_percent),
      hint: 'доля завершённых до дедлайна',
      icon: <PercentageOutlined />,
    },
    {
      key: 'overdue_count',
      title: 'Просрочено',
      value: formatCount(stats.overdue_count),
      hint: 'завершены позже дедлайна',
      icon: <WarningOutlined />,
      danger: stats.overdue_count > 0,
    },
  ];
}

export function StatsMetricCards({ stats }: StatsMetricCardsProps) {
  const cards = buildMetricCards(stats);

  return (
    <Row gutter={[12, 12]} data-testid="stats-metrics">
      {cards.map((card) => (
        <Col key={card.key} xs={24} sm={12} xl={8}>
          <Card size="small" data-testid={`metric-${card.key}`}>
            <Flex vertical gap={2}>
              <Flex justify="space-between" align="center" gap={8}>
                <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                  {card.title}
                </Typography.Text>
                <span style={{ color: card.danger ? '#cf1322' : '#1677ff' }}>{card.icon}</span>
              </Flex>
              <Typography.Text
                strong
                style={{
                  fontSize: 26,
                  lineHeight: 1.2,
                  color: card.danger ? '#cf1322' : undefined,
                }}
                data-testid={`metric-value-${card.key}`}
              >
                {card.value}
              </Typography.Text>
              <Typography.Text type="secondary" style={{ fontSize: 11 }}>
                {card.hint}
              </Typography.Text>
            </Flex>
          </Card>
        </Col>
      ))}
    </Row>
  );
}
