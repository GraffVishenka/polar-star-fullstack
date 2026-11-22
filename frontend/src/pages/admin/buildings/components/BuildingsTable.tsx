/**
 * Таблица зданий (EQ-1, EQ-6): название, адрес, число складов и переключатель
 * активности. Физическое удаление доступно только для неиспользуемых зданий —
 * решение принимает сервер (`409 BUILDING_IN_USE`), страница показывает причины.
 */
import { Button, Flex, Switch, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DeleteOutlined, EditOutlined, InboxOutlined } from '@ant-design/icons';
import type { Building } from '@/api/types';
import { EmptyState } from '@/components/EmptyState';

export interface BuildingsTableProps {
  buildings: Building[];
  warehouseCounts: Map<number, number>;
  loading?: boolean;
  selectedBuildingId: number | null;
  onSelect: (building: Building) => void;
  onEdit: (building: Building) => void;
  onDelete: (building: Building) => void;
  onToggleActive: (building: Building, nextActive: boolean) => void;
}

export function BuildingsTable({
  buildings,
  warehouseCounts,
  loading = false,
  selectedBuildingId,
  onSelect,
  onEdit,
  onDelete,
  onToggleActive,
}: BuildingsTableProps) {
  const columns: ColumnsType<Building> = [
    {
      title: 'Здание',
      dataIndex: 'name',
      key: 'name',
      render: (_value, record) => (
        <Flex vertical gap={2}>
          <Flex align="center" gap={6} wrap>
            <Typography.Text strong>{record.name}</Typography.Text>
            {record.is_active ? null : (
              <Tag color="default" data-testid={`building-inactive-${record.id}`}>
                Неактивно
              </Tag>
            )}
          </Flex>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            #{record.id}
          </Typography.Text>
        </Flex>
      ),
    },
    {
      title: 'Адрес',
      dataIndex: 'address',
      key: 'address',
      render: (value: string | null) => value ?? '—',
    },
    {
      title: 'Складов',
      key: 'warehouses',
      width: 140,
      render: (_value, record) => (
        <Button
          type="link"
          size="small"
          icon={<InboxOutlined />}
          onClick={() => onSelect(record)}
          data-testid={`building-warehouses-${record.id}`}
        >
          {warehouseCounts.get(record.id) ?? 0}
        </Button>
      ),
    },
    {
      title: 'Активно',
      key: 'is_active',
      width: 130,
      render: (_value, record) => (
        <Switch
          checked={record.is_active}
          checkedChildren="Да"
          unCheckedChildren="Нет"
          aria-label={`Активность здания ${record.name}`}
          data-testid={`building-active-${record.id}`}
          onChange={(checked) => onToggleActive(record, checked)}
        />
      ),
    },
    {
      title: '',
      key: 'actions',
      width: 130,
      align: 'right',
      render: (_value, record) => (
        <Flex gap={4} justify="flex-end">
          <Button
            size="small"
            type="text"
            icon={<EditOutlined />}
            aria-label={`Изменить здание ${record.name}`}
            onClick={() => onEdit(record)}
            data-testid={`building-edit-${record.id}`}
          />
          <Button
            size="small"
            type="text"
            danger
            icon={<DeleteOutlined />}
            aria-label={`Удалить здание ${record.name}`}
            onClick={() => onDelete(record)}
            data-testid={`building-delete-${record.id}`}
          />
        </Flex>
      ),
    },
  ];

  return (
    <div data-testid="buildings-table">
      <Table<Building>
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={buildings}
        loading={loading}
        pagination={false}
        rowClassName={(record) => (record.id === selectedBuildingId ? 'admin-row--selected' : '')}
        locale={{
          emptyText: (
            <EmptyState
              title="Зданий пока нет"
              description="Создайте здание, затем добавьте в него склады и кабинеты."
            />
          ),
        }}
      />
    </div>
  );
}
