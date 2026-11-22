/** Публичный периметр (§3.11, §3.13): доступен без авторизации. */
import { apiRequest, buildUrl } from './client';
import { uploadFileToEndpoint } from './upload';
import type {
  FileMeta,
  KbNode,
  KbTreeNode,
  Page,
  ProblemType,
  PublicBuilding,
  PublicCommentCreateRequest,
  PublicTicket,
  PublicTicketCard,
  PublicTicketCreateRequest,
  PublicTicketCreated,
  PublicTicketList,
  TicketComment,
} from './types';

/** TICKET-6: только активные здания с названием и счётчиками new/in_progress/deferred. */
export function fetchPublicBuildings(): Promise<PublicBuilding[]> {
  return apiRequest<PublicBuilding[]>('/public/buildings', { skipAuth: true });
}

/**
 * TICKET-5: без `building_id` — сводка «Все здания», с ним — дашборд здания.
 * Публичный ответ не содержит `description`, комментариев и файлов.
 */
export function fetchPublicTickets(buildingId?: number | null): Promise<PublicTicketList> {
  return apiRequest<PublicTicketList>('/public/tickets', {
    skipAuth: true,
    query: buildingId ? { building_id: buildingId } : undefined,
  });
}

/** TICKET-4: аноним создаёт заявку с обязательным зданием. */
export function createPublicTicket(body: PublicTicketCreateRequest): Promise<PublicTicketCreated> {
  return apiRequest<PublicTicketCreated>('/public/tickets', {
    method: 'POST',
    body,
    skipAuth: true,
    skipRefresh: true,
  });
}

/** TICKET-13: карточка по бессрочному токену; чужой/битый токен → 404. */
export function fetchPublicTicket(token: string): Promise<PublicTicket> {
  return apiRequest<PublicTicket>(`/public/tickets/${encodeURIComponent(token)}`, {
    skipAuth: true,
  });
}

export function createPublicComment(
  token: string,
  body: PublicCommentCreateRequest,
): Promise<TicketComment> {
  return apiRequest<TicketComment>(`/public/tickets/${encodeURIComponent(token)}/comments`, {
    method: 'POST',
    body,
    skipAuth: true,
    skipRefresh: true,
  });
}

/** DOC-1: Word-выгрузка анонимной заявки по токену. */
export function publicTicketExportPath(token: string): string {
  return `/public/tickets/${encodeURIComponent(token)}/export.docx`;
}

/** Готовый путь карточки по токену (TICKET-13). */
export function publicTicketPath(token: string): string {
  return `/t/${token}`;
}

/**
 * Виды проблем для публичной формы (TICKET-4).
 *
 * Контракт §3.13 не отдаёт справочник анониму, а `problem_type_id` обязателен
 * (TICKET-2). MSW реализует `GET /api/public/problem-types`; это аддитивное
 * расширение — при фиксации `openapi.yaml` согласовать с lead/be.
 */
export function fetchPublicProblemTypes(): Promise<ProblemType[]> {
  return apiRequest<ProblemType[]>('/public/problem-types', { skipAuth: true });
}

/**
 * Комментарии по токену с серверной пагинацией (TICKET-14: 50 на страницу).
 * Аддитивный `GET /api/public/tickets/{token}/comments` — в контракте §3.13
 * был только POST; нужен, чтобы не отдавать весь список в карточке.
 */
export function fetchPublicComments(
  token: string,
  params: { page?: number; size?: number } = {},
): Promise<Page<TicketComment>> {
  return apiRequest<Page<TicketComment>>(`/public/tickets/${encodeURIComponent(token)}/comments`, {
    skipAuth: true,
    query: params,
  });
}

/**
 * Вложение анонима к комментарию (PUB-6): до 5 файлов, фото/видео.
 * Аддитивный `POST /api/public/tickets/{token}/files` — `POST /api/files`
 * доступен только USER/ADMIN (Б3), у анонима аккаунта нет.
 */
export function uploadPublicCommentFile(
  token: string,
  file: File,
  options: { onProgress?: (percent: number) => void; signal?: AbortSignal } = {},
): Promise<FileMeta> {
  return uploadFileToEndpoint(
    buildUrl(`/public/tickets/${encodeURIComponent(token)}/files`),
    file,
    { withAuth: false, onProgress: options.onProgress, signal: options.signal },
  );
}

/** БЗ-7: публичное дерево без скрытых корневых разделов. */
export function fetchPublicKbTree(): Promise<KbTreeNode[]> {
  return apiRequest<KbTreeNode[]>('/public/kb/tree', { skipAuth: true });
}

export function fetchPublicKbNode(id: number): Promise<KbNode> {
  return apiRequest<KbNode>(`/public/kb/nodes/${id}`, { skipAuth: true });
}

/** Файл комментария/статьи, доступный анониму только по токену заявки. */
export function publicFileUrl(fileId: number, token: string): string {
  return `/api/public/files/${fileId}?token=${encodeURIComponent(token)}`;
}

export type { PublicTicketCard };
