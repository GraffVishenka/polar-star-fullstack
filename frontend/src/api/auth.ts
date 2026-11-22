/** Аутентификация (§3.2). Токен живёт только в памяти клиента. */
import {
  apiRequest,
  bootstrapSession,
  clearSession,
  getRefreshedUser,
  setAccessToken,
} from './client';
import type { AuthUser, ChangePasswordRequest, LoginRequest, LoginResponse } from './types';

/** AUTH-1: вход по логину и паролю, access-токен кладём в память. */
export async function login(input: LoginRequest): Promise<LoginResponse> {
  const response = await apiRequest<LoginResponse>('/auth/login', {
    method: 'POST',
    body: input,
    skipAuth: true,
    skipRefresh: true,
  });
  setAccessToken(response.access_token);
  return response;
}

/** AUTH-2: logout отзывает refresh-цепочку и чистит память клиента. */
export async function logout(): Promise<void> {
  try {
    await apiRequest<void>('/auth/logout', { method: 'POST' });
  } finally {
    clearSession();
  }
}

export function fetchMe(): Promise<AuthUser> {
  return apiRequest<AuthUser>('/auth/me');
}

/** AUTH-5: смена своего пароля (current + новый ≥ 8 символов). */
export function changeOwnPassword(body: ChangePasswordRequest): Promise<void> {
  return apiRequest<void>('/auth/password', { method: 'POST', body });
}

/**
 * Восстановление сессии при старте SPA: refresh-cookie → access-токен →
 * (при необходимости) `GET /auth/me`. Возвращает null, если сессии нет.
 */
export async function restoreSession(): Promise<AuthUser | null> {
  const session = await bootstrapSession();
  if (!session) return null;
  if (session.user) return session.user;
  try {
    return await fetchMe();
  } catch {
    clearSession();
    return null;
  }
}

export { getRefreshedUser };
