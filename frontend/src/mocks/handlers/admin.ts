/** MSW: админ-панель — пользователи, настройки, справочники, праздники, корзина (§3.2, §3.4, §3.10). */
import { http } from 'msw';
import type {
  AdminUserCreateRequest,
  AdminUserUpdateRequest,
  AppSettingsUpdateRequest,
} from '@/api/types';
import { guard } from '../auth';
import {
  db,
  computeStats,
  findBuilding,
  findUser,
  nextId,
  toAdminUser,
  toTrashItem,
  type MockDictionaryItem,
  type MockUser,
} from '../db';
import {
  API,
  conflict,
  created,
  fail,
  json,
  noContent,
  notFound,
  paginate,
  pathId,
  readBody,
  validation,
} from '../http';

const WEEKDAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

/** TRASH-4: срок хранения корзины до автоочистки. */
const TRASH_RETENTION_DAYS = 90;

/** Идентификаторы потомков узла БЗ — для окончательного удаления (TRASH-3). */
function descendantKbIds(id: number): number[] {
  const collect = (parentId: number, acc: number[] = []): number[] => {
    db.kbNodes
      .filter((node) => node.parent_id === parentId)
      .forEach((child) => {
        acc.push(child.id);
        collect(child.id, acc);
      });
    return acc;
  };
  return collect(id);
}

function isKnownTimezone(timezone: string): boolean {
  try {
    new Intl.DateTimeFormat('ru-RU', { timeZone: timezone });
    return true;
  } catch {
    return false;
  }
}

function activeAdmins(): MockUser[] {
  return db.users.filter((user) => user.role === 'ADMIN' && user.is_active);
}

function validateUserBuilding(role: string, buildingId: number | null | undefined): string | null {
  if (role !== 'USER') return null;
  // AUTH-4: у USER здание обязательно.
  if (buildingId === null || buildingId === undefined) {
    return 'BUILDING_REQUIRED_FOR_USER';
  }
  const building = findBuilding(Number(buildingId));
  if (!building) return 'BUILDING_REQUIRED_FOR_USER';
  // AUTH-8: перевод в неактивное здание запрещён.
  if (!building.is_active) return 'BUILDING_INACTIVE';
  return null;
}

// --- Справочники (Ф6): деактивация вместо удаления -------------------------

type DictionaryKind = 'problem' | 'equipment';

function dictionaryOf(kind: DictionaryKind): MockDictionaryItem[] {
  return kind === 'problem' ? db.problemTypes : db.equipmentTypes;
}

/** Сколько действующих записей ссылается на значение справочника. */
function dictionaryUsage(kind: DictionaryKind, id: number): number {
  return kind === 'problem'
    ? db.tickets.filter((ticket) => ticket.problem_type_id === id && ticket.deleted_at === null)
        .length
    : db.equipment.filter((item) => item.equipment_type_id === id && item.deleted_at === null)
        .length;
}

function toDictionary(item: MockDictionaryItem, kind: DictionaryKind) {
  return { ...item, usage_count: dictionaryUsage(kind, item.id) };
}

function sortedDictionaries(kind: DictionaryKind) {
  return [...dictionaryOf(kind)].sort(
    (a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name),
  );
}

function nextDictionarySortOrder(kind: DictionaryKind): number {
  const items = dictionaryOf(kind);
  return items.length ? Math.max(...items.map((item) => item.sort_order)) + 10 : 10;
}

function dictionaryNameError(
  kind: DictionaryKind,
  name: string,
  excludeId?: number,
): { status: number; code: string; message: string } | null {
  if (!name) return { status: 422, code: 'VALIDATION_ERROR', message: 'Укажите название' };
  if (name.length > 100) {
    return { status: 422, code: 'VALIDATION_ERROR', message: 'Название — не более 100 символов' };
  }
  const duplicate = dictionaryOf(kind).some(
    (item) => item.id !== excludeId && item.name.toLowerCase() === name.toLowerCase(),
  );
  return duplicate ? { status: 409, code: 'CONFLICT', message: 'Такое название уже есть' } : null;
}

function parseDictionaryBody(body: {
  name?: unknown;
  sort_order?: unknown;
  is_active?: unknown;
}): { name?: string; sort_order?: number; is_active?: boolean; error?: string } {
  const parsed: { name?: string; sort_order?: number; is_active?: boolean; error?: string } = {};
  if (body.name !== undefined) {
    parsed.name = typeof body.name === 'string' ? body.name.trim() : '';
  }
  if (body.sort_order !== undefined) {
    const order = Number(body.sort_order);
    if (!Number.isInteger(order) || order < 0 || order > 100_000) {
      return { error: 'Порядок сортировки — целое число от 0' };
    }
    parsed.sort_order = order;
  }
  if (body.is_active !== undefined) parsed.is_active = Boolean(body.is_active);
  return parsed;
}

