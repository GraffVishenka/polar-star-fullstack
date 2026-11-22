/**
 * Карточка оборудования в Drawer (Ф4).
 *
 * Показывает поля единицы (EQ-2), текущее местоположение (здание/помещение) и
 * журнал перемещений таймлайном (EQ-5): кто, когда, откуда/куда, комментарий.
 * Неактивное здание в истории помечается бейджем «Неактивно» (EQ-1).
 *
 * После перемещения данные обновляются инвалидацией запросов — без перезагрузки.
 */
import { useEffect, useMemo, useState } from 'react';
import {
  ArrowRightOutlined,
  DeleteOutlined,
  EditOutlined,
  FileExcelOutlined,
  SwapOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Flex,
  Skeleton,
  Timeline,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import type { Building, Equipment } from '@/api/types';
import { EmptyState } from '@/components/EmptyState';
import {
  MOVEMENTS_MAX_SIZE,
  MOVEMENTS_PAGE_SIZE,
  useEquipmentItem,
  useEquipmentMovements,
} from '../useEquipmentData';
import { EntityName } from './InactiveTag';

export interface EquipmentDetailsDrawerProps {
  equipmentId: number | null;
  buildings: Building[];
  isAdmin: boolean;
  onClose: () => void;
  onMove: (equipment: Equipment) => void;
  onEdit: (equipment: Equipment) => void;
  onDelete: (equipment: Equipment) => void;
}

export function formatDateTime(value: string): string {
  const parsed = dayjs(value);
  return parsed.isValid() ? parsed.format('DD.MM.YYYY HH:mm') : value;
}

export function EquipmentDetailsDrawer({
  equipmentId,
  buildings,
  isAdmin,
  onClose,
  onMove,
  onEdit,
  onDelete,
}: EquipmentDetailsDrawerProps) {
  const [journalLimit, setJournalLimit] = useState(MOVEMENTS_PAGE_SIZE);
  // При открытии другой единицы лента журнала снова начинается с первой страницы (EQ-5).
  useEffect(() => {
    setJournalLimit(MOVEMENTS_PAGE_SIZE);
  }, [equipmentId]);

  const itemQuery = useEquipmentItem(equipmentId);
  const movementsQuery = useEquipmentMovements(equipmentId, journalLimit);
  const item = itemQuery.data;

  const buildingByName = useMemo(
    () => new Map(buildings.map((building) => [building.name, building])),
    [buildings],
  );

  const movements = movementsQuery.data?.items ?? [];
  const movementsTotal = movementsQuery.data?.total ?? 0;

  const timelineItems = movements.map((movement) => ({
    color: 'blue',
    children: (
      <Flex vertical gap={2} data-testid={`movement-item-${movement.id}`}>
        <Typography.Text strong>
          {formatDateTime(movement.moved_at)} · {movement.moved_by_name}
        </Typography.Text>
        <Flex align="center" gap={4} wrap>
          <EntityName
            name={`«${movement.from_warehouse_name}», ${movement.from_building_name}`}
            inactive={buildingByName.get(movement.from_building_name)?.is_active === false}
            compact
          />
          <ArrowRightOutlined style={{ fontSize: 11, color: '#8c8c8c' }} />
          <EntityName
            name={`«${movement.to_warehouse_name}», ${movement.to_building_name}`}
            inactive={buildingByName.get(movement.to_building_name)?.is_active === false}
            compact
          />
        </Flex>
        {movement.comment ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            Комментарий: {movement.comment}
          </Typography.Text>
        ) : null}
      </Flex>
    ),
  }));

  return (
    <Drawer
      open={equipmentId !== null}
      width={640}
      onClose={onClose}
      title={item ? `${item.name}` : 'Карточка оборудования'}
      extra={
        item ? (
          <Flex gap={8} wrap>
            <Button
              size="small"
              icon={<EditOutlined />}
              onClick={() => onEdit(item)}
              data-testid="edit-equipment"
            >
              Изменить
            </Button>
            {isAdmin ? (
              <Button
                size="small"
                danger
                icon={<DeleteOutlined />}
                onClick={() => onDelete(item)}
                data-testid="delete-equipment"
              >
                Удалить
              </Button>
            ) : null}
          </Flex>
        ) : null
      }
    >
      <div data-testid="equipment-drawer">
        {itemQuery.isLoading ? <Skeleton active paragraph={{ rows: 6 }} /> : null}

        {itemQuery.isError ? (
          <Alert
            type="error"
            showIcon
            message="Не удалось загрузить карточку оборудования"
            description="Повторите попытку: возможно, единица была удалена."
            action={
              <Button size="small" onClick={() => void itemQuery.refetch()}>
                Повторить
              </Button>
            }
          />
        ) : null}

        {item ? (
          <Flex vertical gap={16}>
            <div data-testid="equipment-fields">
              <Descriptions
                size="small"
                column={1}
                bordered
                items={[
                  { key: 'name', label: 'Наименование', children: item.name },
                  { key: 'type', label: 'Тип', children: item.equipment_type_name },
                  {
                    key: 'serial',
                    label: 'Серийный номер',
                    children: <Typography.Text code>{item.serial_number}</Typography.Text>,
                  },
                  {
                    key: 'registration',
                    label: 'Регистрационный номер',
                    children: <Typography.Text code>{item.registration_number}</Typography.Text>,
                  },
                  {
                    key: 'created',
                    label: 'Добавлено',
                    children: formatDateTime(item.created_at),
                  },
                ]}
              />
            </div>

            <Card
              size="small"
              title="Текущее местоположение"
              data-testid="equipment-location"
              extra={
                <Flex gap={8} wrap>
                  <Button
                    type="primary"
                    size="small"
                    icon={<SwapOutlined />}
                    onClick={() => onMove(item)}
                    data-testid="open-move-dialog"
                  >
                    Переместить
                  </Button>
                  {/* EQ-8: ведомость помещения формируется в блоке Ф8 — здесь честная заглушка. */}
                  <Tooltip title="Ведомость помещения появится в блоке Ф8 (Excel)">
                    <Button
                      size="small"
                      icon={<FileExcelOutlined />}
                      disabled
                      data-testid="inventory-stub"
                    >
                      Ведомость помещения
                    </Button>
                  </Tooltip>
                </Flex>
              }
            >
              <Flex vertical gap={4}>
                <Typography.Text>
                  Здание:{' '}
                  <Typography.Text strong>
                    <EntityName name={item.building_name} inactive={!item.is_building_active} />
                  </Typography.Text>
                </Typography.Text>
                <Typography.Text>
                  Помещение: <Typography.Text strong>{item.warehouse_name}</Typography.Text>
                </Typography.Text>
              </Flex>
            </Card>

            <Card
              size="small"
              title="Журнал перемещений"
              data-testid="equipment-journal"
              extra={
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  записей: {movementsTotal}
                </Typography.Text>
              }
            >
              {movementsQuery.isLoading ? <Skeleton active paragraph={{ rows: 3 }} /> : null}

              {!movementsQuery.isLoading && movements.length === 0 ? (
                <EmptyState
                  compact
                  title="Перемещений не было"
                  description="Журнал появится после первого перемещения (EQ-5)."
                />
              ) : null}

              {movements.length > 0 ? (
                <div data-testid="movement-list">
                  <Timeline items={timelineItems} />
                </div>
              ) : null}

              {movementsTotal > movements.length ? (
                <Button
                  size="small"
                  block
                  onClick={() => setJournalLimit(MOVEMENTS_MAX_SIZE)}
                  data-testid="movements-more"
                >
                  Показать ещё
                </Button>
              ) : null}
            </Card>
          </Flex>
        ) : null}
      </div>
    </Drawer>
  );
}
