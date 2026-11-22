/** Заявки авторизованных пользователей (§3.3). USER — только своё здание (TICKET-7). */
import { apiRequest } from './client';
import type {
  AssignableUser,
  Page,
  Ticket,
  TicketComment,
  TicketCommentCreateRequest,
  TicketCreateRequest,
  TicketListParams,
  TicketUpdateRequest,
  TicketUpdateResponse,
} from './types';

function toQuery(
  params: TicketListParams,
): Record<string, string | number | boolean | null | undefined | Array<string | number>> {
  return {
    page: params.page,
    size: params.size,
    status: params.status,
    executor_id: params.executor_id,
    problem_type_id: params.problem_type_id,
    // TICKET-7: у USER фильтр заблокирован на своём здании и сервер его игнорирует.
    building_id: params.building_id === 'all' ? undefined : params.building_id,
    date_from: params.date_from,
    date_to: params.date_to,
    search: params.search,
    sort_by: params.sort_by,
    sort_dir: params.sort_dir,
  };
}

/** TICKET-15: серверная пагинация и фильтры. */
export function fetchTickets(params: TicketListParams = {}): Promise<Page<Ticket>> {
  return apiRequest<Page<Ticket>>('/tickets', { query: toQuery(params) });
}

export function fetchTicket(id: number): Promise<Ticket> {
  return apiRequest<Ticket>(`/tickets/${id}`);
}

export function createTicket(body: TicketCreateRequest): Promise<Ticket> {
  return apiRequest<Ticket>('/tickets', { method: 'POST', body });
}

/** Правка полей/статуса/исполнителя; ответ может содержать `executor_reset` (TICKET-9). */
export function updateTicket(id: number, body: TicketUpdateRequest): Promise<TicketUpdateResponse> {
  return apiRequest<TicketUpdateResponse>(`/tickets/${id}`, { method: 'PATCH', body });
}

/** TICKET-12: удаление в корзину — только ADMIN. */
export function deleteTicket(id: number): Promise<void> {
  return apiRequest<void>(`/tickets/${id}`, { method: 'DELETE' });
}

export function fetchTicketComments(
  id: number,
  params: { page?: number; size?: number } = {},
): Promise<Page<TicketComment>> {
  return apiRequest<Page<TicketComment>>(`/tickets/${id}/comments`, { query: params });
}

export function createTicketComment(
  id: number,
  body: TicketCommentCreateRequest,
): Promise<TicketComment> {
  return apiRequest<TicketComment>(`/tickets/${id}/comments`, { method: 'POST', body });
}

/** DOC-1: путь Word-выгрузки для ExportButton (скачивание идёт через apiDownload). */
export function ticketExportPath(id: number): string {
  return `/tickets/${id}/export.docx`;
}

/** TICKET-8: список исполнителей, доступных для назначения на заявку. */
export function fetchAssignableUsers(ticketId?: number): Promise<AssignableUser[]> {
  return apiRequest<AssignableUser[]>('/users/assignable', { query: { ticket_id: ticketId } });
}
