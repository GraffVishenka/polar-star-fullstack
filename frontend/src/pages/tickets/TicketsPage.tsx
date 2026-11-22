/**
 * `/tickets` — каркас будущей доски заявок (блок Ф2, агент fe-1).
 *
 * На месте страницы: заголовок, пояснение о правах по зданию (TICKET-7)
 * и пустое состояние. Доска в 4 колонки, фильтры и карточка — в Ф2.
 */
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Col, Flex, Row, Typography } from 'antd';
import { TICKET_STATUS_ORDER, TICKET_STATUS_LABELS } from '@/app/config';
import { BuildingBadge } from '@/components/BuildingBadge';
import { EmptyState } from '@/components/EmptyState';
import { PageHeader } from '@/components/PageHeader';
import { StatusTag } from '@/components/StatusTag';
import { useAuth } from '@/auth/AuthContext';

export function TicketsPage() {
  const { user, isAdmin, refreshUser } = useAuth();

  return (
    <Flex vertical gap={16} data-testid="tickets-page">
      <PageHeader
        title="Заявки"
        description={
          isAdmin
            ? 'Все здания · фильтры и доска появятся в блоке Ф2'
            : 'Только заявки вашего здания — ограничение действует на сервере (TICKET-7)'
        }
        extra={
          <Button type="primary" icon={<PlusOutlined />} disabled>
            Создать заявку
          </Button>
        }
      />

      <Flex gap={8} wrap align="center">
        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
          Здание:
        </Typography.Text>
        <BuildingBadge
          buildingId={user?.building_id}
          buildingName={user?.building_name}
          allBuildings={isAdmin}
        />
      </Flex>

      <Alert
        type="info"
        showIcon
        message="Доска заявок будет реализована в блоке Ф2"
        description="API, MSW-моки и общие компоненты (StatusTag, DeadlineBadge, FileUploader, ConfirmDialog) уже готовы: колонки, фильтры и карточка подключаются к контракту без ожидания бэкенда."
        action={
          <Button size="small" icon={<ReloadOutlined />} onClick={() => void refreshUser()}>
            Обновить профиль
          </Button>
        }
      />

      <Row gutter={[16, 16]}>
        {TICKET_STATUS_ORDER.map((status) => (
          <Col key={status} xs={24} sm={12} xl={6}>
            <Card
              size="small"
              title={
                <Flex justify="space-between" align="center">
                  <StatusTag status={status} label={TICKET_STATUS_LABELS[status]} />
                  <Typography.Text type="secondary">0</Typography.Text>
                </Flex>
              }
              data-testid={`ticket-column-${status}`}
            >
              <EmptyState
                compact
                title="Пока пусто"
                description="Карточки появятся после загрузки данных."
              />
            </Card>
          </Col>
        ))}
      </Row>
    </Flex>
  );
}
