/**
 * MSW: база знаний (§3.5, §3.13).
 *
 * Моделируется поведение бэкенда (Б6), чтобы UI-тесты проверяли реальные
 * правила, а не заглушки:
 *  - KB-2: вложенность любая, цикл → `409 CYCLE_DETECTED`, нет родителя → `400`;
 *  - KB-3: удаление только ADMIN, каскадно, ответ с числом потомков;
 *  - KB-4: санитизация `content_html` по allowlist (эмуляция bluemonday);
 *  - KB-5: ссылки на медиа проверяются — чужой/удалённый файл → `400 INVALID_MEDIA_REF`;
 *  - KB-6: `PATCH`/`move` переиндексируют соседей шагом 10;
 *  - KB-7: `is_hidden_for_anon` только у корней, у потомков наследуется.
 */
import { http } from 'msw';
import type { KbMoveRequest, KbNodeCreateRequest, KbNodeUpdateRequest } from '@/api/types';
import { guard } from '../auth';
import { db, findUser, isKbNodeHidden, nextId, toKbNode, toKbTree, type MockKbNode } from '../db';
import { API, conflict, fail, json, notFound, pathId, readBody, validation } from '../http';

function childrenOf(parentId: number | null): MockKbNode[] {
  return db.kbNodes
    .filter((node) => node.parent_id === parentId)
    .sort((a, b) => a.sort_order - b.sort_order || a.title.localeCompare(b.title));
}

function descendantIds(id: number, acc: number[] = []): number[] {
  db.kbNodes
    .filter((node) => node.parent_id === id)
    .forEach((child) => {
      acc.push(child.id);
      descendantIds(child.id, acc);
    });
  return acc;
}

/** KB-6: переиндексация соседей шагом 10 (порядок в массиве — итоговый). */
function reindex(parentId: number | null, ordered?: MockKbNode[]): void {
  const list = ordered ?? childrenOf(parentId);
  list.forEach((node, index) => {
    node.sort_order = (index + 1) * 10;
  });
}

/** Эмуляция bluemonday (KB-4): скрипты, iframe, обработчики onX и style вырезаются. */
export function sanitizeMockKbHtml(html: string): string {
  return html
    .replace(/<script[\s\S]*?<\/script>/gi, '')
    .replace(/<(iframe|object|embed|form|style)[\s\S]*?<\/\1>/gi, '')
    .replace(/<(iframe|object|embed|form|style|input|button)\b[^>]*>/gi, '')
    .replace(/\son[a-z]+\s*=\s*"[^"]*"/gi, '')
    .replace(/\son[a-z]+\s*=\s*'[^']*'/gi, '')
    .replace(/\sstyle\s*=\s*"[^"]*"/gi, '')
    .replace(/\sstyle\s*=\s*'[^']*'/gi, '')
    .replace(/javascript:/gi, '');
}

function mediaIdsIn(html: string): number[] {
  const ids = new Set<number>();
  const pattern = /\/api\/files\/(\d+)/g;
  let match: RegExpExecArray | null = pattern.exec(html);
  while (match) {
    ids.add(Number(match[1]));
    match = pattern.exec(html);
  }
  return [...ids];
}

/** KB-5: все ссылки на файлы должны существовать и быть загружены со scope=kb. */
function invalidMedia(html: string): number | null {
  for (const id of mediaIdsIn(html)) {
    const file = db.files.find((item) => item.id === id);
    if (!file || file.scope !== 'kb') return id;
  }
  return null;
}

function normalizeParent(raw: unknown): number | null | 'invalid' {
  if (raw === undefined || raw === null || raw === '') return null;
  const value = Number(raw);
  return Number.isSafeInteger(value) && value > 0 ? value : 'invalid';
}

/** Корневой раздел может быть скрыт; у потомков флаг всегда сброшен (KB-1). */
function syncHiddenFlag(node: MockKbNode): void {
  if (node.parent_id !== null) node.is_hidden_for_anon = false;
}

function touch(node: MockKbNode, userName: string): void {
  node.updated_at = new Date().toISOString();
  node.updated_by_name = userName;
}

