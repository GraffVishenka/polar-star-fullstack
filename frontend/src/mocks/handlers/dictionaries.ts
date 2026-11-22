/**
 * MSW: справочники и настройки, доступные авторизованным/публично.
 *
 * Аддитивные расширения контракта §3.13 (Б7): без них USER не может заполнить
 * форму заявки, а публичные экраны — показать названия и цвета статусов.
 *   - `GET /api/problem-types`   — виды проблем для форм и фильтров (USER/ADMIN);
 *   - `GET /api/equipment-types` — типы оборудования для формы (USER/ADMIN);
 *   - `GET /api/public/settings` — название приложения, календарь и статусы (аноним).
 *
 * Возвращаются только активные значения: деактивированные справочники остаются
 * в старых записях (ТИКЕТ-2, EQ-2), но не предлагаются для новых.
 */
import { http } from 'msw';
import { guard } from '../auth';
import { db } from '../db';
import { API, json } from '../http';

function activeProblemTypes() {
  return db.problemTypes
    .filter((item) => item.is_active)
    .sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name));
}

function activeEquipmentTypes() {
  return db.equipmentTypes
    .filter((item) => item.is_active)
    .sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name));
}

export const dictionaryHandlers = [
  http.get(`${API}/problem-types`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    return json(activeProblemTypes());
  }),

  http.get(`${API}/equipment-types`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    return json(activeEquipmentTypes());
  }),

  // Публичные настройки (без авторизации): только безопасные поля.
  http.get(`${API}/public/settings`, () =>
    json({
      app_name: db.settings.app_name,
      deadline_days: db.settings.deadline_days,
      workdays: db.settings.workdays,
      timezone: db.settings.timezone,
      statuses: db.settings.statuses,
    }),
  ),
];
