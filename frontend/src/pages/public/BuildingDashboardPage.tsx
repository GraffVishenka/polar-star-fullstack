/**
 * `/b/:buildingId` — публичный дашборд здания (PUB-2, PUB-4, TICKET-5).
 *
 * Три колонки (`new`, `in_progress`, `deferred`) с карточками только из
 * разрешённых полей; `done` и внутренние данные наружу не отдаются. Режим
 * `/b/all` — сводка по всем активным зданиям, только чтение: кнопка создания
 * неактивна с подсказкой. Неактивное/несуществующее здание → 404.
 */
import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { BankOutlined, EnvironmentOutlined, PlusOutlined } from '@ant-design/icons';
import { Alert, Button, Col, Flex, Row, Skeleton, Tooltip, Typography } from 'antd';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { fetchPublicTickets } from '@/api/public';
import { queryKeys } from '@/app/queryClient';
import {
  APP_NAME,
  PUBLIC_STATUS_ORDER,
  TICKET_STATUS_COLORS,
  TICKET_STATUS_LABELS,
} from '@/app/config';
import { BuildingBadge } from '@/components/BuildingBadge';
import { PublicNotFound } from './components/PublicNotFound';
import { PublicStatusColumn } from './components/PublicStatusColumn';
import { PublicTicketCreateModal } from './components/PublicTicketCreateModal';
import { useAnonBuilding } from '@/hooks/useAnonBuilding';

