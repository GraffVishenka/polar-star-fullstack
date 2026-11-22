/** MSW: база знаний (§3.5, §3.13). */
import { http } from 'msw';
import type { KbMoveRequest, KbNodeCreateRequest, KbNodeUpdateRequest } from '@/api/types';
import { guard } from '../auth';
import { db, findUser, isKbNodeHidden, nextId, toKbNode, toKbTree } from '../db';
import { API, conflict, fail, json, notFound, pathId, readBody, validation } from '../http';

function descendantIds(id: number, acc: number[] = []): number[] {
  const children = db.kbNodes.filter((node) => node.parent_id === id);
  children.forEach((child) => {
    acc.push(child.id);
    descendantIds(child.id, acc);
  });
  return acc;
}

export const kbHandlers = [
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

    const parentId =
      body.parent_id === undefined || body.parent_id === null ? null : Number(body.parent_id);
    if (parentId !== null && !db.kbNodes.some((node) => node.id === parentId)) {
      return fail(400, 'VALIDATION_ERROR', 'Родительский раздел не найден');
    }

    const siblings = db.kbNodes.filter((node) => node.parent_id === parentId);
    const now = new Date().toISOString();
    const node = {
      id: nextId(db.kbNodes),
      title,
      content_html: typeof body.content_html === 'string' ? body.content_html : '',
      parent_id: parentId,
      sort_order:
        body.sort_order ??
        (siblings.length ? Math.max(...siblings.map((node) => node.sort_order)) + 10 : 10),
      is_hidden_for_anon: Boolean(body.is_hidden_for_anon) && parentId === null,
      created_at: now,
      updated_at: now,
      updated_by_name: auth.user.full_name,
      media_ids: [] as number[],
    };
    db.kbNodes.push(node);
    return json(toKbNode(node), 201);
  }),

  http.patch(`${API}/kb/nodes/:id`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');

    const body = await readBody<KbNodeUpdateRequest>(request);

    if (body.is_hidden_for_anon !== undefined) {
      // KB-7: тумблер видимости — только ADMIN, только у корневых разделов.
      if (auth.user.role !== 'ADMIN')
        return fail(403, 'FORBIDDEN', 'Тумблер видимости доступен администратору');
      if (node.parent_id !== null) {
        return validation('Видимость настраивается только у корневых разделов');
      }
      node.is_hidden_for_anon = Boolean(body.is_hidden_for_anon);
    }

    if (body.title !== undefined) {
      const title = String(body.title).trim();
      if (!title || title.length > 200) return validation('Заголовок — от 1 до 200 символов');
      node.title = title;
    }
    if (body.content_html !== undefined) {
      if (String(body.content_html).length > 100_000) {
        return validation('Содержимое статьи — не более 100 000 символов');
      }
      // KB-4: на бэкенде санитизация bluemonday; в моках вырезаем script/on*.
      node.content_html = String(body.content_html)
        .replace(/<script[\s\S]*?<\/script>/gi, '')
        .replace(/\son\w+="[^"]*"/gi, '');
    }
    if (body.parent_id !== undefined) {
      const parentId = body.parent_id === null ? null : Number(body.parent_id);
      if (parentId === node.id || descendantIds(node.id).includes(parentId ?? -1)) {
        return conflict('CYCLE_DETECTED', 'Нельзя вложить раздел в самого себя или потомка');
      }
      if (parentId !== null && !db.kbNodes.some((item) => item.id === parentId)) {
        return fail(400, 'VALIDATION_ERROR', 'Родительский раздел не найден');
      }
      node.parent_id = parentId;
    }
    if (body.sort_order !== undefined) node.sort_order = Number(body.sort_order);

    node.updated_at = new Date().toISOString();
    node.updated_by_name = auth.user.full_name;
    return json(toKbNode(node));
  }),

  // KB-6: перенос с переиндексацией соседей шагом 10.
  http.post(`${API}/kb/nodes/:id/move`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');

    const body = await readBody<KbMoveRequest>(request);
    const parentId =
      body.parent_id === undefined || body.parent_id === null ? null : Number(body.parent_id);

    if (parentId === node.id || descendantIds(node.id).includes(parentId ?? -1)) {
      return conflict('CYCLE_DETECTED', 'Нельзя переместить раздел внутрь себя');
    }
    if (parentId !== null && !db.kbNodes.some((item) => item.id === parentId)) {
      return fail(400, 'VALIDATION_ERROR', 'Родительский раздел не найден');
    }

    node.parent_id = parentId;
    const siblings = db.kbNodes
      .filter((item) => item.parent_id === parentId && item.id !== node.id)
      .sort((a, b) => a.sort_order - b.sort_order);

    const beforeId =
      body.before_node_id === undefined || body.before_node_id === null
        ? null
        : Number(body.before_node_id);
    const foundIndex = beforeId === null ? -1 : siblings.findIndex((item) => item.id === beforeId);
    const insertIndex = foundIndex === -1 ? siblings.length : foundIndex;
    const ordered = [...siblings];
    ordered.splice(insertIndex, 0, node);

    ordered.forEach((item, index) => {
      item.sort_order = (index + 1) * 10;
      item.updated_at = new Date().toISOString();
    });
    node.updated_by_name = auth.user.full_name;

    return json(toKbNode(node));
  }),

  // KB-3: удаление только ADMIN, каскадно в корзину, ответ — число потомков.
  http.delete(`${API}/kb/nodes/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const node = db.kbNodes.find((item) => item.id === pathId(params.id));
    if (!node) return notFound('Раздел не найден');

    const removed = [node.id, ...descendantIds(node.id)];
    db.kbNodes = db.kbNodes.filter((item) => !removed.includes(item.id));
    db.trash.push({
      type: 'kb',
      id: node.id,
      deleted_at: new Date().toISOString(),
      deleted_by_user_id: auth.user.id,
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
    node.updated_at = new Date().toISOString();
    node.updated_by_name = findUser(auth.user.id)?.full_name ?? auth.user.full_name;
    return json({ ...toKbNode(node), is_hidden_effective: isKbNodeHidden(node) });
  }),
];