export const kbHandlers = [
  // KB-8: дерево одним запросом, сортировка `sort_order, title`.
  http.get(`${API}/kb/tree`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    return json(toKbTree());
  }),

  http.get(`${API}/kb/nodes/:id`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');
    return json(toKbNode(node));
  }),

  http.post(`${API}/kb/nodes`, async ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const body = await readBody<KbNodeCreateRequest>(request);
    const title = typeof body.title === 'string' ? body.title.trim() : '';
    if (!title || title.length > 200) {
      return validation('Заголовок — от 1 до 200 символов', { title: '1–200' });
    }

    const parentId = normalizeParent(body.parent_id);
    if (
      parentId === 'invalid' ||
      (parentId !== null && !db.kbNodes.some((n) => n.id === parentId))
    ) {
      // KB-2: удалённый/несуществующий родитель → 400.
      return fail(400, 'VALIDATION_ERROR', 'Родительский раздел не найден');
    }

    const contentHtml =
      typeof body.content_html === 'string' ? sanitizeMockKbHtml(body.content_html) : '';
    const badMedia = invalidMedia(contentHtml);
    if (badMedia !== null) {
      return fail(400, 'INVALID_MEDIA_REF', 'Файл не найден или доступен в другой области', {
        file_id: badMedia,
      });
    }

    const siblings = childrenOf(parentId);
    const now = new Date().toISOString();
    const node: MockKbNode = {
      id: nextId(db.kbNodes),
      title,
      content_html: contentHtml,
      parent_id: parentId,
      sort_order: 0,
      is_hidden_for_anon: Boolean(body.is_hidden_for_anon) && parentId === null,
      created_at: now,
      updated_at: now,
      updated_by_name: auth.user.full_name,
      media_ids: mediaIdsIn(contentHtml),
    };

    // Сохраняем порядок: новый узел — в конец, либо на позицию sort_order (KB-6).
    const ordered = [...siblings];
    const requested = body.sort_order === undefined ? NaN : Number(body.sort_order);
    if (Number.isFinite(requested)) {
      const position = ordered.findIndex((item) => item.sort_order > requested);
      ordered.splice(position === -1 ? ordered.length : position, 0, node);
    } else {
      ordered.push(node);
    }
    db.kbNodes.push(node);
    reindex(parentId, ordered);

    return json(toKbNode(node), 201);
  }),

  http.patch(`${API}/kb/nodes/:id`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');

    const body = await readBody<KbNodeUpdateRequest>(request);
    const previousParent = node.parent_id;

    if (body.title !== undefined) {
      const title = String(body.title).trim();
      if (!title || title.length > 200) return validation('Заголовок — от 1 до 200 символов');
      node.title = title;
    }

    if (body.is_hidden_for_anon !== undefined) {
      // KB-7: тумблер видимости — только ADMIN, только у корневых разделов.
      if (auth.user.role !== 'ADMIN') {
        return fail(403, 'FORBIDDEN', 'Тумблер видимости доступен администратору');
      }
      if (node.parent_id !== null) {
        return validation('Видимость настраивается только у корневых разделов');
      }
      node.is_hidden_for_anon = Boolean(body.is_hidden_for_anon);
    }

    if (body.content_html !== undefined) {
      if (String(body.content_html).length > 100_000) {
        return validation('Содержимое статьи — не более 100 000 символов');
      }
      const html = sanitizeMockKbHtml(String(body.content_html));
      const badMedia = invalidMedia(html);
      if (badMedia !== null) {
        return fail(400, 'INVALID_MEDIA_REF', 'Файл не найден или доступен в другой области', {
          file_id: badMedia,
        });
      }
      node.content_html = html;
      node.media_ids = mediaIdsIn(html);
    }

    if (body.parent_id !== undefined) {
      const parentId = normalizeParent(body.parent_id);
      if (parentId === 'invalid') return validation('Некорректный родительский раздел');
      if (parentId === node.id || descendantIds(node.id).includes(parentId ?? -1)) {
        // KB-2: цикл запрещён.
        return conflict('CYCLE_DETECTED', 'Нельзя вложить раздел в самого себя или потомка');
      }
      if (parentId !== null && !db.kbNodes.some((item) => item.id === parentId)) {
        return fail(400, 'VALIDATION_ERROR', 'Родительский раздел не найден');
      }
      node.parent_id = parentId;
      syncHiddenFlag(node);
    }

    if (body.sort_order !== undefined) {
      const sortOrder = Number(body.sort_order);
      if (!Number.isFinite(sortOrder)) return validation('Некорректный порядок сортировки');
      node.sort_order = sortOrder;
    }

    touch(node, auth.user.full_name);
    reindex(node.parent_id);
    if (previousParent !== node.parent_id) reindex(previousParent);

    return json(toKbNode(node));
  }),

  // KB-6: перенос с переиндексацией соседей шагом 10.
  http.post(`${API}/kb/nodes/:id/move`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');

    const body = await readBody<KbMoveRequest>(request);
    const parentId = normalizeParent(body.parent_id);
    if (parentId === 'invalid') return validation('Некорректный родительский раздел');
    const previousParent = node.parent_id;

    if (parentId === node.id || descendantIds(node.id).includes(parentId ?? -1)) {
      return conflict('CYCLE_DETECTED', 'Нельзя переместить раздел внутрь себя');
    }
    if (parentId !== null && !db.kbNodes.some((item) => item.id === parentId)) {
      return fail(400, 'VALIDATION_ERROR', 'Родительский раздел не найден');
    }

    const beforeId =
      body.before_node_id === undefined || body.before_node_id === null
        ? null
        : Number(body.before_node_id);
    if (beforeId !== null && !Number.isSafeInteger(beforeId)) {
      return validation('Некорректный опорный узел');
    }
    const siblings = childrenOf(parentId).filter((item) => item.id !== node.id);
    if (beforeId !== null && !siblings.some((item) => item.id === beforeId)) {
      // Опорный узел должен быть соседом в целевом родителе.
      return fail(400, 'VALIDATION_ERROR', 'Опорный раздел не найден среди соседей');
    }

    node.parent_id = parentId;
    syncHiddenFlag(node);
    const insertAt =
      beforeId === null ? siblings.length : siblings.findIndex((i) => i.id === beforeId);
    const ordered = [...siblings];
    ordered.splice(insertAt, 0, node);
    reindex(parentId, ordered);
    if (previousParent !== parentId) reindex(previousParent);
    touch(node, auth.user.full_name);

    return json(toKbNode(node));
  }),

  // KB-3: удаление только ADMIN, каскадно в корзину, ответ — число потомков.
  http.delete(`${API}/kb/nodes/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');

    const removed = [node.id, ...descendantIds(node.id)];
    const parentId = node.parent_id;
    db.kbNodes = db.kbNodes.filter((item) => !removed.includes(item.id));
    reindex(parentId);
    db.trash.push({
      type: 'kb',
      id: node.id,
      deleted_at: new Date().toISOString(),
      deleted_by_user_id: auth.user.id,
      // TRASH-2: снимок корневого раздела — восстановление без потомков (TRASH-5).
      snapshot: {
        title: node.title,
        parent_id: node.parent_id,
        sort_order: node.sort_order,
        is_hidden_for_anon: node.is_hidden_for_anon,
      },
    });
    return json({ deleted_id: node.id, descendants_deleted: removed.length - 1 });
  }),

  http.patch(`${API}/kb/nodes/:id/visibility`, async ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');
    if (node.parent_id !== null) {
      return validation('Видимость настраивается только у корневых разделов');
    }

    const body = await readBody<{ is_hidden_for_anon?: boolean }>(request);
    node.is_hidden_for_anon = Boolean(body.is_hidden_for_anon);
    touch(node, findUser(auth.user.id)?.full_name ?? auth.user.full_name);
    return json({ ...toKbNode(node), is_hidden_effective: isKbNodeHidden(node) });
  }),
];
