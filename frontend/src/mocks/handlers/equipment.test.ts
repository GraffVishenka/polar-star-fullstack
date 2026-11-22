/**
 * Правила MSW-мока оборудования (§3.6 EQ-2..EQ-5), которые проверяет UI.
 *
 * Мок обязан вести себя как сервер: 422 BUILDING_INACTIVE для USER, 409
 * SAME_WAREHOUSE, 404 с откатом в массовом перемещении и запись в журнал
 * по каждой перемещённой единице.
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { setAccessToken } from '@/api/client';
import {
  bulkMoveEquipment,
  createEquipment,
  fetchEquipmentItem,
  fetchEquipmentMovements,
  moveEquipment,
} from '@/api/equipment';
import { issueAccessToken } from '@/mocks/auth';

describe('MSW: перемещение оборудования (EQ-3)', () => {
  beforeEach(() => setAccessToken(issueAccessToken(2)));

  it('USER получает 422 BUILDING_INACTIVE при перемещении в неактивное здание', async () => {
    await expect(moveEquipment(1, { to_warehouse_id: 5 })).rejects.toMatchObject({
      code: 'BUILDING_INACTIVE',
      status: 422,
    });
    // Данные не изменились.
    const item = await fetchEquipmentItem(1);
    expect(item.warehouse_id).toBe(2);
  });

  it('ADMIN может переместить оборудование в неактивное здание и получает запись в журнале', async () => {
    setAccessToken(issueAccessToken(1));

    const updated = await moveEquipment(1, { to_warehouse_id: 5, comment: 'В архив' });
    expect(updated.building_id).toBe(4);
    expect(updated.is_building_active).toBe(false);

    const journal = await fetchEquipmentMovements(1);
    expect(journal.items[0]).toMatchObject({
      from_warehouse_id: 2,
      to_warehouse_id: 5,
      comment: 'В архив',
      to_building_name: 'Старый корпус',
    });
  });

  it('перемещение в тот же склад → 409 SAME_WAREHOUSE', async () => {
    await expect(moveEquipment(1, { to_warehouse_id: 2 })).rejects.toMatchObject({
      code: 'SAME_WAREHOUSE',
      status: 409,
    });
  });

  it('дубликат регистрационного номера при создании → 409 DUPLICATE_REGISTRATION_NUMBER', async () => {
    await expect(
      createEquipment({
        name: 'Сканер',
        equipment_type_id: 1,
        serial_number: 'SC-1',
        registration_number: 'inv-1001',
        warehouse_id: 1,
      }),
    ).rejects.toMatchObject({ code: 'DUPLICATE_REGISTRATION_NUMBER', status: 409 });
  });
});

describe('MSW: массовое перемещение (EQ-4)', () => {
  beforeEach(() => setAccessToken(issueAccessToken(2)));

  it('создаёт запись журнала на каждую единицу и возвращает итог', async () => {
    const result = await bulkMoveEquipment({
      equipment_ids: [1, 2],
      to_warehouse_id: 1,
      comment: 'Инвентаризация',
    });

    expect(result).toMatchObject({
      moved_count: 2,
      to_warehouse_id: 1,
      to_warehouse_name: 'Кабинет 101',
      movements_created: 2,
    });

    const first = await fetchEquipmentMovements(1);
    const second = await fetchEquipmentMovements(2);
    expect(first.items[0]?.comment).toBe('Инвентаризация');
    expect(second.items[0]?.comment).toBe('Инвентаризация');
  });

  it('несуществующий id → 404 и полный откат (ни одна единица не перемещена)', async () => {
    await expect(
      bulkMoveEquipment({ equipment_ids: [1, 999], to_warehouse_id: 1 }),
    ).rejects.toMatchObject({ code: 'NOT_FOUND', status: 404 });

    const item = await fetchEquipmentItem(1);
    expect(item.warehouse_id).toBe(2);
  });

  it('больше 200 единиц → 422 VALIDATION_ERROR', async () => {
    const ids = Array.from({ length: 201 }, (_value, index) => index + 1);
    await expect(
      bulkMoveEquipment({ equipment_ids: ids, to_warehouse_id: 1 }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 422 });
  });

  it('USER не может массово переместить оборудование в неактивное здание', async () => {
    await expect(
      bulkMoveEquipment({ equipment_ids: [1, 2], to_warehouse_id: 5 }),
    ).rejects.toMatchObject({ code: 'BUILDING_INACTIVE', status: 422 });

    const item = await fetchEquipmentItem(1);
    expect(item.warehouse_id).toBe(2);
  });
});