export const adminHandlers = [
  // --- Пользователи -------------------------------------------------------
  http.get(`${API}/admin/users`, ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const url = new URL(request.url);
    const role = url.searchParams.get('role');
    const buildingId = Number(url.searchParams.get('building_id'));
    const isActive = url.searchParams.get('is_active');
    const search = (url.searchParams.get('search') ?? '').toLowerCase();
    const page = Number(url.searchParams.get('page') ?? '1') || 1;
    const size = Number(url.searchParams.get('size') ?? '20') || 20;

    const items = db.users
      .filter((user) => (role ? user.role === role : true))
      .filter((user) =>
        Number.isSafeInteger(buildingId) && buildingId > 0 ? user.building_id === buildingId : true,
      )
      .filter((user) => (isActive === null ? true : String(user.is_active) === isActive))
      .filter((user) =>
        search
          ? user.full_name.toLowerCase().includes(search) ||
            user.login.toLowerCase().includes(search)
          : true,
      )
      .map(toAdminUser);

    return json(paginate(items, page, size));
  }),

  http.post(`${API}/admin/users`, async ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const body = await readBody<AdminUserCreateRequest>(request);

    const login = typeof body.login === 'string' ? body.login.trim() : '';
    const fullName = typeof body.full_name === 'string' ? body.full_name.trim() : '';
    const password = typeof body.password === 'string' ? body.password : '';
    const role = body.role === 'ADMIN' ? 'ADMIN' : 'USER';
    const buildingId =
      body.building_id === undefined || body.building_id === null ? null : Number(body.building_id);

    if (!login) return validation('Укажите логин');
    if (db.users.some((user) => user.login.toLowerCase() === login.toLowerCase())) {
      return conflict('CONFLICT', 'Логин уже используется');
    }
    if (!fullName) return validation('Укажите ФИО');
    if (password.length < 8) return validation('Временный пароль — не короче 8 символов');

    const buildingError = validateUserBuilding(role, buildingId);
    if (buildingError === 'BUILDING_REQUIRED_FOR_USER') {
      return fail(422, 'BUILDING_REQUIRED_FOR_USER', 'Для сотрудника обязательно выберите здание');
    }
    if (buildingError === 'BUILDING_INACTIVE') {
      return fail(422, 'BUILDING_INACTIVE', 'Здание деактивировано');
    }

    const user: MockUser = {
      id: nextId(db.users),
      login,
      password,
      full_name: fullName,
      role,
      building_id: role === 'USER' ? buildingId : null,
      is_active: true,
      // AUTH-4: новый аккаунт создаётся с временным паролем.
      must_change_password: true,
      created_at: new Date().toISOString(),
      last_login_at: null,
    };
    db.users.push(user);
    return created(toAdminUser(user));
  }),

  http.patch(`${API}/admin/users/:id`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const user = findUser(pathId(params.id) ?? 0);
    if (!user) return notFound('Пользователь не найден');
    if (user.id === auth.user.id) {
      return conflict('CANNOT_MODIFY_SELF', 'Нельзя изменить собственную учётную запись');
    }

    const body = await readBody<AdminUserUpdateRequest>(request);
    const nextRole = body.role ?? user.role;
    const nextBuildingId =
      body.building_id !== undefined
        ? body.building_id === null
          ? null
          : Number(body.building_id)
        : user.building_id;
    const nextActive = body.is_active ?? user.is_active;

    const losingAdmin = user.role === 'ADMIN' && (nextRole !== 'ADMIN' || nextActive === false);
    if (losingAdmin && activeAdmins().length <= 1) {
      return conflict('LAST_ADMIN', 'Нельзя понизить или заблокировать последнего администратора');
    }

    if (nextRole === 'USER') {
      const buildingError = validateUserBuilding('USER', nextBuildingId);
      if (buildingError === 'BUILDING_REQUIRED_FOR_USER') {
        return fail(
          422,
          'BUILDING_REQUIRED_FOR_USER',
          'Для сотрудника обязательно выберите здание',
        );
      }
      if (buildingError === 'BUILDING_INACTIVE') {
        return fail(422, 'BUILDING_INACTIVE', 'Здание деактивировано');
      }
    }

    if (body.full_name !== undefined) {
      const fullName = String(body.full_name).trim();
      if (!fullName) return validation('ФИО не может быть пустым');
      user.full_name = fullName;
    }
    if (body.role !== undefined) {
      user.role = body.role;
      // AUTH-8: USER→ADMIN очищает здание, ADMIN→USER требует здание.
      user.building_id = body.role === 'ADMIN' ? null : nextBuildingId;
    } else if (body.building_id !== undefined) {
      user.building_id = nextBuildingId;
    }
    if (body.is_active !== undefined) user.is_active = Boolean(body.is_active);

    return json(toAdminUser(user));
  }),

  http.post(`${API}/admin/users/:id/password`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const user = findUser(pathId(params.id) ?? 0);
    if (!user) return notFound('Пользователь не найден');

    const body = await readBody<{ password?: string }>(request);
    const password = typeof body.password === 'string' ? body.password : '';
    if (password.length < 8) return validation('Пароль — не короче 8 символов');

    user.password = password;
    user.must_change_password = true;
    return noContent();
  }),

  // --- Настройки ----------------------------------------------------------
  http.get(`${API}/admin/settings`, ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    return json({ ...db.settings, holidays: db.holidays });
  }),

  http.put(`${API}/admin/settings`, async ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const body = await readBody<AppSettingsUpdateRequest>(request);

    if (body.deadline_days !== undefined) {
      const days = Number(body.deadline_days);
      if (!Number.isInteger(days) || days < 1 || days > 30) {
        return validation('Срок дедлайна — от 1 до 30 рабочих дней', { deadline_days: '1–30' });
      }
      db.settings.deadline_days = days;
    }
    if (body.workdays !== undefined) {
      if (!Array.isArray(body.workdays) || body.workdays.length === 0) {
        return validation('Выберите хотя бы один рабочий день');
      }
      if (body.workdays.some((day) => !WEEKDAYS.includes(day))) {
        return validation('Недопустимый код дня недели');
      }
      db.settings.workdays = [...body.workdays];
    }
    if (body.timezone !== undefined) {
      const timezone = String(body.timezone).trim();
      // CAL-2: валидируется как IANA-идентификатор.
      if (!timezone || !isKnownTimezone(timezone)) {
        return validation('Неизвестный часовой пояс', {
          timezone: 'IANA, например Asia/Yekaterinburg',
        });
      }
      db.settings.timezone = timezone;
    }
    if (body.deadline_reminder_time !== undefined) {
      const time = String(body.deadline_reminder_time).trim();
      // CAL-3: HH:MM, 00:00–23:59.
      if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(time)) {
        return validation('Время напоминания в формате HH:MM', { deadline_reminder_time: 'HH:MM' });
      }
      db.settings.deadline_reminder_time = time;
    }
    if (body.push_enabled !== undefined) db.settings.push_enabled = Boolean(body.push_enabled);
    if (body.app_name !== undefined) {
      const name = String(body.app_name).trim();
      if (!name) {
        return validation('Название приложения не может быть пустым', { app_name: 'обязательное' });
      }
      if (name.length > 120) {
        return validation('Название приложения — не более 120 символов', {
          app_name: 'не более 120',
        });
      }
      db.settings.app_name = name;
    }
    if (body.statuses !== undefined) {
      const allowed = ['new', 'in_progress', 'deferred', 'done'];
      if (
        !Array.isArray(body.statuses) ||
        body.statuses.length !== 4 ||
        body.statuses.some((status) => !allowed.includes(status.code))
      ) {
        return validation('Передайте названия и цвета для 4 фиксированных статусов', {
          statuses: '4 фиксированных кода',
        });
      }
      const invalidLabel = body.statuses.some((status) => !String(status.label).trim());
      const invalidColor = body.statuses.some(
        (status) => !/^#[0-9a-f]{6}$/i.test(String(status.color).trim()),
      );
      if (invalidLabel || invalidColor) {
        return validation('Название статуса не пустое, цвет — HEX вида #RRGGBB', {
          statuses: invalidLabel ? 'пустое название' : 'цвет #RRGGBB',
        });
      }
      db.settings.statuses = body.statuses.map((status) => ({
        code: status.code,
        label: String(status.label).trim(),
        color: String(status.color).trim(),
      }));
    }

    db.settings.updated_at = new Date().toISOString();
    return json({ ...db.settings, holidays: db.holidays });
  }),

  // --- Справочники (Ф6): деактивация вместо удаления -----------------------
  http.get(`${API}/admin/problem-types`, ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    return json(sortedDictionaries('problem').map((item) => toDictionary(item, 'problem')));
  }),

  http.post(`${API}/admin/problem-types`, async ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const body = await readBody<{ name?: unknown; sort_order?: unknown }>(request);
    const parsed = parseDictionaryBody(body);
    if (parsed.error) return validation(parsed.error);
    const nameError = dictionaryNameError('problem', parsed.name ?? '');
    if (nameError) return fail(nameError.status, nameError.code, nameError.message);
    const item: MockDictionaryItem = {
      id: nextId(db.problemTypes),
      name: parsed.name ?? '',
      sort_order: parsed.sort_order ?? nextDictionarySortOrder('problem'),
      is_active: true,
    };
    db.problemTypes.push(item);
    return created(toDictionary(item, 'problem'));
  }),

  http.patch(`${API}/admin/problem-types/:id`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const item = db.problemTypes.find((type) => type.id === pathId(params.id));
    if (!item) return notFound('Вид проблемы не найден');
    const body = await readBody<{ name?: unknown; sort_order?: unknown; is_active?: unknown }>(
      request,
    );
    const parsed = parseDictionaryBody(body);
    if (parsed.error) return validation(parsed.error);
    if (parsed.name !== undefined) {
      const nameError = dictionaryNameError('problem', parsed.name, item.id);
      if (nameError) return fail(nameError.status, nameError.code, nameError.message);
      item.name = parsed.name;
    }
    if (parsed.sort_order !== undefined) item.sort_order = parsed.sort_order;
    if (parsed.is_active !== undefined) item.is_active = parsed.is_active;
    return json(toDictionary(item, 'problem'));
  }),

  http.delete(`${API}/admin/problem-types/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const id = pathId(params.id) ?? 0;
    // Удаление в UI запрещено: деактивированный справочник остаётся в истории.
    if (dictionaryUsage('problem', id) > 0) {
      return conflict('CONFLICT', 'Вид проблемы используется в заявках — деактивируйте его');
    }
    db.problemTypes = db.problemTypes.filter((type) => type.id !== id);
    return noContent();
  }),

  http.get(`${API}/admin/equipment-types`, ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    return json(sortedDictionaries('equipment').map((item) => toDictionary(item, 'equipment')));
  }),

  http.post(`${API}/admin/equipment-types`, async ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const body = await readBody<{ name?: unknown; sort_order?: unknown }>(request);
    const parsed = parseDictionaryBody(body);
    if (parsed.error) return validation(parsed.error);
    const nameError = dictionaryNameError('equipment', parsed.name ?? '');
    if (nameError) return fail(nameError.status, nameError.code, nameError.message);
    const item: MockDictionaryItem = {
      id: nextId(db.equipmentTypes),
      name: parsed.name ?? '',
      sort_order: parsed.sort_order ?? nextDictionarySortOrder('equipment'),
      is_active: true,
    };
    db.equipmentTypes.push(item);
    return created(toDictionary(item, 'equipment'));
  }),

  http.patch(`${API}/admin/equipment-types/:id`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const item = db.equipmentTypes.find((type) => type.id === pathId(params.id));
    if (!item) return notFound('Тип оборудования не найден');
    const body = await readBody<{ name?: unknown; sort_order?: unknown; is_active?: unknown }>(
      request,
    );
    const parsed = parseDictionaryBody(body);
    if (parsed.error) return validation(parsed.error);
    if (parsed.name !== undefined) {
      const nameError = dictionaryNameError('equipment', parsed.name, item.id);
      if (nameError) return fail(nameError.status, nameError.code, nameError.message);
      item.name = parsed.name;
    }
    if (parsed.sort_order !== undefined) item.sort_order = parsed.sort_order;
    if (parsed.is_active !== undefined) item.is_active = parsed.is_active;
    return json(toDictionary(item, 'equipment'));
  }),

  http.delete(`${API}/admin/equipment-types/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const id = pathId(params.id) ?? 0;
    if (dictionaryUsage('equipment', id) > 0) {
      return conflict('CONFLICT', 'Тип используется в оборудовании — деактивируйте его');
    }
    db.equipmentTypes = db.equipmentTypes.filter((type) => type.id !== id);
    return noContent();
  }),

  // --- Праздники (CAL-1) --------------------------------------------------
  http.get(`${API}/admin/holidays`, ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    return json(db.holidays);
  }),

  http.post(`${API}/admin/holidays`, async ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const body = await readBody<{ date?: string; name?: string }>(request);
    const date = typeof body.date === 'string' ? body.date.trim() : '';
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) {
      return validation('Дата праздника в формате YYYY-MM-DD');
    }
    if (db.holidays.some((holiday) => holiday.date === date)) {
      return conflict('CONFLICT', 'Праздник на эту дату уже добавлен');
    }
    const holiday = {
      id: nextId(db.holidays),
      date,
      name: typeof body.name === 'string' && body.name.trim() ? body.name.trim() : 'Праздник',
    };
    db.holidays.push(holiday);
    return created(holiday);
  }),

  http.patch(`${API}/admin/holidays/:id`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const holiday = db.holidays.find((item) => item.id === pathId(params.id));
    if (!holiday) return notFound('Праздник не найден');
    const body = await readBody<{ date?: string; name?: string }>(request);
    if (body.date !== undefined) {
      const date = String(body.date).trim();
      if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) {
        return validation('Дата праздника в формате YYYY-MM-DD', { date: 'YYYY-MM-DD' });
      }
      // Дубликат даты ломает идемпотентность рабочих дней.
      if (db.holidays.some((item) => item.id !== holiday.id && item.date === date)) {
        return conflict('CONFLICT', 'Праздник на эту дату уже добавлен');
      }
      holiday.date = date;
    }
    if (body.name !== undefined) {
      const name = String(body.name).trim();
      if (!name) return validation('Название праздника не может быть пустым', { name: 'обязательное' });
      holiday.name = name;
    }
    return json(holiday);
  }),

  http.delete(`${API}/admin/holidays/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const id = pathId(params.id) ?? 0;
    if (!db.holidays.some((holiday) => holiday.id === id)) return notFound('Праздник не найден');
    db.holidays = db.holidays.filter((holiday) => holiday.id !== id);
    return noContent();
  }),

  // --- Корзина (TRASH-1..TRASH-5) ----------------------------------------
  http.get(`${API}/admin/trash`, ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const url = new URL(request.url);
    const type = url.searchParams.get('type') ?? 'ticket';
    const page = Number(url.searchParams.get('page') ?? '1') || 1;
    const size = Number(url.searchParams.get('size') ?? '20') || 20;

    // TRASH-4: элементы старше 90 дней удаляются ежедневной джобой — мок
    // отбрасывает их на чтении, чтобы дата автоочистки была не декоративной.
    const cutoff = Date.now() - TRASH_RETENTION_DAYS * 24 * 60 * 60 * 1000;
    const items = db.trash
      .filter((item) => item.type === type)
      .filter((item) => new Date(item.deleted_at).getTime() >= cutoff)
      .sort((a, b) => b.deleted_at.localeCompare(a.deleted_at))
      .map(toTrashItem);
    return json(paginate(items, page, size));
  }),

  http.post(`${API}/admin/trash/:type/:id/restore`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const type = String(params.type);
    const id = pathId(params.id) ?? 0;
    const trashItem = db.trash.find((item) => item.type === type && item.id === id);
    if (!trashItem) return notFound('Элемент корзины не найден');

    const removeFromTrash = () => {
      db.trash = db.trash.filter((item) => !(item.type === type && item.id === id));
    };
    const restored = (warnings: string[] = []) => json({ type, id, restored: true, warnings });

    if (type === 'ticket') {
      const ticket = db.tickets.find((item) => item.id === id);
      if (!ticket) return notFound('Заявка не найдена');
      // TRASH-2: заявка возвращается со статусом и зданием, completed_at сохраняется.
      ticket.deleted_at = null;
      removeFromTrash();
      const building = findBuilding(ticket.building_id);
      return restored(
        building && !building.is_active
          ? ['Здание деактивировано: заявка восстановлена, но публично недоступна']
          : [],
      );
    }

    if (type === 'kb') {
      // TRASH-2/TRASH-5: узел восстанавливается один, без потомков;
      // если родитель удалён — восстановление в него запрещено.
      const existing = db.kbNodes.find((item) => item.id === id);
      if (existing) {
        // Данные уже на месте (например, элемент попал в корзину повторно) — снимаем метку.
        removeFromTrash();
        return restored(['Потомки узла не восстановлены']);
      }
      const snapshot = trashItem.snapshot;
      const parentId = snapshot?.parent_id ?? null;
      if (parentId !== null && !db.kbNodes.some((item) => item.id === parentId)) {
        return conflict(
          'CONFLICT',
          'Родительский раздел удалён: восстановите его первым или восстановите узел в корень',
        );
      }
      const now = new Date().toISOString();
      db.kbNodes.push({
        id,
        title: snapshot?.title ?? `Узел №${id}`,
        content_html: '',
        parent_id: parentId,
        sort_order: snapshot?.sort_order ?? 10,
        is_hidden_for_anon: Boolean(snapshot?.is_hidden_for_anon) && parentId === null,
        created_at: now,
        updated_at: now,
        updated_by_name: auth.user.full_name,
        media_ids: [],
      });
      removeFromTrash();
      return restored(['Потомки узла не восстановлены (TRASH-5)']);
    }

    const equipment = db.equipment.find((item) => item.id === id);
    if (!equipment) return notFound('Оборудование не найдено');
    if (!db.warehouses.some((warehouse) => warehouse.id === equipment.warehouse_id)) {
      return conflict('CONFLICT', 'Помещение оборудования больше не существует');
    }
    // TRASH-2: конфликт регистрационного номера среди неудалённых единиц → 409.
    const duplicate = db.equipment.some(
      (other) =>
        other.id !== equipment.id &&
        other.deleted_at === null &&
        other.registration_number.toLowerCase() === equipment.registration_number.toLowerCase(),
    );
    if (duplicate) {
      return fail(409, 'DUPLICATE_REGISTRATION_NUMBER', 'Регистрационный номер уже занят', {
        registration_number: equipment.registration_number,
      });
    }
    equipment.deleted_at = null;
    removeFromTrash();
    return restored();
  }),

  http.delete(`${API}/admin/trash/:type/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const type = String(params.type);
    const id = pathId(params.id) ?? 0;
    if (!db.trash.some((item) => item.type === type && item.id === id)) {
      return notFound('Элемент корзины не найден');
    }

    // TRASH-3: физическое удаление необратимо.
    if (type === 'ticket') {
      db.tickets = db.tickets.filter((ticket) => ticket.id !== id);
      db.comments = db.comments.filter((comment) => comment.ticket_id !== id);
      db.files = db.files.filter((file) => file.ticket_id !== id);
    } else if (type === 'kb') {
      const removed = [id, ...descendantKbIds(id)];
      db.kbNodes = db.kbNodes.filter((node) => !removed.includes(node.id));
    } else {
      db.equipment = db.equipment.filter((item) => item.id !== id);
      db.movements = db.movements.filter((movement) => movement.equipment_id !== id);
    }
    db.trash = db.trash.filter((item) => !(item.type === type && item.id === id));
    return noContent();
  }),

  // Сводная статистика — та же форма, что STATS-1.
  http.get(`${API}/admin/stats`, ({ request }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const url = new URL(request.url);
    const rawUserId = Number(url.searchParams.get('user_id'));
    const rawBuildingId = Number(url.searchParams.get('building_id'));
    const userId = Number.isSafeInteger(rawUserId) && rawUserId > 0 ? rawUserId : null;
    const buildingId =
      Number.isSafeInteger(rawBuildingId) && rawBuildingId > 0 ? rawBuildingId : null;
    const from = url.searchParams.get('from') ?? undefined;
    const to = url.searchParams.get('to') ?? undefined;

    // STATS-3: период по `completed_at`; обратный диапазон — 422.
    if (from && to && from > to) {
      return validation('Дата начала периода позже даты окончания', { from: 'должна быть <= to' });
    }

    const stats = computeStats(userId, buildingId, from, to);
    if (userId) return json(stats);

    // Аддитивное расширение (согласуется с lead/be): сводка по сотрудникам,
    // чтобы `/admin/stats` мог показать таблицу без N отдельных запросов.
    const employees = db.users
      .filter((user) => user.role === 'USER' && user.is_active)
      .filter((user) => !buildingId || user.building_id === buildingId)
      .map((user) => {
        const employeeStats = computeStats(user.id, buildingId, from, to);
        return {
          user_id: user.id,
          user_name: user.full_name,
          total_done: employeeStats.total_done,
          done_in_period: employeeStats.done_in_period,
          avg_resolution_hours: employeeStats.avg_resolution_hours,
          median_resolution_hours: employeeStats.median_resolution_hours,
          on_time_percent: employeeStats.on_time_percent,
          overdue_count: employeeStats.overdue_count,
        };
      });
    return json({ ...stats, employees });
  }),
];
