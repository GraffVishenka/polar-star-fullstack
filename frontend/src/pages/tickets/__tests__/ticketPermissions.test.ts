/** Права на действия с заявкой и правила сброса исполнителя (TICKET-8..TICKET-12). */
import { describe, expect, it } from 'vitest';
import type { AuthUser, Role, Ticket } from '@/api/types';
import {
  getTicketPermissions,
  needsBuildingAssignment,
  willResetExecutor,
} from '../components/ticketPermissions';

function makeUser(overrides: Partial<AuthUser> = {}): AuthUser {
  return {
    id: 2,
    login: 'user',
    full_name: 'Иванов Иван Иванович',
    role: 'USER',
    building_id: 1,
    building_name: 'Центральный офис',
    must_change_password: false,
    is_active: true,
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function makeTicket(overrides: Partial<Ticket> = {}): Ticket {
  return {
    id: 1,
    building_id: 1,
    building_name: 'Центральный офис',
    applicant_name: 'Иванов Иван Иванович',
    cabinet: 'каб. 204',
    problem_type_id: 1,
    problem_type_name: 'Освещение',
    description: 'Не работает свет',
    deadline: '2026-03-05',
    is_overdue: false,
    executor_id: null,
    executor_name: null,
    executor_role: null,
    status: 'new',
    status_label: 'Новая',
    status_color: '#1677ff',
    completed_at: null,
    created_at: '2026-03-01T10:00:00Z',
    updated_at: '2026-03-01T10:00:00Z',
    is_anonymous: false,
    created_by_user_id: 2,
    comments_count: 0,
    files: [],
    ...overrides,
  };
}

function withRole(role: Role): AuthUser {
  return makeUser(
    role === 'ADMIN' ? { role: 'ADMIN', building_id: null, building_name: null } : { role: 'USER' },
  );
}

describe('getTicketPermissions', () => {
  it('USER своего здания может менять статус и назначать себя', () => {
    const permissions = getTicketPermissions(makeTicket(), withRole('USER'));

    expect(permissions.canChangeStatus).toBe(true);
    expect(permissions.canAssignSelf).toBe(true);
    expect(permissions.canComment).toBe(true);
    expect(permissions.canEditFields).toBe(false);
    expect(permissions.canUnassign).toBe(false);
    expect(permissions.canChangeDeadline).toBe(false);
    expect(permissions.canChangeBuilding).toBe(false);
    expect(permissions.canDelete).toBe(false);
  });

  it('USER-исполнитель может править поля, но не может переназначить себя повторно', () => {
    const ticket = makeTicket({ executor_id: 2, executor_name: 'Иванов', executor_role: 'USER' });
    const permissions = getTicketPermissions(ticket, withRole('USER'));

    expect(permissions.canEditFields).toBe(true);
    expect(permissions.canAssignSelf).toBe(false);
  });

  it('USER не видит чужие заявки: прав нет', () => {
    const ticket = makeTicket({ building_id: 2, building_name: 'Производственный корпус' });
    const permissions = getTicketPermissions(ticket, withRole('USER'));

    expect(permissions.canChangeStatus).toBe(false);
    expect(permissions.canComment).toBe(false);
    expect(permissions.canAssignSelf).toBe(false);
    expect(permissions.canEditFields).toBe(false);
  });

  it('ADMIN может всё, включая снятие исполнителя и удаление', () => {
    const permissions = getTicketPermissions(makeTicket(), withRole('ADMIN'));

    expect(permissions.canChangeStatus).toBe(true);
    expect(permissions.canEditFields).toBe(true);
    expect(permissions.canAssignAny).toBe(true);
    expect(permissions.canUnassign).toBe(true);
    expect(permissions.canChangeDeadline).toBe(true);
    expect(permissions.canChangeBuilding).toBe(true);
    expect(permissions.canDelete).toBe(true);
    expect(permissions.canAssignSelf).toBe(false);
  });

  it('без заявки и пользователя действий нет', () => {
    const permissions = getTicketPermissions(null, null);
    expect(Object.values(permissions).every((value) => value === false)).toBe(true);
  });
});

describe('willResetExecutor (TICKET-9)', () => {
  it('USER-исполнитель снимается при переезде в другое здание', () => {
    const ticket = makeTicket({ executor_id: 2, executor_role: 'USER' });
    expect(willResetExecutor(ticket, 2)).toBe(true);
  });

  it('ADMIN-исполнитель сохраняется', () => {
    const ticket = makeTicket({ executor_id: 1, executor_role: 'ADMIN' });
    expect(willResetExecutor(ticket, 2)).toBe(false);
  });

  it('без исполнителя и при том же здании сброса нет', () => {
    expect(willResetExecutor(makeTicket(), 2)).toBe(false);
    expect(willResetExecutor(makeTicket({ executor_id: 2, executor_role: 'USER' }), 1)).toBe(false);
  });
});

describe('needsBuildingAssignment (AUTH-9)', () => {
  it('true только у USER без здания', () => {
    expect(needsBuildingAssignment(makeUser({ building_id: null }))).toBe(true);
    expect(needsBuildingAssignment(makeUser())).toBe(false);
    expect(needsBuildingAssignment(withRole('ADMIN'))).toBe(false);
    expect(needsBuildingAssignment(null)).toBe(false);
  });
});
