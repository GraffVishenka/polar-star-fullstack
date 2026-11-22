/**
 * Права на действия с заявкой — зеркало матрицы §3.1/§3.3 на клиенте.
 *
 * Сервер проверяет те же правила (403/404), клиент лишь прячет недоступные
 * действия, чтобы не показывать заведомо ошибочные кнопки.
 */
import type { AuthUser, Ticket } from '@/api/types';

export interface TicketPermissions {
  /** TICKET-11: поля правит исполнитель, ADMIN — всегда. */
  canEditFields: boolean;
  /** TICKET-10: статус меняет USER своего здания, ADMIN — любой. */
  canChangeStatus: boolean;
  /** TICKET-8: USER назначает только себя, в своём здании. */
  canAssignSelf: boolean;
  /** TICKET-8: ADMIN назначает USER из здания заявки или ADMIN. */
  canAssignAny: boolean;
  /** TICKET-8: снимает исполнителя только ADMIN. */
  canUnassign: boolean;
  /** TICKET-9: дедлайн меняет только ADMIN. */
  canChangeDeadline: boolean;
  /** TICKET-9: здание заявки меняет только ADMIN. */
  canChangeBuilding: boolean;
  /** TICKET-12: удаление в корзину — только ADMIN. */
  canDelete: boolean;
  /** TICKET-14: комментарии — сотрудники здания заявки и ADMIN. */
  canComment: boolean;
}

export function getTicketPermissions(
  ticket: Ticket | null,
  user: AuthUser | null,
): TicketPermissions {
  const isAdmin = user?.role === 'ADMIN';
  const sameBuilding = user !== null && ticket !== null && ticket.building_id === user.building_id;
  const isExecutor = user !== null && ticket !== null && ticket.executor_id === user.id;

  return {
    canEditFields: Boolean(ticket) && (isAdmin || (sameBuilding && isExecutor)),
    canChangeStatus: Boolean(ticket) && (isAdmin || sameBuilding),
    canAssignSelf: user?.role === 'USER' && Boolean(ticket) && sameBuilding && !isExecutor,
    canAssignAny: isAdmin,
    canUnassign: isAdmin,
    canChangeDeadline: isAdmin,
    canChangeBuilding: isAdmin,
    canDelete: isAdmin,
    canComment: Boolean(ticket) && (isAdmin || sameBuilding),
  };
}

/**
 * TICKET-9: при смене здания USER-исполнитель из другого здания снимается,
 * ADMIN-исполнитель сохраняется. USER-исполнитель всегда принадлежит зданию
 * заявки (TICKET-8), поэтому смена здания = сброс.
 */
export function willResetExecutor(ticket: Ticket | null, targetBuildingId: number | null): boolean {
  if (!ticket || targetBuildingId === null) return false;
  if (ticket.executor_id === null) return false;
  if (ticket.executor_role !== 'USER') return false;
  return targetBuildingId !== ticket.building_id;
}

/** AUTH-9: у сотрудника не назначено здание — заявки недоступны. */
export function needsBuildingAssignment(user: AuthUser | null): boolean {
  return user?.role === 'USER' && !user.building_id;
}
