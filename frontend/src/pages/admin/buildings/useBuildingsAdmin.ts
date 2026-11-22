/**
 * Данные админ-раздела «Здания и склады» (§3.6 EQ-1, EQ-6).
 *
 * Чтение доступно всем авторизованным, запись — только ADMIN. Ключи кэша общие
 * (`queryKeys.buildings` / `queryKeys.warehouses`), поэтому после правки здания
 * обновляется и раздел «Оборудование», и публичный выбор зданий.
 */
import { useCallback } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  createBuilding,
  createWarehouse,
  deleteBuilding,
  deleteWarehouse,
  fetchBuildings,
  fetchWarehouses,
  updateBuilding,
  updateWarehouse,
} from '@/api/equipment';
import type {
  Building,
  BuildingCreateRequest,
  BuildingUpdateRequest,
  Warehouse,
  WarehouseCreateRequest,
  WarehouseUpdateRequest,
} from '@/api/types';
import { queryKeys } from '@/app/queryClient';

export function useAdminBuildings() {
  return useQuery({
    queryKey: queryKeys.buildings,
    queryFn: () => fetchBuildings(),
  });
}

export function useAdminWarehouses() {
  return useQuery({
    queryKey: queryKeys.warehouses(),
    queryFn: () => fetchWarehouses(),
  });
}

/**
 * Инвалидация инфраструктуры.
 *
 * EQ-6: деактивация здания должна сразу убрать его из публичного выбора
 * и из форм заявки/сотрудника, поэтому чистим и `public/buildings`.
 */
export function useInvalidateAdminInfra() {
  const client = useQueryClient();
  return useCallback(() => {
    void client.invalidateQueries({ queryKey: queryKeys.buildings });
    void client.invalidateQueries({ queryKey: ['warehouses'] });
    void client.invalidateQueries({ queryKey: queryKeys.publicBuildings });
    // Заявки и оборудование ссылаются на здания/склады в названиях.
    void client.invalidateQueries({ queryKey: ['tickets'] });
    void client.invalidateQueries({ queryKey: ['equipment'] });
  }, [client]);
}

export function useCreateBuilding() {
  return useMutation({
    mutationFn: (body: BuildingCreateRequest) => createBuilding(body),
  });
}

export function useUpdateBuilding() {
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: BuildingUpdateRequest }) =>
      updateBuilding(id, body),
  });
}

export function useDeleteBuilding() {
  return useMutation({
    mutationFn: (id: number) => deleteBuilding(id),
  });
}

export function useCreateWarehouse() {
  return useMutation({
    mutationFn: (body: WarehouseCreateRequest) => createWarehouse(body),
  });
}

export function useUpdateWarehouse() {
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: WarehouseUpdateRequest }) =>
      updateWarehouse(id, body),
  });
}

export function useDeleteWarehouse() {
  return useMutation({
    mutationFn: (id: number) => deleteWarehouse(id),
  });
}

/** Число складов по зданиям — контракт не отдаёт счётчик в `Building` (EQ-1). */
export function countWarehousesByBuilding(warehouses: Warehouse[]): Map<number, number> {
  const counts = new Map<number, number>();
  warehouses.forEach((warehouse) => {
    counts.set(warehouse.building_id, (counts.get(warehouse.building_id) ?? 0) + 1);
  });
  return counts;
}

/** Название здания для заголовков и подтверждений. */
export function buildingTitle(building: Building | null): string {
  return building ? `«${building.name}»` : 'здание';
}
