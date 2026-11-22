/**
 * Правила адресной строки `/equipment` (Ф4): фильтры и страница в URL.
 */
import { describe, expect, it } from 'vitest';
import {
  EMPTY_EQUIPMENT_URL_STATE,
  EQUIPMENT_PAGE_SIZE_DEFAULT,
  buildEquipmentSearch,
  parseEquipmentSearch,
  patchEquipmentSearch,
  resetEquipmentFilters,
  selectBuilding,
  selectWarehouse,
} from './equipmentSearchParams';

describe('equipmentSearchParams', () => {
  it('пустая строка даёт состояние по умолчанию', () => {
    expect(parseEquipmentSearch('')).toEqual(EMPTY_EQUIPMENT_URL_STATE);
  });

  it('разбирает фильтры, поиск и страницу, игнорируя мусор и ограничивая size', () => {
    const state = parseEquipmentSearch(
      '?building=2&warehouse=3&type=1&q=%20INV-1001%20&page=2&size=500',
    );

    expect(state).toEqual({
      buildingId: 2,
      warehouseId: 3,
      typeId: 1,
      search: 'INV-1001',
      page: 2,
      size: 100,
    });
    expect(parseEquipmentSearch('?building=abc&page=-3')).toEqual(EMPTY_EQUIPMENT_URL_STATE);
  });

  it('не пишет в URL значения по умолчанию', () => {
    const params = buildEquipmentSearch({
      ...EMPTY_EQUIPMENT_URL_STATE,
      buildingId: 1,
      search: 'INV',
    });
    expect(params.toString()).toBe('building=1&q=INV');

    const empty = buildEquipmentSearch(EMPTY_EQUIPMENT_URL_STATE);
    expect(empty.toString()).toBe('');
    expect(
      parseEquipmentSearch(
        buildEquipmentSearch({ ...EMPTY_EQUIPMENT_URL_STATE, size: 50 }).toString(),
      ).size,
    ).toBe(50);
  });

  it('смена фильтра сбрасывает страницу, смена только страницы — нет', () => {
    const onPage2 = { ...EMPTY_EQUIPMENT_URL_STATE, page: 2 };

    expect(patchEquipmentSearch(onPage2, { typeId: 3 }).page).toBe(1);
    expect(patchEquipmentSearch(onPage2, { search: 'Dell' }).page).toBe(1);
    expect(patchEquipmentSearch(onPage2, { page: 3 }).page).toBe(3);
    // Смена размера страницы тоже возвращает на первую страницу.
    expect(patchEquipmentSearch(onPage2, { size: 50 }).page).toBe(1);
    expect(patchEquipmentSearch(onPage2, { page: 1 }).page).toBe(1);
  });

  it('навигация «здание → склад» согласована: выбор здания сбрасывает склад', () => {
    const withWarehouse = selectWarehouse(EMPTY_EQUIPMENT_URL_STATE, 3, 2);
    expect(withWarehouse).toMatchObject({ buildingId: 2, warehouseId: 3, page: 1 });

    expect(selectBuilding(withWarehouse, 1)).toMatchObject({ buildingId: 1, warehouseId: null });
    expect(selectBuilding(withWarehouse, null)).toMatchObject({
      buildingId: null,
      warehouseId: null,
    });
  });

  it('сброс фильтров сохраняет выбранный размер страницы', () => {
    const state = {
      ...EMPTY_EQUIPMENT_URL_STATE,
      buildingId: 1,
      warehouseId: 2,
      page: 3,
      size: 50,
    };
    expect(resetEquipmentFilters(state)).toEqual({
      ...EMPTY_EQUIPMENT_URL_STATE,
      size: 50,
    });
    expect(EQUIPMENT_PAGE_SIZE_DEFAULT).toBe(20);
  });
});
