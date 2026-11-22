/**
 * Чистые помощники push-клиента (Ф9): конвертация VAPID-ключа, определение
 * поддержки/HTTPS, тело подписки и хранение id в localStorage.
 */
import { afterEach, describe, expect, it } from 'vitest';
import {
  PUSH_STORAGE_KEY,
  clearStoredSubscriptionId,
  detectPushSupport,
  getCurrentPushSubscription,
  isPushSecureContext,
  readStoredSubscriptionId,
  readSubscriptionPayload,
  storeSubscriptionId,
  urlBase64ToUint8Array,
} from './pushClient';
import { installPushEnvironment, type PushTestEnvironment } from './testing/pushEnv';

let pushEnv: PushTestEnvironment | null = null;

afterEach(() => {
  pushEnv?.cleanup();
  pushEnv = null;
});

describe('pushClient', () => {
  it('переводит VAPID-ключ из base64url в Uint8Array', () => {
    expect(Array.from(urlBase64ToUint8Array('AQAB'))).toEqual([1, 0, 1]);
    // base64url: `-` и `_` вместо `+` и `/`.
    expect(Array.from(urlBase64ToUint8Array('-_8'))).toEqual([251, 255]);
  });

  it('без service worker/PushManager/Notification сообщает об отсутствии поддержки', () => {
    const support = detectPushSupport();
    expect(support.supported).toBe(false);
    expect(support.reason).toMatch(/service worker/);
  });

  it('определяет поддержку и подписку при установленном окружении', async () => {
    pushEnv = installPushEnvironment({ hasSubscription: true });

    expect(detectPushSupport()).toEqual({ supported: true, reason: null });
    const subscription = await getCurrentPushSubscription();
    expect(subscription?.endpoint).toBe('https://push.example.test/device-1');
  });

  it('считает localhost защищённым контекстом, а явный insecure — нет', () => {
    expect(isPushSecureContext()).toBe(true);

    pushEnv = installPushEnvironment({ secureContext: false });
    expect(isPushSecureContext()).toBe(false);
  });

  it('собирает тело подписки из toJSON с keys и user_agent', () => {
    pushEnv = installPushEnvironment({ hasSubscription: true });
    const subscription = pushEnv.getSubscription();
    expect(subscription).not.toBeNull();

    const payload = readSubscriptionPayload(subscription as unknown as PushSubscription);
    expect(payload.endpoint).toBe('https://push.example.test/device-1');
    expect(payload.keys).toEqual({ p256dh: 'fake-p256dh', auth: 'fake-auth' });
    expect(typeof payload.user_agent).toBe('string');
  });

  it('хранит и очищает id подписки устройства', () => {
    expect(readStoredSubscriptionId()).toBeNull();

    storeSubscriptionId(7);
    expect(window.localStorage.getItem(PUSH_STORAGE_KEY)).toBe('7');
    expect(readStoredSubscriptionId()).toBe(7);

    clearStoredSubscriptionId();
    expect(readStoredSubscriptionId()).toBeNull();
  });
});
