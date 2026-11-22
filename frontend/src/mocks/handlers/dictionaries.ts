/**
 * MSW: справочники и настройки, доступные авторизованным/публично.
 *
 * Контракт Б7:
 *   - `GET /api/problem-types`   — активные виды проблем (USER/ADMIN);
 *     `include_inactive=true` учитывается только для ADMIN;
 *   - `GET /api/equipment-types` — аналогично для типов оборудования;
 *   - `GET /api/public/settings` — только `app_name` и статусы (PUB-1);
 *     календарь, часовой пояс, срок и push наружу не отдаются.
 *
 * Деактивированные элементы остаются в старых записях (ТИКЕТ-2, EQ-2),
 * но не предлагаются для новых.
 */
import { http } from 'msw';
import { guard } from '../auth';
import { db, type MockDictionaryItem } from '../db';
import { API, json } from '../http';

function parseIncludeInactive(request: Request): boolean {
  return new URL(request.url).searchParams.get('include_inactive') === 'true';
}

function visibleDictionaries(
  items: MockDictionaryItem[],
  includeInactive: boolean,
): MockDictionaryItem[] {
  return items
    .filter((item) => includeInactive || item.is_active)
    .sort((a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name));
}

export const dictionaryHandlers = [
  http.get(`${API}/problem-types`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    // `include_inactive` раскрывает историю только администратору.
    const includeInactive = auth.user.role === 'ADMIN' && parseIncludeInactive(request);
    return json(visibleDictionaries(db.problemTypes, includeInactive));
  }),

  http.get(`${API}/equipment-types`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const includeInactive = auth.user.role === 'ADMIN' && parseIncludeInactive(request);
    return json(visibleDictionaries(db.equipmentTypes, includeInactive));
  }),

  // Публичные настройки (без авторизации): только безопасное подмножество.
  http.get(`${API}/public/settings`, () =>
    json({
      app_name: db.settings.app_name,
      statuses: db.settings.statuses,
    }),
  ),
];
