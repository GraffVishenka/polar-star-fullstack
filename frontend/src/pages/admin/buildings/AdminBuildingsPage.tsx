/**
 * `/admin/buildings` — здания и склады (Ф7, §3.6 EQ-1/EQ-6, только ADMIN).
 *
 * Экран разделён на две части: таблица зданий (название, адрес, число складов,
 * переключатель активности) и помещения выбранного здания (CRUD).
 *
 * Бизнес-правила, которые проверяет сервер, а UI объясняет:
 *  - деактивация (`is_active=false`) убирает здание из публичного выбора и форм,
 *    история/заявки/оборудование сохраняются (EQ-6);
 *  - физическое удаление — только для неиспользуемых зданий, иначе
 *    `409 BUILDING_IN_USE` с причинами в `details.reasons`;
 *  - имя помещения уникально в здании, непустой склад не удаляется
 *    (`409 WAREHOUSE_NOT_EMPTY`) — данные не теряются.
 */
import { useCallback, useMemo, useState } from 'react';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { Alert, App, Button, Col, Flex, Row, Skeleton, Typography } from 'antd';
import type { Building, Warehouse } from '@/api/types';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { PageHeader } from '@/components/PageHeader';
import { BuildingsTable } from './components/BuildingsTable';
import { BuildingFormModal } from './components/BuildingFormModal';
import { WarehouseFormModal } from './components/WarehouseFormModal';
import { WarehousesPanel } from './components/WarehousesPanel';
import { describeInfraError, type InfraErrorText } from '../errors';
import {
  countWarehousesByBuilding,
  useAdminBuildings,
  useAdminWarehouses,
  useDeleteBuilding,
  useDeleteWarehouse,
  useInvalidateAdminInfra,
  useUpdateBuilding,
} from './useBuildingsAdmin';

