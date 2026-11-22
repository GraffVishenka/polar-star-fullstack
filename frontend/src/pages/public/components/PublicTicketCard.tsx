/**
 * Публичная карточка дашборда (TICKET-5).
 *
 * Только разрешённые поля: id, здание (в сводке), заявитель, кабинет, вид
 * проблемы, статус, дедлайн, исполнитель. Ни описания, ни комментариев, ни
 * вложений — детали доступны исключительно по токен-ссылке (TICKET-13).
 *
 * Просрочку публичный API не отдаёт (`is_overdue` только в токен-карточке),
 * поэтому дедлайн показываем без клиентских вычислений (NFR-7).
 */
import { UserOutlined } from '@ant-design/icons';
import { Card, Flex, Tag, Tooltip, Typography } from 'antd';
import type { PublicTicketCard as PublicTicketCardModel } from '@/api/types';
import { DeadlineBadge } from '@/components/DeadlineBadge';
import { StatusTag } from '@/components/StatusTag';

export interface PublicTicketCardProps {
  ticket: PublicTicketCardModel;
  /** В режиме «Все здания» показываем здание в карточке. */
  showBuilding?: boolean;
}

export function PublicTicketCard({ ticket, showBuilding = false }: PublicTicketCardProps) {
  return (
    <Card
      size="small"
      className="ticket-card"
      data-testid={`public-ticket-card-${ticket.id}`}
      data-status={ticket.status}
    >
      <Flex vertical gap={6}>
        <Flex align="center" justify="space-between" gap={8}>
          <Typography.Text strong>№{ticket.id}</Typography.Text>
          <StatusTag
            status={ticket.status}
            label={ticket.status_label}
            color={ticket.status_color}
            size="small"
          />
        </Flex>

        {showBuilding ? (
          <Tag color="geekblue" style={{ marginInlineEnd: 0, width: 'fit-content' }}>
            {ticket.building_name}
          </Tag>
        ) : null}

        <Tooltip title={ticket.applicant_name}>
          <Typography.Text ellipsis style={{ fontSize: 14 }}>
            {ticket.applicant_name}
          </Typography.Text>
        </Tooltip>

        <Typography.Text type="secondary" style={{ fontSize: 12 }} ellipsis>
          {ticket.cabinet} · {ticket.problem_type_name}
        </Typography.Text>

        <DeadlineBadge deadline={ticket.deadline} size="small" />

        <Typography.Text type="secondary" style={{ fontSize: 12 }} ellipsis>
          <UserOutlined /> {ticket.executor_name ?? 'Не назначен'}
        </Typography.Text>
      </Flex>
    </Card>
  );
}
