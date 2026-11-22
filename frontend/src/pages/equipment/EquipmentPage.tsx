/**
 * `/equipment` — оборудование организации (блок Ф4, §3.6).
 *
 * Экран состоит из навигации «здание → склад/кабинет → оборудование», панели
 * фильтров и таблицы с серверной пагинацией. Фильтры и страница живут в URL,
 * поэтому ссылка воспроизводит выборку, а F5 ничего не сбрасывает (Ф4).
 *
 * Права (§3.1): читать, создавать, править и перемещать (в т.ч. массово) может
 * любой авторизованный; удалять — только ADMIN (в корзину, TRASH-*).
 */
import { useCallback, useMemo, useState, type ReactNode } from 'react';
import { PlusOutlined, ReloadOutlined, SwapOutlined } from '@ant-design/icons';
import { Alert, App, Button, Col, Flex, Row, Skeleton, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { useSearchParams } from 'react-router-dom';
import { deleteEquipment } from '@/api/equipment';
import type { Equipment, EquipmentBulkMoveResponse } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { PageHeader } from '@/components/PageHeader';
import { BulkMoveDialog } from './components/BulkMoveDialog';
import { EquipmentDetailsDrawer } from './components/EquipmentDetailsDrawer';
import { EquipmentFilters } from './components/EquipmentFilters';
import { EquipmentFormModal } from './components/EquipmentFormModal';
import { EquipmentTable } from './components/EquipmentTable';
import { EquipmentTreeNav, type EquipmentLocationSelection } from './components/EquipmentTreeNav';
import { EntityName } from './components/InactiveTag';
import { MoveEquipmentDialog } from './components/MoveEquipmentDialog';
import {
  buildEquipmentSearch,
  parseEquipmentSearch,
  patchEquipmentSearch,
  resetEquipmentFilters,
  selectBuilding,
  selectWarehouse,
  type EquipmentSearchPatch,
} from './equipmentSearchParams';
import { describeEquipmentError } from './errors';
import {
  useBuildings,
  useEquipmentList,
  useEquipmentTypes,
  useInvalidateEquipment,
  useWarehouses,
} from './useEquipmentData';

/** EQ-4: за одну операцию можно переместить не больше 200 единиц. */
const BULK_LIMIT = 200;

export function EquipmentPage() {
  const { isAdmin } = useAuth();
  const { message } = App.useApp();

  const [searchParams, setSearchParams] = useSearchParams();
  const state = useMemo(() => parseEquipmentSearch(searchParams.toString()), [searchParams]);

  const buildingsQuery = useBuildings();
  const warehousesQuery = useWarehouses();
  const typesQuery = useEquipmentTypes(isAdmin);
  const listQuery = useEquipmentList(state);
  const invalidate = useInvalidateEquipment();

  const buildings = useMemo(() => buildingsQuery.data ?? [], [buildingsQuery.data]);
  const warehouses = useMemo(() => warehousesQuery.data ?? [], [warehousesQuery.data]);
  const types = typesQuery.data ?? [];

  const items = listQuery.data?.items ?? [];
  const total = listQuery.data?.total ?? 0;

  const [selectedIds, setSelectedIds] = useState<number[]>([]);
  const [detailsId, setDetailsId] = useState<number | null>(null);
  const [moveTarget, setMoveTarget] = useState<Equipment | null>(null);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [formState, setFormState] = useState<{ open: boolean; equipment: Equipment | null }>({
    open: false,
    equipment: null,
  });
  const [deleteTarget, setDeleteTarget] = useState<Equipment | null>(null);

  const currentBuilding = buildings.find((building) => building.id === state.buildingId);
  const currentWarehouse = warehouses.find((warehouse) => warehouse.id === state.warehouseId);

  const applyPatch = useCallback(
    (patch: EquipmentSearchPatch) => {
      setSearchParams(buildEquipmentSearch(patchEquipmentSearch(state, patch)));
    },
    [setSearchParams, state],
  );

  const handleTreeSelect = useCallback(
    (selection: EquipmentLocationSelection) => {
      const next =
        selection.warehouseId !== null
          ? selectWarehouse(state, selection.warehouseId, selection.buildingId)
          : selectBuilding(state, selection.buildingId);
      setSearchParams(buildEquipmentSearch(next));
    },
    [setSearchParams, state],
  );

  const handleResetFilters = useCallback(() => {
    setSearchParams(buildEquipmentSearch(resetEquipmentFilters(state)));
  }, [setSearchParams, state]);

  const handleSelectionChange = useCallback(
    (ids: number[]) => {
      if (ids.length > BULK_LIMIT) {
        message.warning(`За одну операцию можно выбрать не больше ${BULK_LIMIT} единиц (EQ-4)`);
        return;
      }
      setSelectedIds(ids);
    },
    [message],
  );

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteEquipment(id),
    onSuccess: (_result, id) => {
      invalidate();
      if (detailsId === id) setDetailsId(null);
      message.success('Оборудование перемещено в корзину');
    },
    onError: (cause) => message.error(describeEquipmentError(cause, 'delete')),
  });

  const handleMoved = useCallback(
    (updated: Equipment) => {
      invalidate();
      message.success(`Оборудование перемещено в «${updated.warehouse_name}»`);
    },
    [invalidate, message],
  );

  const handleBulkCompleted = useCallback(
    (result: EquipmentBulkMoveResponse) => {
      invalidate();
      message.success(`Перемещено единиц: ${result.moved_count} → «${result.to_warehouse_name}»`);
    },
    [invalidate, message],
  );

  // Выбор сбрасываем при закрытии диалога: на шаге «итог» счётчик ещё нужен.
  const closeBulkDialog = useCallback(() => {
    setBulkOpen(false);
    setSelectedIds([]);
  }, []);

  const handleSaved = useCallback(
    (saved: Equipment) => {
      invalidate();
      message.success(
        formState.equipment
          ? `Сохранено: ${saved.name}`
          : `Добавлено: ${saved.name} (${saved.registration_number})`,
      );
    },
    [formState.equipment, invalidate, message],
  );

  const breadcrumbs = useMemo(() => {
    const crumbs: Array<{ title: ReactNode; href?: string }> = [
      { title: 'Оборудование', href: '/equipment' },
    ];
    if (currentBuilding) {
      crumbs.push({
        title: (
          <EntityName name={currentBuilding.name} inactive={!currentBuilding.is_active} compact />
        ),
      });
    }
    if (currentWarehouse) crumbs.push({ title: currentWarehouse.name });
    return crumbs;
  }, [currentBuilding, currentWarehouse]);

  return (
    <Flex vertical gap={16} data-testid="equipment-page">
      <PageHeader
        title="Оборудование"
        description="Все авторизованные видят оборудование всех зданий (EQ-7): так возможны перемещения между зданиями."
        breadcrumbs={breadcrumbs}
        extra={
          <Flex gap={8} wrap>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setFormState({ open: true, equipment: null })}
              data-testid="create-equipment"
            >
              Добавить оборудование
            </Button>
            <Button
              icon={<ReloadOutlined />}
              loading={listQuery.isFetching}
              onClick={() => void listQuery.refetch()}
              data-testid="refresh-equipment"
            >
              Обновить
            </Button>
          </Flex>
        }
      />

      {currentBuilding && !currentBuilding.is_active ? (
        <Alert
          type="warning"
          showIcon
          message={`Здание «${currentBuilding.name}» деактивировано`}
          description={
            isAdmin
              ? 'ADMIN может перемещать оборудование в неактивное здание: история сохраняется (EQ-3, EQ-6).'
              : 'Здание остаётся в истории и фильтрах, но переместить оборудование внутрь может только администратор (EQ-3).'
          }
          data-testid="inactive-building-alert"
        />
      ) : null}

      <Row gutter={[16, 16]}>
        <Col xs={24} lg={8} xl={7}>
          <EquipmentTreeNav
            buildings={buildings}
            warehouses={warehouses}
            buildingId={state.buildingId}
            warehouseId={state.warehouseId}
            onSelect={handleTreeSelect}
            loading={buildingsQuery.isLoading || warehousesQuery.isLoading}
          />
        </Col>

        <Col xs={24} lg={16} xl={17}>
          <Flex vertical gap={12}>
            <EquipmentFilters
              state={state}
              buildings={buildings}
              warehouses={warehouses}
              types={types}
              onChange={applyPatch}
              onReset={handleResetFilters}
              loading={buildingsQuery.isLoading || warehousesQuery.isLoading}
            />

            {selectedIds.length > 0 ? (
              <Flex align="center" justify="space-between" gap={8} wrap data-testid="selection-bar">
                <Typography.Text>
                  Выбрано единиц: <Typography.Text strong>{selectedIds.length}</Typography.Text>
                </Typography.Text>
                <Flex gap={8} wrap>
                  <Button
                    type="primary"
                    icon={<SwapOutlined />}
                    onClick={() => setBulkOpen(true)}
                    data-testid="bulk-move-open"
                  >
                    Переместить выбранные
                  </Button>
                  <Button onClick={() => setSelectedIds([])} data-testid="selection-clear">
                    Сбросить выбор
                  </Button>
                </Flex>
              </Flex>
            ) : null}

            {listQuery.isError ? (
              <Alert
                type="error"
                showIcon
                message="Не удалось загрузить список оборудования"
                description="Проверьте соединение и повторите попытку."
                action={
                  <Button size="small" onClick={() => void listQuery.refetch()}>
                    Повторить
                  </Button>
                }
                data-testid="equipment-list-error"
              />
            ) : listQuery.isLoading ? (
              <Skeleton active paragraph={{ rows: 6 }} />
            ) : (
              <EquipmentTable
                items={items}
                total={total}
                page={state.page}
                size={state.size}
                loading={listQuery.isFetching}
                selectedIds={selectedIds}
                onSelectionChange={handleSelectionChange}
                onOpen={(item) => setDetailsId(item.id)}
                onPageChange={(page, size) => applyPatch({ page, size })}
              />
            )}
          </Flex>
        </Col>
      </Row>

      <EquipmentDetailsDrawer
        equipmentId={detailsId}
        buildings={buildings}
        isAdmin={isAdmin}
        onClose={() => setDetailsId(null)}
        onMove={(item) => setMoveTarget(item)}
        onEdit={(item) => setFormState({ open: true, equipment: item })}
        onDelete={(item) => setDeleteTarget(item)}
      />

      <MoveEquipmentDialog
        open={moveTarget !== null}
        equipment={moveTarget}
        buildings={buildings}
        warehouses={warehouses}
        isAdmin={isAdmin}
        onClose={() => setMoveTarget(null)}
        onMoved={handleMoved}
      />

      <BulkMoveDialog
        open={bulkOpen}
        equipmentIds={selectedIds}
        buildings={buildings}
        warehouses={warehouses}
        isAdmin={isAdmin}
        onClose={closeBulkDialog}
        onCompleted={handleBulkCompleted}
      />

      <EquipmentFormModal
        open={formState.open}
        equipment={formState.equipment}
        buildings={buildings}
        warehouses={warehouses}
        types={types}
        isAdmin={isAdmin}
        onClose={() => {
          setFormState({ open: false, equipment: null });
          // Страховка от частичного успеха (обновление + перемещение): обновляем список.
          invalidate();
        }}
        onSaved={handleSaved}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Удалить оборудование?"
        description={
          deleteTarget
            ? `«${deleteTarget.name}» (${deleteTarget.registration_number}) будет перемещено в корзину.`
            : undefined
        }
        warning="Восстановить единицу можно из корзины (TRASH-2)."
        okText="В корзину"
        danger
        loading={deleteMutation.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={async () => {
          if (!deleteTarget) return;
          try {
            await deleteMutation.mutateAsync(deleteTarget.id);
          } finally {
            setDeleteTarget(null);
          }
        }}
      />
    </Flex>
  );
}
