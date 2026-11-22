/**
 * `/` — выбор здания (§3.11 PUB-1, TICKET-6).
 *
 * Аноним видит только активные здания со счётчиками new/in_progress/deferred.
 * Если в cookie `anon_building_id` лежит доступное здание — сразу редирект на
 * `/b/{id}`. Ссылка «Сменить здание» приходит с `?change=1` и отключает
 * авторедирект, чтобы можно было выбрать другое здание.
 */
import { useCallback } from 'react';
import { BankOutlined, EnvironmentOutlined, ReloadOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Col, Flex, Row, Skeleton, Typography } from 'antd';
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom';
import { useAnonBuilding } from '@/hooks/useAnonBuilding';
import { EmptyState } from '@/components/EmptyState';
import type { PublicBuilding } from '@/api/types';

function Counters({ building }: { building: PublicBuilding }) {
  const { counts } = building;
  const cells = [
    { label: 'Новые', value: counts.new, color: '#1677ff' },
    { label: 'В работе', value: counts.in_progress, color: '#fa8c16' },
    { label: 'Отложены', value: counts.deferred, color: '#8c8c8c' },
  ];
  return (
    <Flex gap={16} wrap>
      {cells.map((cell) => (
        <Flex key={cell.label} vertical style={{ minWidth: 64 }}>
          <Typography.Text style={{ fontSize: 22, fontWeight: 600, color: cell.color }}>
            {cell.value}
          </Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {cell.label}
          </Typography.Text>
        </Flex>
      ))}
    </Flex>
  );
}

export function BuildingSelectPage() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const allowChoice = searchParams.get('change') === '1';
  const { buildings, isLoading, isError, error, redirectPath, selectBuilding, refetch } =
    useAnonBuilding();

  const handlePick = useCallback(
    (building: PublicBuilding) => {
      selectBuilding(building.id);
      navigate(`/b/${building.id}`);
    },
    [navigate, selectBuilding],
  );

  if (!allowChoice && redirectPath) return <Navigate to={redirectPath} replace />;

  return (
    <Flex vertical gap={24} data-testid="building-select-page">
      <Flex vertical gap={8} className="page-hero">
        <Typography.Title level={1} style={{ margin: 0, fontSize: 'clamp(24px, 6vw, 34px)' }}>
          Выберите здание
        </Typography.Title>
        <Typography.Paragraph type="secondary" style={{ margin: 0, maxWidth: 640 }}>
          Здесь собраны заявки по зданиям: новые, в работе и отложенные. Выберите своё здание —
          выбор запомнится на 30 дней, вернуться можно в любой момент.
        </Typography.Paragraph>
      </Flex>

      {isError ? (
        <Alert
          type="error"
          showIcon
          message="Не удалось загрузить список зданий"
          description={error?.message}
          action={
            <Button size="small" icon={<ReloadOutlined />} onClick={refetch}>
              Повторить
            </Button>
          }
          data-testid="buildings-error"
        />
      ) : null}

      {isLoading ? (
        <Row gutter={[16, 16]}>
          {[0, 1, 2].map((key) => (
            <Col key={key} xs={24} md={12} xl={8}>
              <Card>
                <Skeleton active paragraph={{ rows: 2 }} />
              </Card>
            </Col>
          ))}
        </Row>
      ) : null}

      {!isLoading && !isError && buildings.length === 0 ? (
        <EmptyState
          title="Активных зданий пока нет"
          description="Обратитесь к администратору: здания добавляются в Admin-панели."
        />
      ) : null}

      <Row gutter={[16, 16]} data-testid="buildings-grid">
        {buildings.map((building) => (
          <Col key={building.id} xs={24} md={12} xl={8}>
            <Card
              hoverable
              className="building-card"
              data-testid={`building-card-${building.id}`}
              onClick={() => handlePick(building)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') handlePick(building);
              }}
              tabIndex={0}
              role="button"
              aria-label={`Открыть заявки здания ${building.name}`}
            >
              <Flex vertical gap={12}>
                <Flex align="center" gap={8}>
                  <BankOutlined style={{ fontSize: 20, color: '#1677ff' }} />
                  <Typography.Text strong style={{ fontSize: 17 }}>
                    {building.name}
                  </Typography.Text>
                </Flex>
                {building.address ? (
                  <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                    <EnvironmentOutlined /> {building.address}
                  </Typography.Text>
                ) : null}
                <Counters building={building} />
              </Flex>
            </Card>
          </Col>
        ))}
      </Row>

      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        Завершённые заявки в счётчиках не учитываются. Данные зданий проверяются на сервере при
        каждом запросе.
      </Typography.Text>
    </Flex>
  );
}
