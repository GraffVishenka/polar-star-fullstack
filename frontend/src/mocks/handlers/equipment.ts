/** MSW: здания, склады, оборудование, перемещения, ведомость (§3.6, §3.13). */
import { http } from 'msw';
import type {
  BuildingCreateRequest,
  BuildingUpdateRequest,
  EquipmentBulkMoveRequest,
  EquipmentCreateRequest,
  EquipmentMoveRequest,
  EquipmentUpdateRequest,
  WarehouseCreateRequest,
  WarehouseUpdateRequest,
} from '@/api/types';
import { guard } from '../auth';
import {
  db,
  equipmentBulkMoveResult,
  filterEquipment,
  findBuilding,
  findWarehouse,
  nextId,
  toBuilding,
  toEquipment,
  toMovement,
  toWarehouse,
} from '../db';
import type { MockEquipment } from '../db';
import {
  API,
  conflict,
  created,
  fail,
  fileResponse,
  json,
  noContent,
  notFound,
  paginate,
  pathId,
  queryId,
  readBody,
  validation,
} from '../http';

function duplicateRegistrationNumber(registrationNumber: string, exceptId?: number): boolean {
  return db.equipment.some(
    (item) =>
      item.deleted_at === null &&
      item.id !== exceptId &&
      item.registration_number.toLowerCase() === registrationNumber.toLowerCase(),
  );
}

