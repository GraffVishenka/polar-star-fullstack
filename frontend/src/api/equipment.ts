/**
 * Оборудование, здания и склады (§3.6).
 * Чтение доступно всем авторизованным; запись зданий/складов — только ADMIN (EQ-6).
 */
import { apiRequest } from './client';
import type {
  Building,
  BuildingCreateRequest,
  BuildingUpdateRequest,
  Equipment,
  EquipmentBulkMoveRequest,
  EquipmentBulkMoveResponse,
  EquipmentCreateRequest,
  EquipmentListParams,
  EquipmentMoveRequest,
  EquipmentMovement,
  EquipmentUpdateRequest,
  Page,
  Warehouse,
  WarehouseCreateRequest,
  WarehouseUpdateRequest,
} from './types';

// --- Здания ---------------------------------------------------------------

export function fetchBuildings(params: { is_active?: boolean } = {}): Promise<Building[]> {
  return apiRequest<Building[]>('/buildings', { query: params });
}

export function createBuilding(body: BuildingCreateRequest): Promise<Building> {
  return apiRequest<Building>('/buildings', { method: 'POST', body });
}

/** EQ-6: деактивация вместо удаления используется для зданий «в работе». */
export function updateBuilding(id: number, body: BuildingUpdateRequest): Promise<Building> {
  return apiRequest<Building>(`/buildings/${id}`, { method: 'PATCH', body });
}

export function deleteBuilding(id: number): Promise<void> {
  return apiRequest<void>(`/buildings/${id}`, { method: 'DELETE' });
}

// --- Склады/кабинеты ------------------------------------------------------

export function fetchWarehouses(params: { building_id?: number } = {}): Promise<Warehouse[]> {
  return apiRequest<Warehouse[]>('/warehouses', { query: params });
}

export function createWarehouse(body: WarehouseCreateRequest): Promise<Warehouse> {
  return apiRequest<Warehouse>('/warehouses', { method: 'POST', body });
}

export function updateWarehouse(id: number, body: WarehouseUpdateRequest): Promise<Warehouse> {
  return apiRequest<Warehouse>(`/warehouses/${id}`, { method: 'PATCH', body });
}

export function deleteWarehouse(id: number): Promise<void> {
  return apiRequest<void>(`/warehouses/${id}`, { method: 'DELETE' });
}

// --- Единицы оборудования -------------------------------------------------

export function fetchEquipment(params: EquipmentListParams = {}): Promise<Page<Equipment>> {
  return apiRequest<Page<Equipment>>('/equipment', { query: params });
}

export function fetchEquipmentItem(id: number): Promise<Equipment> {
  return apiRequest<Equipment>(`/equipment/${id}`);
}

export function createEquipment(body: EquipmentCreateRequest): Promise<Equipment> {
  return apiRequest<Equipment>('/equipment', { method: 'POST', body });
}

export function updateEquipment(id: number, body: EquipmentUpdateRequest): Promise<Equipment> {
  return apiRequest<Equipment>(`/equipment/${id}`, { method: 'PATCH', body });
}

/** EQ-3: одиночное перемещение; тот же склад → 409 SAME_WAREHOUSE. */
export function moveEquipment(id: number, body: EquipmentMoveRequest): Promise<Equipment> {
  return apiRequest<Equipment>(`/equipment/${id}/move`, { method: 'POST', body });
}

/** EQ-4: массовое перемещение 1..200 единиц в одной транзакции. */
export function bulkMoveEquipment(
  body: EquipmentBulkMoveRequest,
): Promise<EquipmentBulkMoveResponse> {
  return apiRequest<EquipmentBulkMoveResponse>('/equipment/bulk-move', { method: 'POST', body });
}

/** EQ-5: журнал перемещений, только чтение. */
export function fetchEquipmentMovements(
  id: number,
  params: { page?: number; size?: number } = {},
): Promise<Page<EquipmentMovement>> {
  return apiRequest<Page<EquipmentMovement>>(`/equipment/${id}/movements`, { query: params });
}

export function deleteEquipment(id: number): Promise<void> {
  return apiRequest<void>(`/equipment/${id}`, { method: 'DELETE' });
}

/** EQ-8: путь ведомости помещения в Excel (скачивание — через apiDownload). */
export function warehouseInventoryPath(warehouseId: number): string {
  return `/warehouses/${warehouseId}/inventory.xlsx`;
}
