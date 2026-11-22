/**
 * Состояние страницы `/equipment` в адресной строке (Ф4).
 *
 * Требование PLAN Ф4: фильтры (здание, склад, тип), поиск и страница живут
 * в query-параметрах — ссылку можно переслать, а F5 не сбрасывает выбор.
 * Значения по умолчанию в URL не пишутся, чтобы адрес оставался коротким.
 *
 * Модуль намеренно без React: правила разбора/сборки проверяются unit-тестами.
 */

/** Размер страницы по умолчанию (EQ-7: серверная пагинация, `size` ≤ 100). */
export const EQUIPMENT_PAGE_SIZE_DEFAULT = 20;

/** Варианты размера страницы в таблице. */
export const EQUIPMENT_PAGE_SIZE_OPTIONS = [10, 20, 50, 100];

export interface EquipmentUrlState {
  buildingId: number | null;
  warehouseId: number | null;
  typeId: number | null;
  search: string;
  page: number;
  size: number;
}

export const EMPTY_EQUIPMENT_URL_STATE: EquipmentUrlState = {
  buildingId: null,
  warehouseId: null,
  typeId: null,
  search: '',
  page: 1,
  size: EQUIPMENT_PAGE_SIZE_DEFAULT,
};

export interface EquipmentSearchPatch {
  buildingId?: number | null;
  warehouseId?: number | null;
  typeId?: number | null;
  search?: string;
  page?: number;
  size?: number;
}

function readPositiveInteger(raw: string | null | undefined): number | null {
  if (!raw) return null;
  const parsed = Number(raw);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
}

/** Query-строка → состояние страницы; мусор в адресе не ломает экран. */
export function parseEquipmentSearch(search: string): EquipmentUrlState {
  const params = new URLSearchParams(search);
  const size = readPositiveInteger(params.get('size')) ?? EQUIPMENT_PAGE_SIZE_DEFAULT;
  return {
    buildingId: readPositiveInteger(params.get('building')),
    warehouseId: readPositiveInteger(params.get('warehouse')),
    typeId: readPositiveInteger(params.get('type')),
    search: (params.get('q') ?? '').trim(),
    page: readPositiveInteger(params.get('page')) ?? 1,
    size: Math.min(size, 100),
  };
}

/** Состояние → query-параметры (значения по умолчанию опускаются). */
export function buildEquipmentSearch(state: EquipmentUrlState): URLSearchParams {
  const params = new URLSearchParams();
  if (state.buildingId !== null) params.set('building', String(state.buildingId));
  if (state.warehouseId !== null) params.set('warehouse', String(state.warehouseId));
  if (state.typeId !== null) params.set('type', String(state.typeId));
  if (state.search) params.set('q', state.search);
  if (state.page > 1) params.set('page', String(state.page));
  if (state.size !== EQUIPMENT_PAGE_SIZE_DEFAULT) params.set('size', String(state.size));
  return params;
}

/**
 * Изменение фильтров: любой фильтр (и размер страницы) возвращает на первую
 * страницу, иначе можно остаться на несуществующей странице после сужения выборки.
 */
export function patchEquipmentSearch(
  state: EquipmentUrlState,
  patch: EquipmentSearchPatch,
): EquipmentUrlState {
  const next: EquipmentUrlState = {
    buildingId: patch.buildingId !== undefined ? patch.buildingId : state.buildingId,
    warehouseId: patch.warehouseId !== undefined ? patch.warehouseId : state.warehouseId,
    typeId: patch.typeId !== undefined ? patch.typeId : state.typeId,
    search: patch.search !== undefined ? patch.search.trim() : state.search,
    page: patch.page ?? state.page,
    size: patch.size ?? state.size,
  };

  const filtersChanged =
    next.buildingId !== state.buildingId ||
    next.warehouseId !== state.warehouseId ||
    next.typeId !== state.typeId ||
    next.search !== state.search;
  const sizeChanged = next.size !== state.size;

  return {
    ...next,
    page: patch.page ?? (filtersChanged || sizeChanged ? 1 : state.page),
  };
}

/** Выбор здания в дереве/фильтре сбрасывает склад: он принадлежит другому зданию. */
export function selectBuilding(
  state: EquipmentUrlState,
  buildingId: number | null,
): EquipmentUrlState {
  return patchEquipmentSearch(state, { buildingId, warehouseId: null });
}

/** Выбор склада одновременно фиксирует его здание — навигация «здание → склад». */
export function selectWarehouse(
  state: EquipmentUrlState,
  warehouseId: number | null,
  buildingId: number | null,
): EquipmentUrlState {
  return patchEquipmentSearch(state, { buildingId, warehouseId });
}

/** Сброс фильтров при сохранении выбранного размера страницы. */
export function resetEquipmentFilters(state: EquipmentUrlState): EquipmentUrlState {
  return { ...EMPTY_EQUIPMENT_URL_STATE, size: state.size };
}
