/** Карточка заявки на доске: только безопасные поля, просрочка — по серверному флагу. */
import { CommentOutlined, UserOutlined } from '@ant-design/icons';
import { Badge, Card, Flex, Tag, Tooltip, Typography } from 'antd';
import type { Ticket } from '@/api/types';
import { DeadlineBadge } from '@/components/DeadlineBadge';

export interface TicketBoardCardProps {
  ticket: Ticket;
  onOpen: (id: number) => void;
}

export function TicketBoardCard({ ticket, onOpen }: TicketBoardCardProps) {
  return (
    <Card
      size="small"
      hoverable
      className="ticket-card"
      data-testid={`ticket-card-${ticket.id}`}
      data-status={ticket.status}
      data-overdue={ticket.is_overdue ? 'true' : 'false'}
      onClick={() => onOpen(ticket.id)}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          onOpen(ticket.id);
        }
      }}
      tabIndex={0}
      role="button"
      aria-label={`Открыть заявку №${ticket.id}`}
      style={ticket.is_overdue ? { borderInlineStart: '3px solid #cf1322' } : undefined}
    >
      <Flex vertical gap={6}>
        <Flex align="center" justify="space-between" gap={8}>
          <Typography.Text strong>№{ticket.id}</Typography.Text>
          {ticket.is_anonymous ? <Tag color="default">аноним</Tag> : null}
        </Flex>

        <Tooltip title={ticket.applicant_name}>
          <Typography.Text ellipsis style={{ fontSize: 14 }}>
            {ticket.applicant_name}
          </Typography.Text>
        </Tooltip>

        <Typography.Text type="secondary" style={{ fontSize: 12 }} ellipsis>
          {ticket.cabinet} · {ticket.problem_type_name}
        </Typography.Text>

        <DeadlineBadge deadline={ticket.deadline} isOverdue={ticket.is_overdue} size="small" />

        <Flex align="center" justify="space-between" gap={8}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }} ellipsis>
            <UserOutlined /> {ticket.executor_name ?? 'Не назначен'}
          </Typography.Text>
          {ticket.comments_count > 0 ? (
            <Badge
              count={ticket.comments_count}
              size="small"
              color="#8c8c8c"
              showZero={false}
              data-testid={`ticket-comments-count-${ticket.id}`}
            >
              <CommentOutlined style={{ color: '#8c8c8c' }} />
            </Badge>
          ) : null}
        </Flex>
      </Flex>
    </Card>
  );
}
