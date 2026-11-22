/** Колонка публичной доски: статус, счётчик, карточки (TICKET-5). */
import { Badge, Card, Flex, Typography } from 'antd';
import type { PublicTicketCard as PublicTicketCardModel, TicketStatus } from '@/api/types';
import { EmptyState } from '@/components/EmptyState';
import { StatusTag } from '@/components/StatusTag';
import { PublicTicketCard } from './PublicTicketCard';

export interface PublicStatusColumnProps {
  status: TicketStatus;
  label: string;
  color: string;
  tickets: PublicTicketCardModel[];
  showBuilding?: boolean;
  onOpenDetails?: (ticket: PublicTicketCardModel) => void;
}

export function PublicStatusColumn({
  status,
  label,
  color,
  tickets,
  showBuilding,
}: PublicStatusColumnProps) {
  return (
    <Card
      className="public-column"
      data-testid={`public-column-${status}`}
      styles={{ body: { padding: 12 } }}
      title={
        <Flex align="center" justify="space-between" gap={8}>
          <StatusTag status={status} label={label} color={color} />
          <Badge
            count={tickets.length}
            showZero
            overflowCount={9999}
            color="#d9d9d9"
            data-testid={`public-column-count-${status}`}
            style={{ color: '#595959', fontWeight: 600 }}
          />
        </Flex>
      }
    >
      {tickets.length === 0 ? (
        <EmptyState compact title="Заявок нет" description="Новые заявки появятся здесь." />
      ) : (
        <Flex vertical gap={8}>
          {tickets.map((ticket) => (
            <PublicTicketCard key={ticket.id} ticket={ticket} showBuilding={showBuilding} />
          ))}
        </Flex>
      )}
      {tickets.length > 0 ? (
        <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginTop: 8 }}>
          Детали заявки доступны по ссылке, которую получил заявитель.
        </Typography.Text>
      ) : null}
    </Card>
  );
}
