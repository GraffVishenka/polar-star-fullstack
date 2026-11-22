/** MSW: заявки авторизованных пользователей (§3.3, §3.13). */
import { http } from 'msw';
import type {
  TicketCommentCreateRequest,
  TicketCreateRequest,
  TicketListParams,
  TicketUpdateRequest,
} from '@/api/types';
import { guard } from '../auth';
import {
  addWorkdays,
  appToday,
  db,
  filterTickets,
  findBuilding,
  findTicket,
  findUser,
  nextId,
  ticketComments,
  toAssignableUser,
  toTicket,
  toTicketListParams,
} from '../db';
import type { MockTicket, MockUser } from '../db';
import {
  API,
  fail,
  fileResponse,
  json,
  notFound,
  paginate,
  pathId,
  readBody,
  validation,
} from '../http';

/** TICKET-7: USER видит только заявки своего здания (чужое → 404). */
function canSee(user: MockUser, ticket: MockTicket): boolean {
  return user.role === 'ADMIN' || ticket.building_id === user.building_id;
}

function buildingNotAssigned() {
  return fail(409, 'BUILDING_NOT_ASSIGNED', 'Здание не назначено, обратитесь к администратору');
}

function parseTicket(body: Partial<TicketCreateRequest>, user: MockUser) {
  const buildingId = user.role === 'ADMIN' ? Number(body.building_id) : Number(user.building_id);
  const building = findBuilding(buildingId);
  const problemType = db.problemTypes.find((type) => type.id === Number(body.problem_type_id));
  const applicant = typeof body.applicant_name === 'string' ? body.applicant_name.trim() : '';
  const cabinet = typeof body.cabinet === 'string' ? body.cabinet.trim() : '';
  const description = typeof body.description === 'string' ? body.description.trim() : '';

  if (!building || !building.is_active) {
    return { error: validation('Выберите активное здание', { building_id: 'не найдено' }) };
  }
  if (!problemType) {
    return { error: validation('Выберите вид проблемы', { problem_type_id: 'не найдено' }) };
  }
  if (applicant.length < 1 || applicant.length > 120) {
    return { error: validation('Заявитель — от 1 до 120 символов', { applicant_name: '1–120' }) };
  }
  if (cabinet.length < 1 || cabinet.length > 32) {
    return { error: validation('Кабинет — от 1 до 32 символов', { cabinet: '1–32' }) };
  }
  if (description.length < 1 || description.length > 5000) {
    return { error: validation('Описание — от 1 до 5000 символов', { description: '1–5000' }) };
  }
  return { building, problemType, applicant, cabinet, description };
}

