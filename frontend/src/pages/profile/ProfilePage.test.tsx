/**
 * `/profile` — учётная запись, push-уведомления и смена пароля (Ф9).
 *
 * Push-состояния проверяются на моках браузерного окружения (jsdom не умеет
 * service worker/PushManager/Notification): не поддерживается, запрещено,
 * успешная подписка, отключение, ошибка сервера, HTTPS-предупреждение.
 */
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import type { AuthUser } from '@/api/types';
import { db } from '@/mocks/db';
import { API } from '@/mocks/http';
import { server } from '@/mocks/server';
import { PUSH_STORAGE_KEY } from '@/push/pushClient';
import { PushProvider } from '@/push/PushProvider';
import { installPushEnvironment, type PushTestEnvironment } from '@/push/testing/pushEnv';
import {
  ADMIN_AUTH,
  USER_AUTH,
  authenticateAs,
  makeAuthValue,
  makeAuthUser,
  renderWithProviders,
} from '@/test/utils';
import { ProfilePage } from './ProfilePage';

let pushEnv: PushTestEnvironment | null = null;

function renderProfile(
  options: {
    user?: AuthUser;
    push?: Parameters<typeof installPushEnvironment>[0];
  } = {},
) {
  const user = options.user ?? USER_AUTH;
  authenticateAs(user.id);
  pushEnv = installPushEnvironment(options.push);
  return renderWithProviders(
    <PushProvider>
      <ProfilePage />
    </PushProvider>,
    { route: '/profile', auth: makeAuthValue({ user }) },
  );
}

afterEach(() => {
  pushEnv?.cleanup();
  pushEnv = null;
});

describe('ProfilePage: учётная запись', () => {
  beforeEach(() => authenticateAs(USER_AUTH.id));

  it('показывает ФИО, логин, роль и здание сотрудника', async () => {
    renderProfile();

    const account = await screen.findByTestId('profile-account');
    expect(account).toHaveTextContent('Иванов Иван Иванович');
    expect(account).toHaveTextContent('user');
    expect(within(account).getByTestId('profile-role')).toHaveTextContent('Сотрудник');
    expect(within(account).getByTestId('profile-building')).toHaveTextContent('Центральный офис');
    expect(screen.queryByTestId('profile-building-missing')).not.toBeInTheDocument();
  });

  it('содержит ссылку на личную статистику `/stats` (STATS-4)', async () => {
    renderProfile();

    const link = await screen.findByTestId('profile-stats-link');
    expect(link.closest('a')).toHaveAttribute('href', '/stats');
    expect(link).toHaveTextContent('Моя статистика');
  });

  it('у ADMIN показывает «Все здания» без привязки к зданию', async () => {
    renderProfile({ user: ADMIN_AUTH });

    const account = await screen.findByTestId('profile-account');
    expect(within(account).getByTestId('profile-role')).toHaveTextContent('Администратор');
    expect(within(account).getByTestId('profile-building')).toHaveTextContent('Все здания');
  });

  it('предупреждает сотрудника без здания (AUTH-9)', async () => {
    renderProfile({ user: makeAuthUser({ building_id: null, building_name: null }) });

    expect(await screen.findByTestId('profile-building-missing')).toHaveTextContent(
      'Здание не назначено',
    );
    expect(screen.getByTestId('profile-building')).toHaveTextContent('Не назначено');
  });
});

