/**
 * Правила MSW-мока корзины (TRASH-1..TRASH-5), которые проверяет админ-страница.
 *
 * Мок ведёт себя как сервер: автоочистка 90 дней (TRASH-4), восстановление
 * заявки в деактивированное здание, узел БЗ только с живым родителем,
 * оборудование с занятым регистрационным номером → 409 (TRASH-2).
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { fetchTrash, purgeTrashItem, restoreTrashItem } from '@/api/admin';
import { fetchKbTree } from '@/api/kb';
import { fetchTicket } from '@/api/tickets';
import { db } from '@/mocks/db';
import { ADMIN_AUTH, authenticateAs } from '@/test/utils';

describe('MSW: корзина', () => {
  beforeEach(() => authenticateAs(ADMIN_AUTH.id));

  it('не показывает элементы старше 90 дней (TRASH-4)', async () => {
    const entry = db.trash.find((item) => item.type === 'equipment');
    expect(entry).toBeDefined();
    entry!.deleted_at = new Date(Date.now() - 100 * 24 * 60 * 60 * 1000).toISOString();

    const page = await fetchTrash({ type: 'equipment' });
    expect(page.items).toHaveLength(0);
    // Сам элемент физически ещё в базе — его уберёт джоба, но список его скрывает.
    expect(db.trash.some((item) => item.type === 'equipment')).toBe(true);
  });

  it('восстанавливает заявку со статусом и зданием (TRASH-2)', async () => {
    await expect(fetchTicket(13)).rejects.toMatchObject({ status: 404 });

    await restoreTrashItem('ticket', 13);

    const restored = await fetchTicket(13);
    expect(restored.id).toBe(13);
    expect(restored.building_id).toBe(4);
    expect(restored.status).toBe('done');
    const trash = await fetchTrash({ type: 'ticket' });
    expect(trash.items.some((item) => item.id === 13)).toBe(false);
  });

  it('не восстанавливает узел БЗ, если родитель удалён (TRASH-2)', async () => {
    db.trash.push({
      type: 'kb',
      id: 99,
      deleted_at: new Date().toISOString(),
      deleted_by_user_id: 1,
      snapshot: { title: 'Сирота', parent_id: 999 },
    });

    await expect(restoreTrashItem('kb', 99)).rejects.toMatchObject({
      code: 'CONFLICT',
      status: 409,
    });
    const tree = await fetchKbTree();
    expect(tree.some((node) => node.id === 99)).toBe(false);
  });

  it('восстанавливает корневой узел БЗ из снимка (TRASH-2)', async () => {
    await restoreTrashItem('kb', 13);

    const tree = await fetchKbTree();
    const restored = tree.find((node) => node.title === 'Архивная инструкция');
    expect(restored).toBeDefined();
    expect(restored?.parent_id).toBeNull();
  });

  it('не восстанавливает оборудование с занятым регистрационным номером (TRASH-2)', async () => {
    await expect(restoreTrashItem('equipment', 10)).rejects.toMatchObject({
      code: 'DUPLICATE_REGISTRATION_NUMBER',
      status: 409,
    });
    const trash = await fetchTrash({ type: 'equipment' });
    expect(trash.items.some((item) => item.id === 10)).toBe(true);
  });

  it('окончательно удаляет заявку и её следы (TRASH-3)', async () => {
    await purgeTrashItem('ticket', 13);

    await expect(fetchTicket(13)).rejects.toMatchObject({ status: 404 });
    expect(db.tickets.some((ticket) => ticket.id === 13)).toBe(false);
    const trash = await fetchTrash({ type: 'ticket' });
    expect(trash.items).toHaveLength(0);
  });
});