export const ticketHandlers = [
  // GET /api/tickets — список с фильтрами и пагинацией (TICKET-15).
  http.get(`${API}/tickets`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    if (auth.user.role === 'USER' && auth.user.building_id === null) return buildingNotAssigned();

    const params: TicketListParams = toTicketListParams(new URL(request.url));
    const userBuildingId = auth.user.role === 'USER' ? auth.user.building_id : null;
    const { items, total } = filterTickets(params, { userBuildingId });

    const size = params.size && params.size > 0 ? Math.min(params.size, 100) : 20;
    const page = params.page && params.page > 0 ? params.page : 1;
    return json({ items: items.map(toTicket), total, page, size });
  }),

  http.post(`${API}/tickets`, async ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    if (auth.user.role === 'USER' && auth.user.building_id === null) return buildingNotAssigned();

    const body = await readBody<TicketCreateRequest>(request);
    // USER создаёт только в своём здании (TICKET-7), ADMIN — в любом.
    if (
      auth.user.role === 'USER' &&
      body.building_id !== undefined &&
      Number(body.building_id) !== auth.user.building_id
    ) {
      return fail(403, 'FORBIDDEN', 'Сотрудник может создать заявку только в своём здании');
    }

    const parsed = parseTicket(body, auth.user);
    if ('error' in parsed) return parsed.error;

    const now = new Date().toISOString();
    const ticket: MockTicket = {
      id: nextId(db.tickets),
      building_id: parsed.building.id,
      applicant_name: parsed.applicant,
      cabinet: parsed.cabinet,
      problem_type_id: parsed.problemType.id,
      description: parsed.description,
      deadline: addWorkdays(appToday(), db.settings.deadline_days, db.settings.workdays),
      executor_id: null,
      status: 'new',
      completed_at: null,
      created_at: now,
      updated_at: now,
      created_by_user_id: auth.user.id,
      access_token: null,
      is_anonymous: false,
      deleted_at: null,
    };
    db.tickets.push(ticket);
    return json(toTicket(ticket), 201);
  }),

  http.get(`${API}/tickets/:id`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    if (auth.user.role === 'USER' && auth.user.building_id === null) return buildingNotAssigned();

    const ticket = findTicket(pathId(params.id) ?? 0);
    if (!ticket || !canSee(auth.user, ticket)) return notFound('Заявка не найдена');
    return json(toTicket(ticket));
  }),

  http.patch(`${API}/tickets/:id`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    if (auth.user.role === 'USER' && auth.user.building_id === null) return buildingNotAssigned();

    const ticket = findTicket(pathId(params.id) ?? 0);
    if (!ticket || !canSee(auth.user, ticket)) return notFound('Заявка не найдена');

    const body = await readBody<TicketUpdateRequest>(request);
    const isAdmin = auth.user.role === 'ADMIN';
    let executorReset = false;

    if (!isAdmin) {
      // TICKET-10: статус меняет USER своего здания.
      // TICKET-11: поля правит только исполнитель.
      const editsFields =
        body.applicant_name !== undefined ||
        body.cabinet !== undefined ||
        body.problem_type_id !== undefined ||
        body.description !== undefined;
      if (editsFields && ticket.executor_id !== auth.user.id) {
        return fail(403, 'NOT_EXECUTOR', 'Править заявку может только её исполнитель');
      }
      if (body.deadline !== undefined) {
        return fail(403, 'FORBIDDEN', 'Дедлайн меняет только администратор');
      }
      if (body.building_id !== undefined) {
        return fail(403, 'FORBIDDEN', 'Здание заявки меняет только администратор');
      }
      // TICKET-8: USER назначает только себя.
      if (body.executor_id !== undefined && body.executor_id !== auth.user.id) {
        return fail(403, 'ASSIGN_OTHER_USER', 'Назначить можно только себя');
      }
    }

    if (body.executor_id !== undefined && body.executor_id !== null) {
      const executor = findUser(Number(body.executor_id));
      if (!executor || !executor.is_active) return validation('Исполнитель не найден');
      if (
        executor.role === 'USER' &&
        executor.building_id !==
          (body.building_id !== undefined ? Number(body.building_id) : ticket.building_id)
      ) {
        return fail(422, 'EXECUTOR_WRONG_BUILDING', 'Исполнитель должен быть из здания заявки');
      }
    }

    if (body.applicant_name !== undefined) {
      const value = String(body.applicant_name).trim();
      if (value.length < 1 || value.length > 120) return validation('Заявитель — 1–120 символов');
      ticket.applicant_name = value;
    }
    if (body.cabinet !== undefined) {
      const value = String(body.cabinet).trim();
      if (value.length < 1 || value.length > 32) return validation('Кабинет — 1–32 символа');
      ticket.cabinet = value;
    }
    if (body.problem_type_id !== undefined) {
      if (!db.problemTypes.some((type) => type.id === Number(body.problem_type_id))) {
        return validation('Вид проблемы не найден');
      }
      ticket.problem_type_id = Number(body.problem_type_id);
    }
    if (body.description !== undefined) {
      const value = String(body.description).trim();
      if (value.length < 1 || value.length > 5000) return validation('Описание — 1–5000 символов');
      ticket.description = value;
    }

    if (body.building_id !== undefined) {
      const building = findBuilding(Number(body.building_id));
      if (!building) return validation('Здание не найдено');
      // TICKET-9: USER-исполнитель из другого здания сбрасывается,
      // ADMIN-исполнитель сохраняется; ответ содержит executor_reset.
      const executor = findUser(ticket.executor_id);
      if (executor && executor.role === 'USER' && executor.building_id !== building.id) {
        ticket.executor_id = null;
        executorReset = true;
      }
      ticket.building_id = building.id;
    }

    if (body.deadline !== undefined) {
      ticket.deadline = body.deadline === null ? null : String(body.deadline);
    }

    if (body.executor_id !== undefined) {
      ticket.executor_id = body.executor_id === null ? null : Number(body.executor_id);
    }

    if (body.status !== undefined) {
      const statuses: MockTicket['status'][] = ['new', 'in_progress', 'deferred', 'done'];
      if (!statuses.includes(body.status)) return validation('Неизвестный статус');
      // TICKET-10: done проставляет completed_at, выход из done обнуляет.
      if (body.status === 'done' && ticket.status !== 'done') {
        ticket.completed_at = new Date().toISOString();
      }
      if (body.status !== 'done' && ticket.status === 'done') {
        ticket.completed_at = null;
      }
      ticket.status = body.status;
    }

    ticket.updated_at = new Date().toISOString();
    return json({ ...toTicket(ticket), executor_reset: executorReset });
  }),

  http.delete(`${API}/tickets/:id`, ({ request, params }) => {
    const auth = guard(request, 'ADMIN');
    if (!auth.ok) return auth.response;
    const ticket = findTicket(pathId(params.id) ?? 0);
    if (!ticket) return notFound('Заявка не найдена');
    ticket.deleted_at = new Date().toISOString();
    db.trash.push({
      type: 'ticket',
      id: ticket.id,
      deleted_at: ticket.deleted_at,
      deleted_by_user_id: auth.user.id,
    });
    return json({ ok: true });
  }),

  http.get(`${API}/tickets/:id/export.docx`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const ticket = findTicket(pathId(params.id) ?? 0);
    if (!ticket || !canSee(auth.user, ticket)) return notFound('Заявка не найдена');
    return fileResponse(
      `Демо Word-заявки №${ticket.id} (MSW). Реальный docx формирует backend Б11.`,
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
      `ticket_${ticket.id}.docx`,
    );
  }),

  http.get(`${API}/tickets/:id/comments`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const ticket = findTicket(pathId(params.id) ?? 0);
    if (!ticket || !canSee(auth.user, ticket)) return notFound('Заявка не найдена');

    const url = new URL(request.url);
    const page = Number(url.searchParams.get('page') ?? '1') || 1;
    const size = Number(url.searchParams.get('size') ?? '50') || 50;
    // TICKET-14: плоский список, 50 на страницу.
    return json(paginate(ticketComments(ticket.id), page, Math.min(size, 50)));
  }),

  http.post(`${API}/tickets/:id/comments`, async ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const ticket = findTicket(pathId(params.id) ?? 0);
    if (!ticket || !canSee(auth.user, ticket)) return notFound('Заявка не найдена');

    const body = await readBody<TicketCommentCreateRequest>(request);
    const text = typeof body.body === 'string' ? body.body.trim() : '';
    const fileIds = Array.isArray(body.file_ids) ? body.file_ids.slice(0, 5) : [];
    if (!text && fileIds.length === 0) {
      return validation('Комментарий пустой: добавьте текст или файл');
    }
    if (text.length > 3000) return validation('Комментарий — не более 3000 символов');

    const comment = {
      id: nextId(db.comments),
      ticket_id: ticket.id,
      author_name: auth.user.full_name,
      author_type: 'user' as const,
      body: text,
      created_at: new Date().toISOString(),
      file_ids: fileIds.filter((id) => db.files.some((file) => file.id === id)),
    };
    db.comments.push(comment);
    ticket.updated_at = comment.created_at;
    db.files
      .filter((file) => comment.file_ids.includes(file.id))
      .forEach((file) => {
        file.ticket_id = ticket.id;
        file.token = null;
      });

    const files = db.files.filter((file) => comment.file_ids.includes(file.id));
    return json(
      {
        id: comment.id,
        ticket_id: comment.ticket_id,
        author_name: comment.author_name,
        author_type: comment.author_type,
        body: comment.body,
        created_at: comment.created_at,
        files: files.map((file) => ({
          id: file.id,
          filename: file.filename,
          content_type: file.content_type,
          size_bytes: file.size_bytes,
          media_kind: file.media_kind,
          url: `/api/files/${file.id}`,
          created_at: file.created_at,
        })),
      },
      201,
    );
  }),

  // TICKET-8: список исполнителей, доступных для назначения.
  http.get(`${API}/users/assignable`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;

    const url = new URL(request.url);
    const ticketId = Number(url.searchParams.get('ticket_id'));
    const ticket = Number.isSafeInteger(ticketId) ? findTicket(ticketId) : undefined;

    if (auth.user.role === 'USER') {
      // USER может назначить только себя.
      return json([toAssignableUser(auth.user)]);
    }

    const users = db.users
      .filter((user) => user.is_active)
      .filter((user) =>
        ticket
          ? user.role === 'ADMIN' || user.building_id === ticket.building_id
          : user.role === 'ADMIN',
      )
      .map(toAssignableUser);
    return json(users);
  }),
];