describe('ProfilePage: push-уведомления', () => {
  beforeEach(() => authenticateAs(USER_AUTH.id));

  it('показывает, что браузер не поддерживает push', async () => {
    renderProfile({ push: { supported: false } });

    expect(await screen.findByTestId('push-status')).toHaveTextContent(
      'Браузер не поддерживает service worker',
    );
    expect(screen.getByTestId('push-state-tag')).toHaveTextContent('не подписано');
    expect(screen.getByTestId('push-enable')).toBeDisabled();
  });

  it('при запрещённом разрешении показывает инструкцию и блокирует включение', async () => {
    renderProfile({ push: { permission: 'denied' } });

    expect(await screen.findByTestId('push-status')).toHaveTextContent('запрещены');
    expect(screen.getByTestId('push-state-tag')).toHaveTextContent('запрещено');
    expect(screen.getByTestId('push-denied-instructions')).toHaveTextContent('Chrome');
    expect(screen.getByTestId('push-denied-instructions')).toHaveTextContent('Firefox');
    expect(screen.getByTestId('push-enable')).toBeDisabled();
    // Разрешение повторно не запрашивается (NOTIF-5).
    expect(pushEnv?.requestPermission).not.toHaveBeenCalled();
  });

  it('подписка отправляет endpoint и ключи на сервер и сохраняет id', async () => {
    renderProfile({ push: { permission: 'default', requestPermissionResult: 'granted' } });

    const enableButton = await screen.findByTestId('push-enable');
    await waitFor(() => expect(enableButton).toBeEnabled());
    await userEvent.click(enableButton);

    expect(await screen.findByTestId('push-status')).toHaveTextContent('Подписка активна');

    // subscribe вызван с VAPID-ключом и userVisibleOnly (NOTIF-1).
    const subscribeCall = pushEnv?.subscribeCalls.at(0);
    expect(subscribeCall?.userVisibleOnly).toBe(true);
    expect(subscribeCall?.applicationServerKey).toBeInstanceOf(Uint8Array);
    expect((subscribeCall?.applicationServerKey as Uint8Array).length).toBeGreaterThan(0);

    // MOCK-сервер получил endpoint и ключи устройства.
    const saved = db.pushSubscriptions.find(
      (item) => item.endpoint === 'https://push.example.test/device-1',
    );
    expect(saved).toMatchObject({ keys: { p256dh: 'fake-p256dh', auth: 'fake-auth' } });
    expect(window.localStorage.getItem(PUSH_STORAGE_KEY)).toBe(String(saved?.id));
  });

  it('отключение удаляет подписку на сервере и очищает localStorage', async () => {
    renderProfile({
      push: { permission: 'granted', hasSubscription: true, storedSubscriptionId: 1 },
    });

    expect(await screen.findByTestId('push-status')).toHaveTextContent('Подписка активна');
    await userEvent.click(screen.getByTestId('push-disable'));

    await waitFor(() =>
      expect(screen.getByTestId('push-status')).toHaveTextContent('Разрешение выдано'),
    );
    // DELETE /push/subscriptions/1 деактивирует подписку в моке.
    expect(db.pushSubscriptions.find((item) => item.id === 1)?.is_active).toBe(false);
    expect(window.localStorage.getItem(PUSH_STORAGE_KEY)).toBeNull();
  });

  it('ошибка сервера переводит переключатель в состояние ошибки', async () => {
    server.use(
      http.post(`${API}/push/subscriptions`, () =>
        HttpResponse.json(
          { code: 'INTERNAL_ERROR', message: 'Сбой', details: null, request_id: 'mock-test' },
          { status: 500 },
        ),
      ),
    );

    renderProfile({ push: { permission: 'granted' } });
    const enableButton = await screen.findByTestId('push-enable');
    await waitFor(() => expect(enableButton).toBeEnabled());
    await userEvent.click(enableButton);

    expect(await screen.findByTestId('push-error')).toHaveTextContent('Ошибка сервера');
    expect(screen.getByTestId('push-status')).toHaveTextContent('Не удалось включить');
    expect(screen.getByTestId('push-state-tag')).toHaveTextContent('не подписано');
  });

  it('предупреждает, что push работает только по HTTPS', async () => {
    renderProfile({ push: { secureContext: false } });

    expect(await screen.findByTestId('push-insecure-hint')).toHaveTextContent(
      'работают только по HTTPS',
    );
    expect(screen.getByTestId('push-enable')).toBeDisabled();
  });

  it('тестовое уведомление недоступно без эндпоинта бэкенда и объясняет почему', async () => {
    renderProfile();

    const testButton = await screen.findByTestId('push-test-notification');
    expect(testButton).toBeDisabled();
    expect(screen.getByTestId('push-test-hint')).toHaveTextContent('Б8');
  });
});

describe('ProfilePage: смена пароля', () => {
  beforeEach(() => authenticateAs(USER_AUTH.id));

  it('валидирует длину нового пароля и совпадение подтверждения', async () => {
    renderProfile();

    const current = await screen.findByTestId('profile-current-password');
    await userEvent.type(current, 'user12345');
    await userEvent.type(screen.getByTestId('profile-new-password'), 'short');
    await userEvent.type(screen.getByTestId('profile-confirm-password'), 'other');
    await userEvent.click(screen.getByTestId('profile-password-submit'));

    expect(await screen.findByText('Новый пароль — не короче 8 символов')).toBeInTheDocument();
    expect(screen.getByText('Пароли не совпадают')).toBeInTheDocument();
    // До сервера запрос не дошёл.
    expect(db.users.find((user) => user.id === USER_AUTH.id)?.password).toBe('user12345');
  });

  it('меняет пароль при корректном текущем пароле', async () => {
    renderProfile();

    const current = await screen.findByTestId('profile-current-password');
    await userEvent.type(current, 'user12345');
    await userEvent.type(screen.getByTestId('profile-new-password'), 'newpass123');
    await userEvent.type(screen.getByTestId('profile-confirm-password'), 'newpass123');
    await userEvent.click(screen.getByTestId('profile-password-submit'));

    expect(await screen.findByTestId('profile-password-success')).toHaveTextContent(
      'Пароль обновлён',
    );
    expect(db.users.find((user) => user.id === USER_AUTH.id)?.password).toBe('newpass123');
  });
});
