/**
 * Таблица оборудования (Ф4, EQ-7).
 *
 * Серверная пагинация: `page`/`size` приходят из URL, `total` — из ответа API.
 * Чекбоксы нужны для массового перемещения (EQ-4): лимит 200 единиц проверяет
 * страница, чтобы пользователь получил подсказку до запроса.
 */
import { useEffect, useState } from 'react';
import { Button, Flex, Table, Typography } from 'antd';
import type { CheckboxProps } from 'antd/es/checkbox';
import type { ColumnsType } from 'antd/es/table';
import type { Equipment } from '@/api/types';
import { EmptyState } from '@/components/EmptyState';
import { EQUIPMENT_PAGE_SIZE_OPTIONS } from '../equipmentSearchParams';
import { EntityName } from './InactiveTag';

/**
 * Узкий экран (< 576 px, NFR-3): компактная пагинация без «Всего единиц»
 * и выбора размера страницы, чтобы таблица не выезжала за 375 px.
 * В jsdom/SSR `innerWidth` большой — тесты видят обычный вариант.
 */
function useNarrowScreen(): boolean {
  const [narrow, setNarrow] = useState(
    () => typeof window !== 'undefined' && window.innerWidth < 576,
  );
  useEffect(() => {
    const onResize = () => setNarrow(window.innerWidth < 576);
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, []);
  return narrow;
}

export interface EquipmentTableProps {
  items: Equipment[];
  total: number;
  page: number;
  size: number;
  loading?: boolean;
  selectedIds: number[];
  onSelectionChange: (ids: number[]) => void;
  onOpen: (item: Equipment) => void;
  onPageChange: (page: number, size: number) => void;
}

export function EquipmentTable({
  items,
  total,
  page,
  size,
  loading = false,
  selectedIds,
  onSelectionChange,
  onOpen,
  onPageChange,
}: EquipmentTableProps) {
  const narrow = useNarrowScreen();

  const columns: ColumnsType<Equipment> = [
    {
      title: 'Наименование',
      dataIndex: 'name',
      key: 'name',
      render: (_value, record) => (
        <Flex vertical gap={2}>
          <Typography.Text strong>{record.name}</Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {record.equipment_type_name}
          </Typography.Text>
        </Flex>
      ),
    },
    {
      title: 'Серийный номер',
      dataIndex: 'serial_number',
      key: 'serial_number',
      responsive: ['md'],
      render: (value: string) => <Typography.Text code>{value}</Typography.Text>,
    },
    {
      title: 'Рег. номер',
      dataIndex: 'registration_number',
      key: 'registration_number',
      render: (value: string) => <Typography.Text code>{value}</Typography.Text>,
    },
    {
      title: 'Здание / помещение',
      dataIndex: 'building_name',
      key: 'location',
      render: (_value, record) => (
        <Flex vertical gap={2}>
          <EntityName name={record.building_name} inactive={!record.is_building_active} />
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {record.warehouse_name}
          </Typography.Text>
        </Flex>
      ),
    },
    {
      title: '',
      key: 'actions',
      width: 110,
      align: 'right',
      render: (_value, record) => (
        <Button
          type="link"
          size="small"
          onClick={() => onOpen(record)}
          data-testid={`open-equipment-${record.id}`}
        >
          Открыть
        </Button>
      ),
    },
  ];

  return (
    <div data-testid="equipment-table">
      <Table<Equipment>
        rowKey="id"
        size="small"
        columns={columns}
        dataSource={items}
        loading={loading}
        scroll={{ x: 720 }}
        rowSelection={{
          selectedRowKeys: selectedIds,
          preserveSelectedRowKeys: true,
          onChange: (keys) => onSelectionChange(keys.map(Number)),
          columnWidth: 40,
          // a11y: чекбокс строки должен иметь доступное имя (не только «Выбрать все»).
          getCheckboxProps: (record) =>
            ({
              'aria-label': `Выбрать оборудование «${record.name}», ${record.registration_number}`,
            }) as unknown as Partial<CheckboxProps>,
        }}
        locale={{
          emptyText: (
            <EmptyState
              title="Оборудование не найдено"
              description="Измените фильтры или добавьте новую единицу оборудования."
            />
          ),
        }}
        pagination={{
          current: page,
          pageSize: size,
          total,
          // На 375 px — компактная пагинация «1 / 5» без длинных подписей.
          simple: narrow,
          showSizeChanger: !narrow,
          pageSizeOptions: EQUIPMENT_PAGE_SIZE_OPTIONS,
          showTotal: narrow ? undefined : (count) => `Всего единиц: ${count}`,
          onChange: onPageChange,
        }}
      />
    </div>
  );
}
