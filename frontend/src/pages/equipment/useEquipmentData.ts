/**
 * Данные раздела «Оборудование» (§3.6): здания, склады, типы, список,
 * карточка и журнал перемещений.
 *
 * Все ограничения (роль, активность здания, дубли рег. номера) проверяет сервер:
 * клиент только читает `code` ошибки и показывает понятный текст.
 */
import { useCallback } from 'react';
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';
import { fetchEquipmentTypes } from '@/api/admin';
import {
  fetchBuildings,
  fetchEquipment,
  fetchEquipmentItem,
  fetchEquipmentMovements,
  fetchWarehouses,
} from '@/api/equipment';
import type { EquipmentListParams, EquipmentType } from '@/api/types';
import { queryKeys } from '@/app/queryClient';
import type { EquipmentUrlState } from './equipmentSearchParams';

/** Сколько записей журнала подгружать за раз (EQ-5: пагинация). */
export const MOVEMENTS_PAGE_SIZE = 20;
/** Максимум записей журнала, который показываем одной лентой. */
export const MOVEMENTS_MAX_SIZE = 100;
/** Сколько строк оборудования читаем для справочника типов у USER. */
const TYPE_OPTIONS_SAMPLE_SIZE = 100;

/**
 * Ключ для вариантов типов у USER.
 *
 * Контракт §3.13 отдаёт справочник типов только ADMIN (`/admin/equipment-types`),
 * поэтому USER собирает варианты из выданных строк оборудования. Ключ отдельный,
 * чтобы инвалидация списка после перемещения не дёргала справочник повторно.
 */
const TYPE_OPTIONS_KEY = ['equipment-type-options'] as const;

export function useBuildings() {
  return useQuery({
    queryKey: queryKeys.buildings,
    queryFn: () => fetchBuildings(),
  });
}

export function useWarehouses() {
  return useQuery({
    queryKey: queryKeys.warehouses(),
    queryFn: () => fetchWarehouses(),
  });
}

/** EQ-7: серверная пагинация и фильтры; предыдущая страница остаётся видимой при загрузке. */
export function useEquipmentList(state: EquipmentUrlState) {
  const params: EquipmentListParams = {
    page: state.page,
    size: state.size,
    building_id: state.buildingId ?? undefined,
    warehouse_id: state.warehouseId ?? undefined,
    equipment_type_id: state.typeId ?? undefined,
    search: state.search || undefined,
  };
  return useQuery({
    queryKey: queryKeys.equipment(params),
    queryFn: () => fetchEquipment(params),
    placeholderData: keepPreviousData,
  });
}

/**
 * Варианты фильтра «тип» и поля формы.
 *
 * ADMIN получает полный справочник из админского эндпоинта; USER — из строк
 * оборудования (публичного справочника типов в контракте нет). Ограничение
 * зафиксировано в отчёте Ф4 для `lead`: при добавлении `GET /api/equipment-types`
 * достаточно поменять реализацию этой функции.
 */
export function useEquipmentTypes(isAdmin: boolean) {
  return useQuery<EquipmentType[]>({
    queryKey: isAdmin ? queryKeys.equipmentTypes : TYPE_OPTIONS_KEY,
    queryFn: async () => {
      if (isAdmin) return fetchEquipmentTypes();
      const page = await fetchEquipment({ page: 1, size: TYPE_OPTIONS_SAMPLE_SIZE });
      const byId = new Map<number, string>();
      page.items.forEach((item) => byId.set(item.equipment_type_id, item.equipment_type_name));
      return [...byId.entries()]
        .map(([id, name]) => ({ id, name }))
        .sort((a, b) => a.name.localeCompare(b.name));
    },
    staleTime: 5 * 60_000,
  });
}

export function useEquipmentItem(id: number | null) {
  return useQuery({
    queryKey: queryKeys.equipmentItem(id ?? 0),
    queryFn: () => fetchEquipmentItem(id as number),
    enabled: id !== null,
  });
}

/** Журнал перемещений: `limit` растёт по кнопке «Показать ещё». */
export function useEquipmentMovements(id: number | null, limit: number) {
  return useQuery({
    queryKey: [...queryKeys.equipmentMovements(id ?? 0), limit],
    queryFn: () =>
      fetchEquipmentMovements(id as number, { page: 1, size: Math.min(limit, MOVEMENTS_MAX_SIZE) }),
    enabled: id !== null,
  });
}

/**
 * Инвалидация всего домена «оборудование» (список, карточка, журнал).
 * После перемещения таблица и журнал обновляются без перезагрузки страницы.
 */
export function useInvalidateEquipment() {
  const client = useQueryClient();
  return useCallback(() => {
    void client.invalidateQueries({ queryKey: ['equipment'] });
  }, [client]);
}
