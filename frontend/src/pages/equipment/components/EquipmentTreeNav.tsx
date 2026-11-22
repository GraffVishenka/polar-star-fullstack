/**
 * Навигация «здание → склад/кабинет → оборудование» (Ф4, EQ-1).
 *
 * Дерево показывает все здания, включая неактивные (EQ-1: они остаются в фильтрах
 * и истории), с бейджем «Неактивно». Выбор узла меняет фильтры в URL:
 *  - «Все здания» — сбросить здание и склад;
 *  - здание — показать оборудование всех его складов;
 *  - склад — показать оборудование этого помещения.
 */
import { useMemo, useState } from 'react';
import type { Key } from 'react';
import { BankOutlined, InboxOutlined } from '@ant-design/icons';
import { Card, Empty, Flex, Tree, Typography } from 'antd';
import type { DataNode } from 'antd/es/tree';
import type { Building, Warehouse } from '@/api/types';
import { EntityName } from './InactiveTag';

const ALL_KEY = 'all';
const BUILDING_PREFIX = 'building-';
const WAREHOUSE_PREFIX = 'warehouse-';

export interface EquipmentLocationSelection {
  buildingId: number | null;
  warehouseId: number | null;
}

export interface EquipmentTreeNavProps {
  buildings: Building[];
  warehouses: Warehouse[];
  buildingId: number | null;
  warehouseId: number | null;
  onSelect: (selection: EquipmentLocationSelection) => void;
  loading?: boolean;
}

export function EquipmentTreeNav({
  buildings,
  warehouses,
  buildingId,
  warehouseId,
  onSelect,
  loading = false,
}: EquipmentTreeNavProps) {
  // `null` — пользователь ещё не управлял раскрытием: раскрываем все здания.
  const [expandedKeys, setExpandedKeys] = useState<Key[] | null>(null);

  const treeData = useMemo<DataNode[]>(
    () => [
      {
        key: ALL_KEY,
        title: (
          <Flex align="center" gap={6} data-testid="tree-node-all">
            <BankOutlined />
            <span>Все здания</span>
          </Flex>
        ),
        children: buildings.map((building) => ({
          key: `${BUILDING_PREFIX}${building.id}`,
          title: (
            <span data-testid={`tree-building-${building.id}`}>
              <EntityName name={building.name} inactive={!building.is_active} compact />
            </span>
          ),
          children: warehouses
            .filter((warehouse) => warehouse.building_id === building.id)
            .map((warehouse) => ({
              key: `${WAREHOUSE_PREFIX}${warehouse.id}`,
              isLeaf: true,
              title: (
                <Flex align="center" gap={6} data-testid={`tree-warehouse-${warehouse.id}`}>
                  <InboxOutlined />
                  <span>{warehouse.name}</span>
                </Flex>
              ),
            })),
        })),
      },
    ],
    [buildings, warehouses],
  );

  const effectiveExpandedKeys = useMemo<Key[]>(
    () =>
      expandedKeys ?? [ALL_KEY, ...buildings.map((building) => `${BUILDING_PREFIX}${building.id}`)],
    [buildings, expandedKeys],
  );

  const selectedKeys: Key[] =
    warehouseId !== null
      ? [`${WAREHOUSE_PREFIX}${warehouseId}`]
      : buildingId !== null
        ? [`${BUILDING_PREFIX}${buildingId}`]
        : [ALL_KEY];

  const handleSelect = (keys: Key[]) => {
    const key = keys[0];
    if (key === undefined) return; // antd дерево может снять выбор — оставляем как было
    const raw = String(key);

    if (raw === ALL_KEY) {
      onSelect({ buildingId: null, warehouseId: null });
      return;
    }
    if (raw.startsWith(WAREHOUSE_PREFIX)) {
      const id = Number(raw.slice(WAREHOUSE_PREFIX.length));
      const warehouse = warehouses.find((item) => item.id === id);
      const parentKey = `${BUILDING_PREFIX}${warehouse?.building_id ?? 0}`;
      setExpandedKeys((prev) => {
        const current = prev ?? effectiveExpandedKeys;
        return current.includes(parentKey) ? current : [...current, parentKey];
      });
      onSelect({ buildingId: warehouse?.building_id ?? null, warehouseId: id });
      return;
    }
    if (raw.startsWith(BUILDING_PREFIX)) {
      const id = Number(raw.slice(BUILDING_PREFIX.length));
      setExpandedKeys((prev) => {
        const current = prev ?? effectiveExpandedKeys;
        return current.includes(raw) ? current : [...current, raw];
      });
      onSelect({ buildingId: id, warehouseId: null });
    }
  };

  return (
    <Card
      size="small"
      title="Расположение"
      extra={
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          здание → помещение
        </Typography.Text>
      }
      loading={loading}
      data-testid="equipment-tree"
    >
      {buildings.length === 0 && !loading ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Зданий пока нет" />
      ) : (
        // Навигационный landmark: скринридер видит отдельную область «Расположение».
        <nav aria-label="Расположение оборудования">
          <Tree
            blockNode
            treeData={treeData}
            expandedKeys={effectiveExpandedKeys}
            selectedKeys={selectedKeys}
            onExpand={(keys) => setExpandedKeys(keys)}
            onSelect={(keys) => handleSelect(keys)}
          />
        </nav>
      )}
    </Card>
  );
}
