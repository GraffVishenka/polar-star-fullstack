/**
 * Авторизация в моках: Bearer-токен вида `mock-access-token-{userId}`
 * и in-memory «refresh-cookie» (в браузере настоящая cookie httpOnly,
 * в моках достаточно модульной переменной).
 *
 * Поведение соответствует AUTH-2/AUTH-4/AUTH-6:
 *  - заблокированный пользователь → 401 на любом запросе;
 *  - `must_change_password` → 428 на всех эндпоинтах, кроме me/password/logout;
 *  - роли проверяются до бизнес-логики.
 */
import type { Role } from '@/api/types';
import { db, findUser, type MockUser } from './db';
import { fail, passwordChangeRequired, unauthorized } from './http';

export const ACCESS_TOKEN_PREFIX = 'mock-access-token-';

/** Пути, разрешённые при `must_change_password` (AUTH-4). */
const PASSWORD_CHANGE_ALLOWED = new Set(['/api/auth/me', '/api/auth/password', '/api/auth/logout']);

let refreshUserId: number | null = null;

export function issueAccessToken(userId: number): string {
  return `${ACCESS_TOKEN_PREFIX}${userId}`;
}

/** Имитация httpOnly refresh-cookie: устанавливается при login, живёт до logout. */
export function startRefreshSession(userId: number): void {
  refreshUserId = userId;
}

export function currentRefreshUserId(): number | null {
  return refreshUserId;
}

export function clearRefreshSession(): void {
  refreshUserId = null;
}

export function tokenUserId(request: Request): number | null {
  const header = request.headers.get('Authorization');
  if (!header?.startsWith('Bearer ')) return null;
  const token = header.slice('Bearer '.length).trim();
  if (!token.startsWith(ACCESS_TOKEN_PREFIX)) return null;
  const raw = token.slice(ACCESS_TOKEN_PREFIX.length);
  const id = Number(raw);
  if (!Number.isSafeInteger(id) || id <= 0) return null;
  return id;
}

export type GuardResult =
  { ok: true; user: MockUser } | { ok: false; response: ReturnType<typeof fail> };

/** Проверка авторизации/роли/обязательной смены пароля. */
export function guard(request: Request, role?: Role | Role[]): GuardResult {
  const userId = tokenUserId(request);
  if (userId === null) return { ok: false, response: unauthorized() };

  const user = findUser(userId);
  // AUTH-6: блокировка действует со следующего запроса.
  if (!user || !user.is_active) return { ok: false, response: unauthorized() };

  const path = new URL(request.url).pathname;
  if (user.must_change_password && !PASSWORD_CHANGE_ALLOWED.has(path)) {
    return { ok: false, response: passwordChangeRequired() };
  }

  if (role) {
    const roles = Array.isArray(role) ? role : [role];
    if (!roles.includes(user.role)) {
      return {
        ok: false,
        response: fail(403, 'FORBIDDEN', 'Недостаточно прав для этого действия'),
      };
    }
  }

  return { ok: true, user };
}

/** Поиск пользователя по логину (без учёта регистра). */
export function findByLogin(login: string): MockUser | undefined {
  const normalized = login.trim().toLowerCase();
  return db.users.find((user) => user.login.toLowerCase() === normalized);
}
