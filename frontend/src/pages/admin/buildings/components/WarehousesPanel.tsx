/**
 * Склады/кабинеты выбранного здания (EQ-1, EQ-6, только ADMIN).
 *
 * Удаление непустого помещения запрещено сервером (`409 WAREHOUSE_NOT_EMPTY`) —
 * страница показывает причину и не теряет данные: оборудование остаётся на месте.
 */
import { Button, Card, Flex, Table, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import type { Building, Warehouse } from '@/api/types';
import { EmptyState } from '@/components/EmptyState';

export interface WarehousesPanelProps {
  building: Building | null;
  warehouses: Warehouse[];
  loading?: boolean;
  onAdd: () => void;
  onEdit: (warehouse: Warehouse) => void;
  onDelete: (warehouse: Warehouse) => void;
}

export function WarehousesPanel({
  building,
  warehouses,
  loading = false,
  onAdd,
  onEdit,
  onDelete,
}: WarehousesPanelProps) {
  const columns: ColumnsType<Warehouse> = [
    {
      title: 'Помещение',
      dataIndex: 'name',
      key: 'name',
      render: (_value, record) => (
        <Flex vertical gap={2}>
          <Typography.Text strong>{record.name}</Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {record.building_name} · #{record.id}
          </Typography.Text>
        </Flex>
      ),
    },
    {
      title: '',
      key: 'actions',
      width: 120,
      align: 'right',
      render: (_value, record) => (
        <Flex gap={4} justify="flex-end">
          <Button
            size="small"
            type="text"
            icon={<EditOutlined />}
            aria-label={`Переименовать помещение ${record.name}`}
            onClick={() => onEdit(record)}
            data-testid={`warehouse-edit-${record.id}`}
          />
          <Button
            size="small"
            type="text"
            danger
            icon={<DeleteOutlined />}
            aria-label={`Удалить помещение ${record.name}`}
            onClick={() => onDelete(record)}
            data-testid={`warehouse-delete-${record.id}`}
          />
        </Flex>
      ),
    },
  ];

  return (
    <Card
      size="small"
      title={building ? `Помещения здания «${building.name}»` : 'Помещения'}
      extra={
        <Button
          type="primary"
          size="small"
          icon={<PlusOutlined />}
          disabled={!building}
          onClick={onAdd}
          data-testid="warehouse-add"
        >
          Добавить помещение
        </Button>
      }
      data-testid="warehouses-panel"
    >
      {building ? (
        <Table<Warehouse>
          rowKey="id"
          size="small"
          columns={columns}
          dataSource={warehouses}
          loading={loading}
          pagination={false}
          locale={{
            emptyText: (
              <EmptyState
                compact
                title="В здании нет помещений"
                description="Добавьте склад или кабинет, чтобы закрепить за ним оборудование."
              />
            ),
          }}
        />
      ) : (
        <EmptyState
          compact
          title="Выберите здание"
          description="Нажмите на число складов в таблице зданий, чтобы управлять помещениями."
        />
      )}
    </Card>
  );
}