/** Тело модального окна с причиной отказа сервера (список из `details.reasons`). */
function InfraErrorContent({ text }: { text: InfraErrorText }) {
  return (
    <div data-testid="infra-error-content">
      <Typography.Paragraph style={{ marginBottom: text.reasons.length ? 8 : 0 }}>
        {text.message}
      </Typography.Paragraph>
      {text.reasons.length ? (
        <ul style={{ margin: 0, paddingInlineStart: 20 }} data-testid="infra-error-reasons">
          {text.reasons.map((reason) => (
            <li key={reason}>{reason}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function AdminBuildingsPage() {
  const { modal, message } = App.useApp();

  const buildingsQuery = useAdminBuildings();
  const warehousesQuery = useAdminWarehouses();
  const invalidate = useInvalidateAdminInfra();

  const updateBuildingMutation = useUpdateBuilding();
  const deleteBuildingMutation = useDeleteBuilding();
  const deleteWarehouseMutation = useDeleteWarehouse();

  const buildings = useMemo(() => buildingsQuery.data ?? [], [buildingsQuery.data]);
  const warehouses = useMemo(() => warehousesQuery.data ?? [], [warehousesQuery.data]);
  const warehouseCounts = useMemo(() => countWarehousesByBuilding(warehouses), [warehouses]);

  const [selectedBuildingId, setSelectedBuildingId] = useState<number | null>(null);
  const effectiveSelectedId = selectedBuildingId ?? buildings[0]?.id ?? null;
  const selectedBuilding = buildings.find((item) => item.id === effectiveSelectedId) ?? null;
  const selectedWarehouses = useMemo(
    () => warehouses.filter((warehouse) => warehouse.building_id === effectiveSelectedId),
    [effectiveSelectedId, warehouses],
  );

  const [buildingForm, setBuildingForm] = useState<{ open: boolean; building: Building | null }>({
    open: false,
    building: null,
  });
  const [warehouseForm, setWarehouseForm] = useState<{
    open: boolean;
    warehouse: Warehouse | null;
  }>({ open: false, warehouse: null });
  const [deleteBuildingTarget, setDeleteBuildingTarget] = useState<Building | null>(null);
  const [deleteWarehouseTarget, setDeleteWarehouseTarget] = useState<Warehouse | null>(null);
  const [deactivateTarget, setDeactivateTarget] = useState<Building | null>(null);

  const showError = useCallback(
    (cause: unknown) => {
      const text = describeInfraError(cause);
      modal.error({
        title: text.title,
        content: <InfraErrorContent text={text} />,
        okText: 'Понятно',
      });
    },
    [modal],
  );

  const setBuildingActive = useCallback(
    async (building: Building, next: boolean) => {
      try {
        await updateBuildingMutation.mutateAsync({ id: building.id, body: { is_active: next } });
        invalidate();
        message.success(
          next
            ? `Здание «${building.name}» активировано`
            : `Здание «${building.name}» деактивировано — оно скрыто из публичного выбора`,
        );
      } catch (cause) {
        showError(cause);
      }
    },
    [invalidate, message, showError, updateBuildingMutation],
  );

  // Включение безопасно и делается сразу, выключение — после подтверждения (EQ-6).
  const handleToggleActive = useCallback(
    (building: Building, next: boolean) => {
      if (next) {
        void setBuildingActive(building, true);
        return;
      }
      setDeactivateTarget(building);
    },
    [setBuildingActive],
  );

  const confirmDeactivate = useCallback(async () => {
    if (!deactivateTarget) return;
    await setBuildingActive(deactivateTarget, false);
    setDeactivateTarget(null);
  }, [deactivateTarget, setBuildingActive]);

  const confirmDeleteBuilding = useCallback(async () => {
    if (!deleteBuildingTarget) return;
    const target = deleteBuildingTarget;
    try {
      await deleteBuildingMutation.mutateAsync(target.id);
      invalidate();
      message.success(`Здание «${target.name}» удалено`);
      if (effectiveSelectedId === target.id) setSelectedBuildingId(null);
    } catch (cause) {
      showError(cause);
    } finally {
      setDeleteBuildingTarget(null);
    }
  }, [
    deleteBuildingMutation,
    deleteBuildingTarget,
    effectiveSelectedId,
    invalidate,
    message,
    showError,
  ]);

  const confirmDeleteWarehouse = useCallback(async () => {
    if (!deleteWarehouseTarget) return;
    const target = deleteWarehouseTarget;
    try {
      await deleteWarehouseMutation.mutateAsync(target.id);
      invalidate();
      message.success(`Помещение «${target.name}» удалено`);
    } catch (cause) {
      showError(cause);
    } finally {
      setDeleteWarehouseTarget(null);
    }
  }, [deleteWarehouseMutation, deleteWarehouseTarget, invalidate, message, showError]);

  const loading = buildingsQuery.isLoading || warehousesQuery.isLoading;

  return (
    <Flex vertical gap={16} data-testid="admin-buildings-page">
      <PageHeader
        title="Здания и склады"
        description="Деактивация скрывает здание из публичного выбора и форм, но сохраняет историю (EQ-6). Физическое удаление — только для неиспользуемых зданий."
        extra={
          <Flex gap={8} wrap>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setBuildingForm({ open: true, building: null })}
              data-testid="building-add"
            >
              Добавить здание
            </Button>
            <Button
              icon={<ReloadOutlined />}
              loading={buildingsQuery.isFetching || warehousesQuery.isFetching}
              onClick={() => {
                void buildingsQuery.refetch();
                void warehousesQuery.refetch();
              }}
              data-testid="buildings-refresh"
            >
              Обновить
            </Button>
          </Flex>
        }
      />

      <Alert
        type="info"
        showIcon
        message="Правила удаления и деактивации"
        description="Удалить можно только здание без складов, оборудования, сотрудников и заявок. Если здание используется, сервер вернёт причину — деактивируйте его. Помещение с оборудованием удалить нельзя: сначала переместите единицы (EQ-6)."
        data-testid="buildings-rules"
      />

      {buildingsQuery.isError ? (
        <Alert
          type="error"
          showIcon
          message="Не удалось загрузить здания"
          action={
            <Button size="small" onClick={() => void buildingsQuery.refetch()}>
              Повторить
            </Button>
          }
        />
      ) : loading ? (
        <Skeleton active paragraph={{ rows: 6 }} />
      ) : (
        <Row gutter={[16, 16]}>
          <Col xs={24} xl={14}>
            <BuildingsTable
              buildings={buildings}
              warehouseCounts={warehouseCounts}
              loading={buildingsQuery.isFetching}
              selectedBuildingId={effectiveSelectedId}
              onSelect={(building) => setSelectedBuildingId(building.id)}
              onEdit={(building) => setBuildingForm({ open: true, building })}
              onDelete={setDeleteBuildingTarget}
              onToggleActive={handleToggleActive}
            />
          </Col>
          <Col xs={24} xl={10}>
            <WarehousesPanel
              building={selectedBuilding}
              warehouses={selectedWarehouses}
              loading={warehousesQuery.isFetching}
              onAdd={() => setWarehouseForm({ open: true, warehouse: null })}
              onEdit={(warehouse) => setWarehouseForm({ open: true, warehouse })}
              onDelete={setDeleteWarehouseTarget}
            />
          </Col>
        </Row>
      )}

      <BuildingFormModal
        open={buildingForm.open}
        building={buildingForm.building}
        onClose={() => setBuildingForm({ open: false, building: null })}
        onSaved={(saved) => {
          invalidate();
          message.success(
            buildingForm.building
              ? `Сохранено: «${saved.name}»`
              : `Добавлено здание «${saved.name}»`,
          );
          setSelectedBuildingId(saved.id);
        }}
      />

      <WarehouseFormModal
        open={warehouseForm.open}
        buildingId={effectiveSelectedId}
        buildingName={selectedBuilding?.name ?? null}
        warehouse={warehouseForm.warehouse}
        onClose={() => setWarehouseForm({ open: false, warehouse: null })}
        onSaved={(saved) => {
          invalidate();
          message.success(
            warehouseForm.warehouse
              ? `Помещение переименовано: «${saved.name}»`
              : `Добавлено помещение «${saved.name}»`,
          );
        }}
      />

      <ConfirmDialog
        open={deactivateTarget !== null}
        title={`Деактивировать здание «${deactivateTarget?.name ?? ''}»?`}
        description="Здание исчезнет из публичного выбора, формы заявки и выбора для сотрудников. История, заявки и оборудование сохранятся (EQ-6)."
        okText="Деактивировать"
        danger
        loading={updateBuildingMutation.isPending}
        onCancel={() => setDeactivateTarget(null)}
        onConfirm={confirmDeactivate}
      />

      <ConfirmDialog
        open={deleteBuildingTarget !== null}
        title={`Удалить здание «${deleteBuildingTarget?.name ?? ''}»?`}
        description="Действие необратимо. Если в здании есть склады, оборудование, сотрудники или заявки, сервер вернёт причину отказа."
        warning="Используемое здание удалить нельзя — деактивируйте его (EQ-6)."
        okText="Удалить здание"
        danger
        loading={deleteBuildingMutation.isPending}
        onCancel={() => setDeleteBuildingTarget(null)}
        onConfirm={confirmDeleteBuilding}
      />

      <ConfirmDialog
        open={deleteWarehouseTarget !== null}
        title={`Удалить помещение «${deleteWarehouseTarget?.name ?? ''}»?`}
        description="Удалить можно только пустое помещение: оборудование должно быть перемещено."
        warning="Если в помещении есть оборудование, данные сохранятся, а сервер вернёт 409 WAREHOUSE_NOT_EMPTY."
        okText="Удалить помещение"
        danger
        loading={deleteWarehouseMutation.isPending}
        onCancel={() => setDeleteWarehouseTarget(null)}
        onConfirm={confirmDeleteWarehouse}
      />
    </Flex>
  );
}
