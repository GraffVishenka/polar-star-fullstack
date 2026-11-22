/** MSW: публичный периметр (§3.11, TICKET-4..TICKET-6, TICKET-13, PUB-7). */
import { http } from 'msw';
import type { KbNode, PublicTicketCreateRequest } from '@/api/types';
import {
  addWorkdays,
  appToday,
  db,
  fileMeta,
  isKbNodeHidden,
  nextId,
  statusMeta,
  ticketComments,
  toKbNode,
  toKbTree,
  toPublicBuilding,
  toPublicCard,
  toPublicTicket,
} from '../db';
import type { MockTicket } from '../db';
import {
  API,
  fail,
  fileResponse,
  json,
  notFound,
  paginate,
  pathId,
  placeholderSvg,
  readBody,
  validation,
} from '../http';

/** 32 случайных байта base64url — формат access-токена анонима (TICKET-13). */
function randomAccessToken(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  let binary = '';
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function findTicketByToken(token: string): MockTicket | undefined {
  return db.tickets.find((ticket) => ticket.access_token === token && ticket.deleted_at === null);
}

function publicTicketFile(fileId: string | readonly string[] | undefined, token: string) {
  const id = pathId(fileId);
  if (id === null) return null;
  const file = db.files.find((item) => item.id === id);
  if (!file) return null;
  const ticket = findTicketByToken(token);
  // Файл доступен анониму только если он принадлежит этой заявке или из БЗ (KB-5).
  if (file.scope === 'kb') return file;
  return ticket && file.ticket_id === ticket.id ? file : null;
}

function fileBody(file: { content_type: string; filename: string }): string {
  if (file.content_type.startsWith('image/svg')) {
    return placeholderSvg(file.filename);
  }
  if (file.content_type.startsWith('image/')) {
    return placeholderSvg(file.filename);
  }
  if (file.content_type.startsWith('video/')) {
    return '';
  }
  return `Демо-файл: ${file.filename}\nСгенерирован MSW-моками (PLAN.md §3.13).`;
}

export const publicHandlers = [
  // TICKET-6: только активные здания со счётчиками без done.
  http.get(`${API}/public/buildings`, () => {
    const buildings = db.buildings
      .filter((building) => building.is_active)
      .sort((a, b) => a.name.localeCompare(b.name))
      .map(toPublicBuilding);
    return json(buildings);
  }),

  // TICKET-4: справочник видов проблем для публичной формы (аддитивно к §3.13).
  http.get(`${API}/public/problem-types`, () => json(db.problemTypes)),

  // TICKET-5: с building_id — дашборд здания, без него — сводка «Все здания».
  http.get(`${API}/public/tickets`, ({ request }) => {
    const url = new URL(request.url);
    const rawBuildingId = url.searchParams.get('building_id');
    const buildingId = rawBuildingId === null ? null : Number(rawBuildingId);

    if (buildingId !== null) {
      const building = db.buildings.find((item) => item.id === buildingId);
      // PUB-2: несуществующее или неактивное здание → 404.
      if (!building || !building.is_active) return notFound('Здание не найдено');
    }

    const items = db.tickets
      .filter((ticket) => ticket.deleted_at === null)
      .filter((ticket) => ticket.status !== 'done') // TICKET-5: done не отдаётся публично
      .filter((ticket) => (buildingId === null ? true : ticket.building_id === buildingId))
      .sort((a, b) => b.created_at.localeCompare(a.created_at))
      .map(toPublicCard);

    return json({ items, total: items.length });
  }),

  // TICKET-4/TICKET-13: публичное создание заявки + одноразовая выдача токена.
  http.post(`${API}/public/tickets`, async ({ request }) => {
    const body = await readBody<PublicTicketCreateRequest>(request);

    if (body.building_id === undefined || body.building_id === null) {
      return fail(422, 'VALIDATION_ERROR', 'Выберите здание', { building_id: 'обязательное поле' });
    }
    const building = db.buildings.find((item) => item.id === Number(body.building_id));
    if (!building || !building.is_active) return notFound('Здание не найдено');

    const applicant = typeof body.applicant_name === 'string' ? body.applicant_name.trim() : '';
    const cabinet = typeof body.cabinet === 'string' ? body.cabinet.trim() : '';
    const description = typeof body.description === 'string' ? body.description.trim() : '';
    const problemType = db.problemTypes.find((type) => type.id === Number(body.problem_type_id));

    if (applicant.length < 1 || applicant.length > 120) {
      return validation('Укажите заявителя (1–120 символов)', { applicant_name: '1–120 символов' });
    }
    if (cabinet.length < 1 || cabinet.length > 32) {
      return validation('Укажите кабинет (1–32 символа)', { cabinet: '1–32 символа' });
    }
    if (!problemType) {
      return validation('Выберите вид проблемы', { problem_type_id: 'не найдено' });
    }
    if (description.length < 1 || description.length > 5000) {
      return validation('Описание — от 1 до 5000 символов', { description: '1–5000 символов' });
    }

    const now = new Date().toISOString();
    const token = randomAccessToken();
    const ticket: MockTicket = {
      id: nextId(db.tickets),
      building_id: building.id,
      applicant_name: applicant,
      cabinet,
      problem_type_id: problemType.id,
      description,
      // TICKET-3 / CAL-4: N рабочих дней от даты создания (считает сервер).
      deadline: addWorkdays(appToday(), db.settings.deadline_days, db.settings.workdays),
      executor_id: null,
      status: 'new',
      completed_at: null,
      created_at: now,
      updated_at: now,
      created_by_user_id: null,
      access_token: token,
      is_anonymous: true,
      deleted_at: null,
    };
    db.tickets.push(ticket);

    const meta = statusMeta('new');
    return json(
      {
        id: ticket.id,
        access_token: token,
        url: `/t/${token}`,
        deadline: ticket.deadline,
        created_at: ticket.created_at,
        status: ticket.status,
        status_label: meta.label,
      },
      201,
    );
  }),

  http.get(`${API}/public/tickets/:token`, ({ params }) => {
    const ticket = findTicketByToken(String(params.token));
    if (!ticket) return notFound('Заявка не найдена');
    return json(toPublicTicket(ticket));
  }),

  http.get(`${API}/public/tickets/:token/export.docx`, ({ params }) => {
    const ticket = findTicketByToken(String(params.token));
    if (!ticket) return notFound('Заявка не найдена');
    return fileResponse(
      `Демо Word-заявки №${ticket.id} (MSW). Реальный docx формирует backend Б11.`,
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      `ticket_${ticket.id}.docx`,
    );
  }),

  // TICKET-14: комментарии по токену, 50 на страницу (аддитивный GET).
  http.get(`${API}/public/tickets/:token/comments`, ({ params, request }) => {
    const ticket = findTicketByToken(String(params.token));
    if (!ticket) return notFound('Заявка не найдена');

    const url = new URL(request.url);
    const page = Number(url.searchParams.get('page') ?? '1') || 1;
    const size = Number(url.searchParams.get('size') ?? '50') || 50;
    return json(paginate(ticketComments(ticket.id), page, Math.min(size, 50)));
  }),

  // PUB-6: вложение анонима к комментарию (аддитивный multipart-эндпоинт).
  http.post(`${API}/public/tickets/:token/files`, async ({ params, request }) => {
    const ticket = findTicketByToken(String(params.token));
    if (!ticket) return notFound('Заявка не найдена');

    let form: FormData;
    try {
      // В браузере MSW-воркер парсит multipart нативно (в jsdom — нет, тесты
      // подменяют загрузчик, как и в Ф2).
      form = await request.formData();
    } catch {
      return validation('Ожидается multipart/form-data');
    }

    const file = form.get('file');
    if (!(file instanceof Blob)) return validation('Файл не передан');

    const contentType = file.type || 'application/octet-stream';
    const isImage = contentType.startsWith('image/');
    const isVideo = contentType.startsWith('video/');
    if (!isImage && !isVideo) {
      return fail(415, 'UNSUPPORTED_MEDIA_TYPE', 'Можно загружать только фото и видео');
    }
    if (file.size > (isImage ? 10 * 1024 * 1024 : 200 * 1024 * 1024)) {
      return fail(413, 'PAYLOAD_TOO_LARGE', isImage ? 'Фото больше 10 МБ' : 'Видео больше 200 МБ');
    }

    const filename =
      typeof (file as File).name === 'string' && (file as File).name
        ? (file as File).name
        : isImage
          ? 'photo.jpg'
          : 'video.mp4';

    const stored = {
      id: nextId(db.files),
      filename,
      content_type: contentType,
      size_bytes: file.size,
      media_kind: isImage ? ('image' as const) : ('video' as const),
      created_at: new Date().toISOString(),
      scope: 'ticket' as const,
      ticket_id: ticket.id,
      token: ticket.access_token,
    };
    db.files.push(stored);
    return json({ file: fileMeta(stored) }, 201);
  }),

  // TICKET-14: комментарий анонима по токену, до 5 вложений.
  http.post(`${API}/public/tickets/:token/comments`, async ({ params, request }) => {
    const ticket = findTicketByToken(String(params.token));
    if (!ticket) return notFound('Заявка не найдена');

    const body = await readBody<{ body?: string; author_name?: string; file_ids?: number[] }>(
      request,
    );
    const text = typeof body.body === 'string' ? body.body.trim() : '';
    const fileIds = Array.isArray(body.file_ids) ? body.file_ids.slice(0, 5) : [];

    if (!text && fileIds.length === 0) {
      return validation('Комментарий пустой: добавьте текст или файл');
    }
    if (text.length > 3000) {
      return validation('Комментарий — не более 3000 символов');
    }

    const comment = {
      id: nextId(db.comments),
      ticket_id: ticket.id,
      author_name: (body.author_name ?? '').trim() || 'Анонимный посетитель',
      author_type: 'anonymous' as const,
      body: text,
      created_at: new Date().toISOString(),
      file_ids: fileIds.filter((id) => db.files.some((file) => file.id === id)),
    };
    db.comments.push(comment);
    db.files
      .filter((file) => comment.file_ids.includes(file.id))
      .forEach((file) => {
        file.ticket_id = ticket.id;
        file.token = ticket.access_token;
      });

    return json(
      {
        id: comment.id,
        ticket_id: comment.ticket_id,
        author_name: comment.author_name,
        author_type: comment.author_type,
        body: comment.body,
        created_at: comment.created_at,
        files: db.files
          .filter((file) => comment.file_ids.includes(file.id))
          .map((file) => ({
            id: file.id,
            filename: file.filename,
            content_type: file.content_type,
            size_bytes: file.size_bytes,
            media_kind: file.media_kind,
            url: `/api/public/files/${file.id}?token=${encodeURIComponent(ticket.access_token ?? '')}`,
            created_at: file.created_at,
          })),
      },
      201,
    );
  }),

  http.get(`${API}/public/files/:id`, ({ params, request }) => {
    const token = new URL(request.url).searchParams.get('token') ?? '';
    const file = publicTicketFile(params.id, token);
    if (!file) return notFound('Файл не найден');
    return fileResponse(
      file.scope === 'kb' ? placeholderSvg(file.filename) : fileBody(file),
      file.content_type,
      undefined,
    );
  }),

  // KB-7: аноним видит только публичные корневые разделы и их потомков.
  http.get(`${API}/public/kb/tree`, () => {
    const visibleIds = new Set(
      db.kbNodes.filter((node) => !isKbNodeHidden(node)).map((node) => node.id),
    );
    return json(toKbTree((node) => visibleIds.has(node.id)));
  }),

  http.get(`${API}/public/kb/nodes/:id`, ({ params }) => {
    const id = pathId(params.id);
    const node = db.kbNodes.find((item) => item.id === id);
    if (!node || isKbNodeHidden(node)) return notFound('Раздел не найден');
    return json<KbNode>(toKbNode(node));
  }),
];

export { fileBody };
