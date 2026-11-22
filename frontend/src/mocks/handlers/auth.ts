/** MSW: аутентификация (§3.2, §3.13). */
import { http } from 'msw';
import type { ChangePasswordRequest, LoginRequest } from '@/api/types';
import {
  clearRefreshSession,
  currentRefreshUserId,
  findByLogin,
  guard,
  issueAccessToken,
  startRefreshSession,
} from '../auth';
import { db, findUser, toAdminUser } from '../db';
import { API, fail, json, noContent, readBody } from '../http';

export const authHandlers = [
  http.post(`${API}/auth/login`, async ({ request }) => {
    const body = await readBody<LoginRequest>(request);
    const login = typeof body.login === 'string' ? body.login : '';
    const password = typeof body.password === 'string' ? body.password : '';
    const user = findByLogin(login);

    // AUTH-1: неверные данные → 401 без раскрытия существования логина.
    if (!user || user.password !== password || !user.is_active) {
      return fail(401, 'INVALID_CREDENTIALS', 'Неверный логин или пароль');
    }

    user.last_login_at = new Date().toISOString();
    startRefreshSession(user.id);
    return json({
      access_token: issueAccessToken(user.id),
      token_type: 'Bearer',
      expires_in: 900,
      user: toAdminUser(user),
    });
  }),

  http.post(`${API}/auth/refresh`, () => {
    const userId = currentRefreshUserId();
    const user = userId === null ? undefined : findUser(userId);
    if (!user || !user.is_active) {
      clearRefreshSession();
      return fail(401, 'UNAUTHORIZED', 'Сессия истекла, войдите заново');
    }
    // AUTH-2: ротация refresh-цепочки; в моках cookie живёт до logout.
    return json({
      access_token: issueAccessToken(user.id),
      token_type: 'Bearer',
      expires_in: 900,
      user: toAdminUser(user),
    });
  }),

  http.post(`${API}/auth/logout`, () => {
    clearRefreshSession();
    return noContent();
  }),

  http.get(`${API}/auth/me`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    return json(toAdminUser(auth.user));
  }),

  http.post(`${API}/auth/password`, async ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;

    const body = await readBody<ChangePasswordRequest>(request);
    const current = typeof body.current_password === 'string' ? body.current_password : '';
    const next = typeof body.new_password === 'string' ? body.new_password : '';

    if (auth.user.password !== current) {
      return fail(400, 'INVALID_CREDENTIALS', 'Текущий пароль указан неверно');
    }
    if (next.length < 8) {
      return fail(422, 'VALIDATION_ERROR', 'Новый пароль должен быть не короче 8 символов', {
        new_password: 'минимум 8 символов',
      });
    }

    auth.user.password = next;
    auth.user.must_change_password = false;
    return noContent();
  }),

  http.get(`${API}/push/public-key`, ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    return json({
      public_key:
        'BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U',
    });
  }),

  http.post(`${API}/push/subscriptions`, async ({ request }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const body = (await request.json().catch(() => ({}))) as {
      endpoint?: string;
      keys?: { p256dh?: string; auth?: string };
      user_agent?: string;
    };
    if (!body.endpoint) return fail(422, 'VALIDATION_ERROR', 'Некорректная подписка push');

    const existing = db.pushSubscriptions.find(
      (subscription) => subscription.endpoint === body.endpoint,
    );
    if (existing) {
      existing.is_active = true;
      existing.user_id = auth.user.id;
      return json({
        id: existing.id,
        endpoint: existing.endpoint,
        created_at: existing.created_at,
        last_success_at: existing.last_success_at,
        is_active: existing.is_active,
      });
    }

    const subscription = {
      id: db.pushSubscriptions.reduce((max, item) => Math.max(max, item.id), 0) + 1,
      user_id: auth.user.id,
      endpoint: body.endpoint,
      keys: { p256dh: body.keys?.p256dh ?? '', auth: body.keys?.auth ?? '' },
      user_agent: body.user_agent ?? null,
      created_at: new Date().toISOString(),
      last_success_at: null,
      is_active: true,
    };
    db.pushSubscriptions.push(subscription);
    return json({
      id: subscription.id,
      endpoint: subscription.endpoint,
      created_at: subscription.created_at,
      last_success_at: subscription.last_success_at,
      is_active: subscription.is_active,
    });
  }),

  http.delete(`${API}/push/subscriptions/:id`, ({ request, params }) => {
    const auth = guard(request);
    if (!auth.ok) return auth.response;
    const id = Number(params.id);
    const subscription = db.pushSubscriptions.find((item) => item.id === id);
    if (!subscription || subscription.user_id !== auth.user.id) {
      return fail(404, 'NOT_FOUND', 'Подписка не найдена');
    }
    subscription.is_active = false;
    return noContent();
  }),
];