export function BuildingDashboardPage() {
  const { buildingId } = useParams<{ buildingId: string }>();
  const navigate = useNavigate();
  const {
    buildings,
    isLoading: buildingsLoading,
    isError,
    error,
    refetch,
    selectBuilding,
  } = useAnonBuilding();

  const [createOpen, setCreateOpen] = useState(false);

  const isAllBuildings = buildingId === 'all';
  const numericId = useMemo(() => {
    const parsed = Number(buildingId);
    return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
  }, [buildingId]);

  const building = useMemo(
    () => (numericId ? (buildings.find((item) => item.id === numericId) ?? null) : null),
    [buildings, numericId],
  );

  const ticketsQuery = useQuery({
    queryKey: queryKeys.publicTickets(isAllBuildings ? null : numericId),
    queryFn: () => fetchPublicTickets(isAllBuildings ? null : numericId),
    enabled: isAllBuildings || numericId !== null,
    staleTime: 15_000,
    retry: false,
  });

  const tickets = useMemo(() => ticketsQuery.data?.items ?? [], [ticketsQuery.data]);

  const grouped = useMemo(() => {
    const map: Record<string, typeof tickets> = { new: [], in_progress: [], deferred: [] };
    tickets.forEach((ticket) => {
      if (map[ticket.status]) map[ticket.status]!.push(ticket);
    });
    return map;
  }, [tickets]);

  /** Подписи/цвета статусов: серверные из карточек, иначе значения по умолчанию. */
  const statusMeta = (status: (typeof PUBLIC_STATUS_ORDER)[number]) => {
    const sample = tickets.find((ticket) => ticket.status === status);
    return {
      label: sample?.status_label ?? TICKET_STATUS_LABELS[status],
      color: sample?.status_color ?? TICKET_STATUS_COLORS[status],
    };
  };

  const isNotFound =
    ticketsQuery.isError && (ticketsQuery.error as { status?: number } | null)?.status === 404;

  if (!isAllBuildings && buildingId !== undefined && numericId === null) {
    return <PublicNotFound title="Здание не найдено" />;
  }

  if (isNotFound) {
    return (
      <PublicNotFound
        title="Здание не найдено"
        description="Возможно, здание деактивировано. Выберите другое здание из списка."
      />
    );
  }

  if (buildingsLoading || (ticketsQuery.isLoading && !ticketsQuery.isError)) {
    return (
      <Flex vertical gap={16} data-testid="public-dashboard-loading">
        <Skeleton active title paragraph={{ rows: 2 }} />
        <Row gutter={[16, 16]}>
          {[0, 1, 2].map((key) => (
            <Col key={key} xs={24} md={8}>
              <Skeleton active paragraph={{ rows: 4 }} />
            </Col>
          ))}
        </Row>
      </Flex>
    );
  }

  if (isError) {
    return (
      <Alert
        type="error"
        showIcon
        message="Не удалось загрузить список зданий"
        description={error?.message}
        action={
          <Button size="small" onClick={refetch}>
            Повторить
          </Button>
        }
        data-testid="public-dashboard-error"
      />
    );
  }

  if (!isAllBuildings && !building) {
    return (
      <PublicNotFound
        title="Здание не найдено"
        description="Возможно, здание деактивировано. Выберите другое здание из списка."
      />
    );
  }

  if (ticketsQuery.isError) {
    return (
      <Alert
        type="error"
        showIcon
        message="Не удалось загрузить заявки"
        description="Проверьте соединение и попробуйте ещё раз."
        action={
          <Button size="small" onClick={() => void ticketsQuery.refetch()}>
            Повторить
          </Button>
        }
        data-testid="public-tickets-error"
      />
    );
  }

  const totalOpen = tickets.length;

  return (
    <Flex vertical gap={20} data-testid="building-dashboard-page">
      <Flex justify="space-between" align="flex-start" gap={12} wrap>
        <Flex vertical gap={6}>
          <Flex align="center" gap={8} wrap>
            <BankOutlined style={{ fontSize: 22, color: '#1677ff' }} />
            <Typography.Title level={1} style={{ margin: 0, fontSize: 'clamp(20px, 5vw, 28px)' }}>
              {isAllBuildings ? 'Все здания' : building!.name}
            </Typography.Title>
            {isAllBuildings ? (
              <BuildingBadge allBuildings />
            ) : (
              <BuildingBadge buildingId={building!.id} buildingName={building!.name} />
            )}
          </Flex>
          {!isAllBuildings && building!.address ? (
            <Typography.Text type="secondary">
              <EnvironmentOutlined /> {building!.address}
            </Typography.Text>
          ) : null}
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {isAllBuildings
              ? `Сводный обзор по ${buildings.length} активным зданиям · только чтение`
              : `${APP_NAME} · открытых заявок: ${totalOpen}`}
          </Typography.Text>
        </Flex>

        <Flex gap={8} wrap>
          <Link to="/?change=1">
            <Button data-testid="change-building">Сменить здание</Button>
          </Link>
          <Tooltip title={isAllBuildings ? 'Выберите здание для создания заявки' : undefined}>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              disabled={isAllBuildings}
              onClick={() => setCreateOpen(true)}
              data-testid="create-ticket-button"
            >
              Создать заявку
            </Button>
          </Tooltip>
        </Flex>
      </Flex>

      <Row gutter={[16, 16]} data-testid="public-status-columns">
        {PUBLIC_STATUS_ORDER.map((status) => (
          <Col key={status} xs={24} md={8}>
            <PublicStatusColumn
              status={status}
              label={statusMeta(status).label}
              color={statusMeta(status).color}
              tickets={grouped[status] ?? []}
              showBuilding={isAllBuildings}
            />
          </Col>
        ))}
      </Row>

      <Alert
        type="info"
        showIcon
        message="Публичная сводка"
        description="Публично видны только заявитель, кабинет, вид проблемы, статус, дедлайн и исполнитель. Описание, комментарии и вложения доступны по персональной ссылке заявки."
      />

      {!isAllBuildings && building ? (
        <PublicTicketCreateModal
          open={createOpen}
          defaultBuildingId={building.id}
          buildings={buildings.map(({ id, name }) => ({ id, name }))}
          buildingsLoading={buildingsLoading}
          onClose={() => setCreateOpen(false)}
          onCreated={({ buildingId: createdBuildingId }) => {
            // PUB-3: cookie обновляется на здание созданной заявки.
            selectBuilding(createdBuildingId);
          }}
          onGoToDashboard={(createdBuildingId) => {
            selectBuilding(createdBuildingId);
            setCreateOpen(false);
            navigate(`/b/${createdBuildingId}`);
          }}
        />
      ) : null}
    </Flex>
  );
}