export const equipmentHandlers = [
  // --- Здания -------------------------------------------------------------
  http.get(`${API}/buildings`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const url = new URL(request.url);
    const isActive = url.searchParams.get('is_active');
    const buildings = db.buildings
      .filter((building) => (isActive === null ? true : String(building.is_active) === isActive))
      .map(toBuilding);
    return json(buildings);
  }),

  http.post(`${API}/buildings`, async ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const body = await readBody<BuildingCreateRequest>(request);
    const name = typeof body.name === 'string' ? body.name.trim() : '';
    if (!name) return validation('Укажите название здания', { name: 'обязательное поле' });

    const building = {
      id: nextId(db.buildings),
      name,
      address: typeof body.address === 'string' && body.address.trim() ? body.address.trim() : null,
      is_active: true,
      created_at: new Date().toISOString(),
    };
    db.buildings.push(building);
    return created(toBuilding(building));
  }),

  http.patch(`${API}/buildings/:id`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const building = findBuilding(pathId(params.id) ?? 0);
    if (!building) return notFound('Здание не найдено');

    const body = await readBody<BuildingUpdateRequest>(request);
    if (body.name !== undefined) {
      const name = String(body.name).trim();
      if (!name) return validation('Название не может быть пустым');
      building.name = name;
    }
    if (body.address !== undefined) {
      building.address = body.address === null ? null : String(body.address).trim() || null;
    }
    if (body.is_active !== undefined) building.is_active = Boolean(body.is_active);
    return json(toBuilding(building));
  }),

  http.delete(`${API}/buildings/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const id = pathId(params.id) ?? 0;
    const building = findBuilding(id);
    if (!building) return notFound('Здание не найдено');

    // EQ-6: физическое удаление только для неиспользуемых зданий.
    const hasWarehouses = db.warehouses.some((warehouse) => warehouse.building_id === id);
    const hasUsers = db.users.some((user) => user.building_id === id);
    const hasTickets = db.tickets.some((ticket) => ticket.building_id === id);
    if (hasWarehouses || hasUsers || hasTickets) {
      return conflict('BUILDING_IN_USE', 'Здание используется: деактивируйте его вместо удаления');
    }
    db.buildings = db.buildings.filter((item) => item.id !== id);
    return noContent();
  }),

  // --- Склады -------------------------------------------------------------
  http.get(`${API}/warehouses`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const buildingId = Number(new URL(request.url).searchParams.get('building_id'));
    const warehouses = db.warehouses
      .filter((warehouse) =>
        Number.isSafeInteger(buildingId) && buildingId > 0
          ? warehouse.building_id === buildingId
          : true,
      )
      .map(toWarehouse);
    return json(warehouses);
  }),

  http.post(`${API}/warehouses`, async ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const body = await readBody<WarehouseCreateRequest>(request);
    const name = typeof body.name === 'string' ? body.name.trim() : '';
    const building = findBuilding(Number(body.building_id));
    if (!building) return validation('Выберите здание');
    if (!name) return validation('Укажите название помещения');

    const duplicate = db.warehouses.some(
      (warehouse) => warehouse.building_id === building.id && warehouse.name === name,
    );
    if (duplicate) return conflict('CONFLICT', 'Помещение с таким названием уже есть в здании');

    const warehouse = {
      id: nextId(db.warehouses),
      building_id: building.id,
      name,
      created_at: new Date().toISOString(),
    };
    db.warehouses.push(warehouse);
    return created(toWarehouse(warehouse));
  }),

  http.patch(`${API}/warehouses/:id`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const warehouse = findWarehouse(pathId(params.id) ?? 0);
    if (!warehouse) return notFound('Помещение не найдено');
    const body = await readBody<WarehouseUpdateRequest>(request);
    if (body.name !== undefined) {
      const name = String(body.name).trim();
      if (!name) return validation('Название не может быть пустым');
      warehouse.name = name;
    }
    return json(toWarehouse(warehouse));
  }),

  http.delete(`${API}/warehouses/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const id = pathId(params.id) ?? 0;
    const warehouse = findWarehouse(id);
    if (!warehouse) return notFound('Помещение не найдено');
    if (db.equipment.some((item) => item.warehouse_id === id && item.deleted_at === null)) {
      return conflict('WAREHOUSE_NOT_EMPTY', 'В помещении есть оборудование — переместите его');
    }
    db.warehouses = db.warehouses.filter((item) => item.id !== id);
    return noContent();
  }),

  // --- Оборудование -------------------------------------------------------
  // EQ-7: фильтры по зданию/складу/типу + поиск, серверная пагинация.
  http.get(`${API}/equipment`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const url = new URL(request.url);
    const page = queryId(request, 'page') ?? 1;
    const size = Math.min(queryId(request, 'size') ?? 20, 100);
    const { items, total } = filterEquipment({
      page,
      size,
      building_id: queryId(request, 'building_id') ?? undefined,
      warehouse_id: queryId(request, 'warehouse_id') ?? undefined,
      equipment_type_id: queryId(request, 'equipment_type_id') ?? undefined,
      search: (url.searchParams.get('search') ?? '').trim() || undefined,
    });
    return json({
      // Стабильный порядок: серверная пагинация не должна «прыгать» между страницами.
      items: [...items].sort((a, b) => a.id - b.id).map(toEquipment),
      total,
      page,
      size,
    });
  }),

  http.post(`${API}/equipment`, async ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const body = await readBody<EquipmentCreateRequest>(request);
    const warehouse = findWarehouse(Number(body.warehouse_id));
    const name = typeof body.name === 'string' ? body.name.trim() : '';
    const serial = typeof body.serial_number === 'string' ? body.serial_number.trim() : '';
    const registration =
      typeof body.registration_number === 'string' ? body.registration_number.trim() : '';

    if (!warehouse) return validation('Выберите помещение');
    if (!db.equipmentTypes.some((type) => type.id === Number(body.equipment_type_id))) {
      return validation('Выберите тип оборудования');
    }
    if (!name || name.length > 200) return validation('Наименование — 1–200 символов');
    if (!serial || serial.length > 100) return validation('Серийный номер — 1–100 символов');
    if (!registration || registration.length > 100) {
      return validation('Регистрационный номер — 1–100 символов');
    }
    if (duplicateRegistrationNumber(registration)) {
      return conflict(
        'DUPLICATE_REGISTRATION_NUMBER',
        'Такой регистрационный номер уже используется',
      );
    }

    const item: MockEquipment = {
      id: nextId(db.equipment),
      name,
      equipment_type_id: Number(body.equipment_type_id),
      serial_number: serial,
      registration_number: registration,
      warehouse_id: warehouse.id,
      created_at: new Date().toISOString(),
      deleted_at: null,
    };
    db.equipment.push(item);
    return created(toEquipment(item));
  }),

  http.get(`${API}/equipment/:id`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const item = db.equipment.find(
      (equipment) => equipment.id === pathId(params.id) && equipment.deleted_at === null,
    );
    if (!item) return notFound('Оборудование не найдено');
    return json(toEquipment(item));
  }),

  http.patch(`${API}/equipment/:id`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const item = db.equipment.find(
      (equipment) => equipment.id === pathId(params.id) && equipment.deleted_at === null,
    );
    if (!item) return notFound('Оборудование не найдено');

    const body = await readBody<EquipmentUpdateRequest>(request);
    if (body.name !== undefined) {
      const name = String(body.name).trim();
      if (!name || name.length > 200) return validation('Наименование — 1–200 символов');
      item.name = name;
    }
    if (body.equipment_type_id !== undefined) {
      if (!db.equipmentTypes.some((type) => type.id === Number(body.equipment_type_id))) {
        return validation('Выберите тип оборудования');
      }
      item.equipment_type_id = Number(body.equipment_type_id);
    }
    if (body.serial_number !== undefined) {
      const serial = String(body.serial_number).trim();
      if (!serial || serial.length > 100) return validation('Серийный номер — 1–100 символов');
      item.serial_number = serial;
    }
    if (body.registration_number !== undefined) {
      const registration = String(body.registration_number).trim();
      if (!registration || registration.length > 100) {
        return validation('Регистрационный номер — 1–100 символов');
      }
      if (duplicateRegistrationNumber(registration, item.id)) {
        return conflict(
          'DUPLICATE_REGISTRATION_NUMBER',
          'Такой регистрационный номер уже используется',
        );
      }
      item.registration_number = registration;
    }
    if (body.warehouse_id !== undefined) {
      if (!findWarehouse(Number(body.warehouse_id))) return validation('Помещение не найдено');
      item.warehouse_id = Number(body.warehouse_id);
    }
    return json(toEquipment(item));
  }),

  // EQ-3: перемещение в «транзакции» с записью журнала.
  //  - тот же склад → 409 SAME_WAREHOUSE;
  //  - неактивное здание (источник или цель) → 422 BUILDING_INACTIVE для USER,
  //    ADMIN перемещает внутри/в неактивное здание (история сохраняется, EQ-6).
  http.post(`${API}/equipment/:id/move`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const item = db.equipment.find(
      (equipment) => equipment.id === pathId(params.id) && equipment.deleted_at === null,
    );
    if (!item) return notFound('Оборудование не найдено');

    const body = await readBody<EquipmentMoveRequest>(request);
    const target = findWarehouse(Number(body.to_warehouse_id));
    if (!target) return validation('Помещение назначения не найдено');
    if (target.id === item.warehouse_id) {
      return fail(409, 'SAME_WAREHOUSE', 'Оборудование уже находится в этом помещении', {
        warehouse_id: target.id,
      });
    }

    const targetBuilding = findBuilding(target.building_id);
    const fromWarehouse = findWarehouse(item.warehouse_id);
    const fromBuilding = findBuilding(fromWarehouse?.building_id ?? null);
    if (auth.user.role !== 'ADMIN' && (!targetBuilding?.is_active || !fromBuilding?.is_active)) {
      return fail(422, 'BUILDING_INACTIVE', 'Нельзя переместить оборудование в неактивное здание', {
        to_building_id: target.building_id,
      });
    }

    const from = item.warehouse_id;
    item.warehouse_id = target.id;
    db.movements.push({
      id: nextId(db.movements),
      equipment_id: item.id,
      from_warehouse_id: from,
      to_warehouse_id: target.id,
      moved_by_user_id: auth.user.id,
      moved_at: new Date().toISOString(),
      comment: typeof body.comment === 'string' && body.comment.trim() ? body.comment.trim() : null,
    });
    return json(toEquipment(item));
  }),

  // EQ-4: массовое перемещение — одна «транзакция», журнал по каждой единице.
  // Несуществующий id → 404 с полным откатом, >200 ids → 422.
  http.post(`${API}/equipment/bulk-move`, async ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const body = await readBody<EquipmentBulkMoveRequest>(request);
    const ids = Array.isArray(body.equipment_ids)
      ? [
          ...new Set(
            body.equipment_ids.map(Number).filter((id) => Number.isSafeInteger(id) && id > 0),
          ),
        ]
      : [];

    if (ids.length < 1 || ids.length > 200) {
      return validation('Выберите от 1 до 200 единиц оборудования', { count: ids.length });
    }
    const target = findWarehouse(Number(body.to_warehouse_id));
    if (!target) return validation('Помещение назначения не найдено');

    const items = ids.map((id) =>
      db.equipment.find((equipment) => equipment.id === id && equipment.deleted_at === null),
    );
    const missing = ids.filter((_id, index) => !items[index]);
    if (missing.length) {
      // Откат: ни одна единица не изменяется, если хотя бы одной нет (EQ-4).
      return fail(404, 'NOT_FOUND', 'Одна из единиц оборудования не найдена', {
        missing_ids: missing,
      });
    }

    const targetBuilding = findBuilding(target.building_id);
    const sourceInactive = items.some((item) => {
      const warehouse = findWarehouse(item!.warehouse_id);
      return findBuilding(warehouse?.building_id ?? null)?.is_active === false;
    });
    if (auth.user.role !== 'ADMIN' && (!targetBuilding?.is_active || sourceInactive)) {
      return fail(422, 'BUILDING_INACTIVE', 'Нельзя переместить оборудование в неактивное здание', {
        to_building_id: target.building_id,
      });
    }

    const toMove = items.filter((item) => item!.warehouse_id !== target.id);
    if (toMove.length === 0) {
      return conflict('SAME_WAREHOUSE', 'Всё выбранное оборудование уже в этом помещении');
    }

    const movedAt = new Date().toISOString();
    toMove.forEach((item) => {
      const from = item!.warehouse_id;
      item!.warehouse_id = target.id;
      db.movements.push({
        id: nextId(db.movements),
        equipment_id: item!.id,
        from_warehouse_id: from,
        to_warehouse_id: target.id,
        moved_by_user_id: auth.user.id,
        moved_at: movedAt,
        comment:
          typeof body.comment === 'string' && body.comment.trim() ? body.comment.trim() : null,
      });
    });

    return json(equipmentBulkMoveResult(toMove.length, target.id));
  }),

  // EQ-5: журнал неизменяем и отдаётся с именами складов/зданий/пользователя.
  http.get(`${API}/equipment/:id/movements`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const id = pathId(params.id) ?? 0;
    const item = db.equipment.find((equipment) => equipment.id === id);
    if (!item) return notFound('Оборудование не найдено');

    const movements = db.movements
      .filter((movement) => movement.equipment_id === id)
      .sort((a, b) => b.moved_at.localeCompare(a.moved_at))
      .map(toMovement);
    return json(paginate(movements, queryId(request, 'page') ?? 1, queryId(request, 'size') ?? 20));
  }),

  // EQ-8: ведомость помещения Excel (в моках — плейсхолдер с нужными заголовками).
  http.get(`${API}/warehouses/:id/inventory.xlsx`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const id = pathId(params.id) ?? 0;
    const warehouse = findWarehouse(id);
    if (!warehouse) return notFound('Помещение не найдено');

    const items = db.equipment.filter(
      (item) => item.warehouse_id === id && item.deleted_at === null,
    );
    if (items.length > 1000) {
      return fail(422, 'VALIDATION_ERROR', 'Слишком много позиций для ведомости (максимум 1000)');
    }

    const rows = items
      .map(
        (item, index) =>
          `${index + 1};${item.name};${db.equipmentTypes.find((type) => type.id === item.equipment_type_id)?.name ?? ''};${item.serial_number};${item.registration_number}`,
      )
      .join('\n');
    const body = `Ведомость помещения (демо MSW)\nЗдание: ${findBuilding(warehouse.building_id)?.name ?? ''}\nПомещение: ${warehouse.name}\n№;Наименование;Тип;Серийный;Регистрационный\n${rows}`;

    return fileResponse(
      body,
      'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
      `inventory_${id}.xlsx`,
    );
  }),

  // Удаление оборудования — только ADMIN, в корзину.
  http.delete(`${API}/equipment/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const item = db.equipment.find(
      (equipment) => equipment.id === pathId(params.id) && equipment.deleted_at === null,
    );
    if (!item) return notFound('Оборудование не найдено');
    item.deleted_at = new Date().toISOString();
    db.trash.push({
      type: 'equipment',
      id: item.id,
      deleted_at: item.deleted_at,
      deleted_by_user_id: auth.user.id,
    });
    return noContent();
  }),
];
