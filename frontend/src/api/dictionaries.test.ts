/**
 * Синхронизация контракта с `openapi/openapi.yaml` (Б7/Ф11):
 * публичные справочники/настройки, `include_inactive` для ADMIN, `employees[]`
 * сводной статистики и новые поля корзины.
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { HttpResponse, http } from 'msw';
import { db } from '@/mocks/db';
import { server } from '@/mocks/server';
import { authenticateAs } from '@/test/utils';
import { fetchTrash } from './admin';
import {
  fetchEquipmentTypesForSelection,
  fetchProblemTypesForSelection,
  fetchPublicSettings,
} from './dictionaries';
import { fetchPublicProblemTypes } from './public';
import { fetchAdminStats } from './stats';

describe('справочники и публичные настройки по контракту', () => {
  beforeEach(() => {
    authenticateAs(2);
  });

  it('GET /api/public/problem-types отдаёт только активные id/name/sort_order', async () => {
    const types = await fetchPublicProblemTypes();

    expect(types.length).toBe(db.problemTypes.filter((item) => item.is_active).length);
    expect(types.some((item) => item.name.includes('Устаревший'))).toBe(false);
    // Контракт PublicProblemType: без is_active и служебных полей.
    expect(Object.keys(types[0]!).sort()).toEqual(['id', 'name', 'sort_order']);
    expect(types.map((item) => item.sort_order)).toEqual(
      [...types.map((item) => item.sort_order)].sort((a, b) => a - b),
    );
  });

  it('GET /api/public/settings содержит только app_name и статусы (PUB-1)', async () => {
    const settings = await fetchPublicSettings();

    expect(Object.keys(settings).sort()).toEqual(['app_name', 'statuses']);
    expect(settings.statuses).toHaveLength(4);
    expect(settings.statuses.map((status) => status.code)).toEqual([
      'new',
      'in_progress',
      'deferred',
      'done',
    ]);
    expect(settings).not.toHaveProperty('timezone');
    expect(settings).not.toHaveProperty('workdays');
    expect(settings).not.toHaveProperty('deadline_reminder_time');
  });

  it('include_inactive раскрывает деактивированные значения только ADMIN', async () => {
    const userVisible = await fetchProblemTypesForSelection({ include_inactive: true });
    expect(userVisible.some((item) => item.is_active === false)).toBe(false);

    authenticateAs(1);
    const adminVisible = await fetchProblemTypesForSelection({ include_inactive: true });
    expect(adminVisible.length).toBeGreaterThan(userVisible.length);
    expect(adminVisible.some((item) => item.is_active === false)).toBe(true);

    // Без флага ADMIN видит только активные элементы.
    const adminActive = await fetchProblemTypesForSelection();
    expect(adminActive.some((item) => item.is_active === false)).toBe(false);
  });

  it('GET /api/equipment-types отдаёт активные типы, ADMIN — и неактивные', async () => {
    const active = await fetchEquipmentTypesForSelection();
    expect(active.some((item) => item.is_active === false)).toBe(false);
    expect(active.some((item) => item.name === 'Принтер')).toBe(true);

    authenticateAs(1);
    const all = await fetchEquipmentTypesForSelection({ include_inactive: true });
    expect(all.some((item) => item.is_active === false)).toBe(true);
  });

  it('GET /api/admin/stats без user_id отдаёт employees[] по сотрудникам', async () => {
    authenticateAs(1);
    const aggregated = await fetchAdminStats({});

    expect(aggregated.employees).toBeDefined();
    expect(aggregated.employees!.length).toBeGreaterThan(0);
    expect(aggregated.employees![0]).toEqual(
      expect.objectContaining({
        user_id: expect.any(Number),
        user_name: expect.any(String),
        done_in_period: expect.any(Number),
      }),
    );

    // С user_id — одна запись без списка сотрудников.
    const single = await fetchAdminStats({ user_id: 2 });
    expect(single.employees).toBeUndefined();
  });

  it('GET /api/admin/trash отдаёт удалившего и здание элемента (TRASH-1)', async () => {
    authenticateAs(1);
    const page = await fetchTrash({ type: 'ticket' });

    expect(page.items.length).toBeGreaterThan(0);
    const item = page.items[0]!;
    expect(item).toHaveProperty('deleted_by_user_id');
    expect(item).toHaveProperty('building_id');
    expect(item).toHaveProperty('building_name');
    expect(typeof item.deleted_by_name === 'string' || item.deleted_by_name === null).toBe(true);
  });
});

describe('переход на фолбэк при недоступном API', () => {
  it('публичные настройки не ломаются от 500 (проверка на уровне запроса)', async () => {
    server.use(
      http.get('*/api/public/settings', () =>
        HttpResponse.json({ code: 'INTERNAL_ERROR' }, { status: 500 }),
      ),
    );
    await expect(fetchPublicSettings()).rejects.toMatchObject({ status: 500 });
  });
});
