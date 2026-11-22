/**
 * Поля выбора местоположения для диалогов перемещения и формы оборудования.
 *
 * Роль влияет на список зданий (EQ-3): USER не может выбрать неактивное здание
 * целью перемещения, ADMIN — может (нужно вернуть оборудование в архив).
 * Неактивные здания в списке ADMIN помечены бейджем «Неактивно» (EQ-1).
 */
import { Form, Select } from 'antd';
import type { Building, Warehouse } from '@/api/types';
import { EntityName } from './InactiveTag';

export interface LocationFieldsProps {
  buildings: Building[];
  warehouses: Warehouse[];
  isAdmin: boolean;
  buildingField?: string;
  warehouseField?: string;
  /** Подсказка под полем склада. */
  warehouseExtra?: string;
}

export function LocationFields({
  buildings,
  warehouses,
  isAdmin,
  buildingField = 'building_id',
  warehouseField = 'to_warehouse_id',
  warehouseExtra,
}: LocationFieldsProps) {
  const form = Form.useFormInstance();
  const buildingId = Form.useWatch(buildingField, form) as number | undefined;

  const selectableBuildings = buildings.filter((building) => isAdmin || building.is_active);
  const buildingOptions = selectableBuildings.map((building) => ({
    value: building.id,
    label: <EntityName name={building.name} inactive={!building.is_active} />,
  }));

  const warehouseOptions = warehouses
    .filter((warehouse) => buildingId === undefined || warehouse.building_id === buildingId)
    .map((warehouse) => {
      const building = buildings.find((item) => item.id === warehouse.building_id);
      return {
        value: warehouse.id,
        label: <EntityName name={warehouse.name} inactive={building?.is_active === false} />,
      };
    });

  return (
    <>
      <Form.Item
        label="Здание"
        name={buildingField}
        rules={[{ required: true, message: 'Выберите здание' }]}
        extra={
          isAdmin
            ? undefined
            : 'Неактивные здания недоступны: перемещение в них выполняет только администратор (EQ-3)'
        }
      >
        <Select
          placeholder="Здание назначения"
          options={buildingOptions}
          data-testid="move-building"
          onChange={() => form.setFieldValue(warehouseField, undefined)}
        />
      </Form.Item>
      <Form.Item
        label="Склад / кабинет"
        name={warehouseField}
        rules={[{ required: true, message: 'Выберите помещение' }]}
        extra={warehouseExtra}
      >
        <Select
          placeholder={
            buildingId === undefined ? 'Сначала выберите здание' : 'Помещение назначения'
          }
          options={warehouseOptions}
          data-testid="move-warehouse"
        />
      </Form.Item>
    </>
  );
}
