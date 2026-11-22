/**
 * Колонка доски: серверный `total` в счётчике и кнопка «Показать ещё»
 * для следующей страницы (TICKET-15: `size` ≤ 100, пагинация на сервере).
 */
import { ReloadOutlined } from '@ant-design/icons';
import { Alert, Badge, Button, Card, Flex, Skeleton, Typography } from 'antd';
import type { TicketListParams, TicketStatus, TicketStatusSetting } from '@/api/types';
import { EmptyState } from '@/components/EmptyState';
import { StatusTag } from '@/components/StatusTag';
import { TicketBoardCard } from './TicketBoardCard';
import { useTicketColumn } from '../hooks/useTicketColumn';

export interface TicketColumnProps {
  status: TicketStatus;
  meta: TicketStatusSetting;
  apiParams: TicketListParams;
  filtersKey: string;
  onOpen: (id: number) => void;
}

export function TicketColumn({ status, meta, apiParams, filtersKey, onOpen }: TicketColumnProps) {
  // Фильтр по статусам (TICKET-15): если колонка не выбрана — не грузим её вовсе.
  const enabled = !apiParams.status?.length || apiParams.status.includes(status);
  const column = useTicketColumn(status, apiParams, filtersKey, enabled);

  return (
    <Card
      className="ticket-column"
      data-testid={`ticket-column-${status}`}
      title={
        <Flex align="center" justify="space-between" gap={8}>
          <StatusTag status={status} label={meta.label} color={meta.color} />
          <Badge
            count={column.total}
            showZero
            overflowCount={9999}
            color="#d9d9d9"
            data-testid={`ticket-column-count-${status}`}
            style={{ color: '#595959', fontWeight: 600 }}
          />
        </Flex>
      }
      styles={{ body: { padding: 12 } }}
    >
      {column.isError ? (
        <Alert
          type="error"
          showIcon
          message="Не удалось загрузить"
          action={
            <Button size="small" icon={<ReloadOutlined />} onClick={column.refetch}>
              Повторить
            </Button>
          }
          data-testid={`ticket-column-error-${status}`}
        />
      ) : null}

      {column.isLoading ? (
        <Flex vertical gap={8}>
          <Skeleton active paragraph={{ rows: 2 }} />
          <Skeleton active paragraph={{ rows: 2 }} />
        </Flex>
      ) : null}

      {!column.isLoading && !column.isError && !enabled ? (
        <EmptyState
          compact
          title="Статус не выбран"
          description="Колонка скрыта фильтром по статусам."
        />
      ) : null}

      {!column.isLoading && !column.isError && enabled && column.tickets.length === 0 ? (
        <EmptyState
          compact
          title="Пусто"
          description={
            apiParams.search || apiParams.executor_id !== undefined || apiParams.problem_type_id
              ? 'По текущим фильтрам заявок нет.'
              : 'Заявок в этом статусе пока нет.'
          }
        />
      ) : null}

      <Flex vertical gap={8} data-testid={`ticket-column-list-${status}`}>
        {column.tickets.map((ticket) => (
          <TicketBoardCard key={ticket.id} ticket={ticket} onOpen={onOpen} />
        ))}
      </Flex>

      {column.hasNextPage ? (
        <Flex justify="center" style={{ marginTop: 12 }}>
          <Button
            size="small"
            loading={column.isFetchingNextPage}
            onClick={column.fetchNextPage}
            data-testid={`ticket-column-more-${status}`}
          >
            Показать ещё
          </Button>
        </Flex>
      ) : null}

      {column.total > column.tickets.length && !column.hasNextPage ? (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
          Показано {column.tickets.length} из {column.total}
        </Typography.Text>
      ) : null}
    </Card>
  );
}
