/**
 * `/b/:buildingId` — публичный дашборд здания (заглушка Ф1, реализация — Ф5).
 *
 * Уже сейчас показывает реальные счётчики и статусы из публичного API:
 * три колонки без `done` (TICKET-5), никаких закрытых полей (description,
 * комментарии, файлы) здесь быть не может.
 * Режим `/b/all` — сводный обзор «Все здания» только для чтения (PUB-4).
 */
import { useMemo } from 'react';
import { BankOutlined, EnvironmentOutlined, PlusOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Col, Flex, Row, Skeleton, Tooltip, Typography } from 'antd';
import { Link, useParams } from 'react-router-dom';
import {
  APP_NAME,
  PUBLIC_STATUS_ORDER,
  TICKET_STATUS_COLORS,
  TICKET_STATUS_LABELS,
} from '@/app/config';
import { BuildingBadge } from '@/components/BuildingBadge';
import { EmptyState } from '@/components/EmptyState';
import { StatusTag } from '@/components/StatusTag';
import { useAnonBuilding } from '@/hooks/useAnonBuilding';

export function BuildingDashboardPage() {
  const { buildingId } = useParams<{ buildingId: string }>();
  const { buildings, isLoading, isError, error, refetch } = useAnonBuilding();

  const isAllBuildings = buildingId === 'all';
  const numericId = useMemo(() => {
    const parsed = Number(buildingId);
    return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
  }, [buildingId]);

  const building = useMemo(
    () => (numericId ? (buildings.find((item) => item.id === numericId) ?? null) : null),
    [buildings, numericId],
  );

  const totals = useMemo(() => {
    const sum = { new: 0, in_progress: 0, deferred: 0 };
    buildings.forEach((item) => {
      sum.new += item.counts.new;
      sum.in_progress += item.counts.in_progress;
      sum.deferred += item.counts.deferred;
    });
    return sum;
  }, [buildings]);

  if (isLoading) {
    return (
      <Flex vertical gap={16}>
        <Skeleton active title paragraph={{ rows: 2 }} />
        <Skeleton active paragraph={{ rows: 4 }} />
      </Flex>
    );
  }

  if (isError) {
    return (
      <Alert
        type="error"
        showIcon
        message="Не удалось загрузить данные здания"
        description={error?.message}
        action={
          <Button size="small" onClick={refetch}>
            Повторить
          </Button>
        }
      />
    );
  }

  if (!isAllBuildings && !building) {
    return (
      <EmptyState
        title="Здание не найдено"
        description="Возможно, здание деактивировано. Выберите другое здание из списка."
        action={{ label: 'К выбору здания', onClick: () => window.location.assign('/?change=1') }}
      />
    );
  }

  const counts = isAllBuildings ? totals : building!.counts;
  const countByStatus = {
    new: counts.new,
    in_progress: counts.in_progress,
    deferred: counts.deferred,
    done: 0,
  };

  return (
    <Flex vertical gap={20} data-testid="building-dashboard-page">
      <Flex justify="space-between" align="flex-start" gap={12} wrap>
        <Flex vertical gap={6}>
          <Flex align="center" gap={8} wrap>
            <BankOutlined style={{ fontSize: 22, color: '#1677ff' }} />
            <Typography.Title level={2} style={{ margin: 0, fontSize: 'clamp(20px, 5vw, 28px)' }}>
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
              : `Публичный дашборд ${APP_NAME} · заявки обновляются автоматически`}
          </Typography.Text>
        </Flex>

        <Flex gap={8} wrap>
          <Link to="/?change=1">
            <Button>Сменить здание</Button>
          </Link>
          <Tooltip
            title={
              isAllBuildings ? 'Выберите здание для создания заявки' : 'Форма создания — в блоке Ф5'
            }
          >
            <Button
              type="primary"
              icon={<PlusOutlined />}
              disabled
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
            <Card
              size="small"
              title={
                <Flex align="center" justify="space-between" gap={8}>
                  <StatusTag
                    status={status}
                    label={TICKET_STATUS_LABELS[status]}
                    color={TICKET_STATUS_COLORS[status]}
                  />
                  <Typography.Text type="secondary">{countByStatus[status]}</Typography.Text>
                </Flex>
              }
              data-testid={`public-column-${status}`}
            >
              {countByStatus[status] === 0 ? (
                <EmptyState compact title="Заявок нет" description="Новые заявки появятся здесь." />
              ) : (
                <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                  Карточки заявок и фильтры появятся в блоке Ф5. Счётчик отражает реальные данные из
                  публичного API.
                </Typography.Text>
              )}
            </Card>
          </Col>
        ))}
      </Row>

      <Alert
        type="info"
        showIcon
        message="Публичная сводка"
        description="Публичный периметр показывает только безопасные поля: заявитель, кабинет, вид проблемы, статус, дедлайн и исполнителя. Описание, комментарии и вложения доступны только по ссылке-токену заявки."
      />
    </Flex>
  );
}
