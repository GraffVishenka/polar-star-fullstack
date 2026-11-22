/**
 * Панель фильтров `/equipment` (Ф4): здание, склад, тип и поиск.
 *
 * Любое изменение фильтра уходит в URL (см. `equipmentSearchParams`), поэтому
 * фильтры переживают F5 и передаются ссылкой. Неактивные здания остаются
 * в списке с бейджем «Неактивно» (EQ-1), но недоступны как цель перемещения
 * у USER — это проверяется в диалоге перемещения и на сервере (EQ-3).
 */
import { useCallback, useEffect, useState } from 'react';
import { ReloadOutlined } from '@ant-design/icons';
import { Button, Card, Col, Flex, Input, Row, Select, Typography } from 'antd';
import type { Building, EquipmentType, Warehouse } from '@/api/types';
import type { EquipmentSearchPatch, EquipmentUrlState } from '../equipmentSearchParams';
import { EntityName } from './InactiveTag';

export interface EquipmentFiltersProps {
  state: EquipmentUrlState;
  buildings: Building[];
  warehouses: Warehouse[];
  types: EquipmentType[];
  onChange: (patch: EquipmentSearchPatch) => void;
  onReset: () => void;
  loading?: boolean;
}

export function EquipmentFilters({
  state,
  buildings,
  warehouses,
  types,
  onChange,
  onReset,
  loading = false,
}: EquipmentFiltersProps) {
  // Черновик поиска: запрос уходит по Enter/кнопке, чтобы не дёргать сервер на каждый символ.
  const [searchDraft, setSearchDraft] = useState(state.search);

  useEffect(() => {
    setSearchDraft(state.search);
  }, [state.search]);

  const visibleWarehouses =
    state.buildingId === null
      ? warehouses
      : warehouses.filter((warehouse) => warehouse.building_id === state.buildingId);

  const buildingById = new Map(buildings.map((building) => [building.id, building]));
  const hasFilters =
    state.buildingId !== null ||
    state.warehouseId !== null ||
    state.typeId !== null ||
    !!state.search;

  const handleSearchChange = useCallback(
    (value: string) => {
      setSearchDraft(value);
      // Очистка крестиком сразу снимает фильтр, не дожидаясь Enter.
      if (!value.trim() && state.search) onChange({ search: '' });
    },
    [onChange, state.search],
  );

  return (
    <Card size="small" data-testid="equipment-filters">
      <Row gutter={[8, 12]} align="bottom">
        <Col xs={24} sm={12} lg={8} xl={6}>
          <Flex vertical gap={4}>
            <Typography.Text style={{ fontSize: 12 }} type="secondary">
              Здание
            </Typography.Text>
            <Select
              data-testid="filter-building"
              aria-label="Фильтр по зданию"
              value={state.buildingId === null ? 'all' : String(state.buildingId)}
              options={[
                { value: 'all', label: 'Все здания' },
                ...buildings.map((building) => ({
                  value: String(building.id),
                  label: <EntityName name={building.name} inactive={!building.is_active} />,
                })),
              ]}
              loading={loading}
              onChange={(value: string) =>
                onChange({ buildingId: value === 'all' ? null : Number(value), warehouseId: null })
              }
            />
          </Flex>
        </Col>

        <Col xs={24} sm={12} lg={8} xl={6}>
          <Flex vertical gap={4}>
            <Typography.Text style={{ fontSize: 12 }} type="secondary">
              Склад / кабинет
            </Typography.Text>
            <Select
              data-testid="filter-warehouse"
              aria-label="Фильтр по складу"
              value={state.warehouseId === null ? 'all' : String(state.warehouseId)}
              options={[
                { value: 'all', label: 'Все помещения' },
                ...visibleWarehouses.map((warehouse) => {
                  const building = buildingById.get(warehouse.building_id);
                  return {
                    value: String(warehouse.id),
                    label: (
                      <EntityName
                        name={
                          state.buildingId === null
                            ? `${warehouse.name} · ${warehouse.building_name}`
                            : warehouse.name
                        }
                        inactive={building?.is_active === false}
                      />
                    ),
                  };
                }),
              ]}
              loading={loading}
              onChange={(value: string) =>
                onChange({ warehouseId: value === 'all' ? null : Number(value) })
              }
            />
          </Flex>
        </Col>

        <Col xs={24} sm={12} lg={8} xl={5}>
          <Flex vertical gap={4}>
            <Typography.Text style={{ fontSize: 12 }} type="secondary">
              Тип оборудования
            </Typography.Text>
            <Select
              data-testid="filter-type"
              aria-label="Фильтр по типу оборудования"
              value={state.typeId === null ? 'all' : String(state.typeId)}
              options={[
                { value: 'all', label: 'Все типы' },
                ...types.map((type) => ({ value: String(type.id), label: type.name })),
              ]}
              loading={loading}
              onChange={(value: string) =>
                onChange({ typeId: value === 'all' ? null : Number(value) })
              }
            />
          </Flex>
        </Col>

        <Col xs={24} sm={12} lg={8} xl={5}>
          <Flex vertical gap={4}>
            <Typography.Text style={{ fontSize: 12 }} type="secondary">
              Поиск
            </Typography.Text>
            <Input.Search
              data-testid="filter-search"
              aria-label="Поиск по названию, серийному или регистрационному номеру"
              placeholder="Название, серийный или рег. номер"
              allowClear
              value={searchDraft}
              loading={loading}
              onChange={(event) => handleSearchChange(event.target.value)}
              onSearch={(value) => onChange({ search: value })}
            />
          </Flex>
        </Col>

        <Col xs={24} xl={24}>
          <Flex align="center" justify="space-between" gap={8} wrap>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              Поиск идёт по названию, серийному и регистрационному номеру; фильтры сохраняются в
              адресе страницы.
            </Typography.Text>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              onClick={onReset}
              disabled={!hasFilters}
              data-testid="filters-reset"
            >
              Сбросить фильтры
            </Button>
          </Flex>
        </Col>
      </Row>
    </Card>
  );
}
