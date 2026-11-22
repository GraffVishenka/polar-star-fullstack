/**
 * Клиент Web Push (Ф9, NOTIF-1..NOTIF-5).
 *
 * Чистые браузерные помощники без React:
 *  - проверка поддержки (`serviceWorker` + `PushManager` + `Notification`);
 *  - регистрация service worker `public/sw.js`;
 *  - подписка `pushManager.subscribe({userVisibleOnly:true, applicationServerKey})`;
 *  - хранение id подписки в localStorage (чтобы отключить её на сервере).
 *
 * VAPID public key берётся с сервера (`GET /api/push/public-key`), а не из env,
 * чтобы ключ можно было менять без пересборки фронта.
 */
import type { PushSubscriptionRequest } from '@/api/types';

/** Ключ localStorage: id подписки на этом устройстве. */
export const PUSH_STORAGE_KEY = 'polar-star.push.subscriptionId';

/**
 * Scope push-service-worker.
 *
 * В dev MSW регистрирует `mockServiceWorker.js` на `/`; чтобы не перетирать его,
 * push-worker получает собственный scope `/push/`. В production (MSW выключен)
 * регистрируем на `/`, тогда клик по уведомлению может сфокусировать вкладку.
 */
export const PUSH_SERVICE_WORKER_URL = '/sw.js';
export const PUSH_SCOPE: string = import.meta.env.VITE_ENABLE_MSW === 'false' ? '/' : '/push/';

export interface PushSupportInfo {
  supported: boolean;
  /** Почему push недоступен — показываем в профиле. */
  reason: string | null;
}

/** NOTIF-1/NOTIF-5: без service worker, PushManager и Notification push не работает. */
export function detectPushSupport(): PushSupportInfo {
  if (typeof window === 'undefined') {
    return { supported: false, reason: 'Push-уведомления доступны только в браузере' };
  }
  if (!('serviceWorker' in navigator)) {
    return { supported: false, reason: 'Браузер не поддерживает service worker' };
  }
  if (typeof window.PushManager === 'undefined') {
    return { supported: false, reason: 'Браузер не поддерживает Push API' };
  }
  if (typeof window.Notification === 'undefined') {
    return { supported: false, reason: 'Браузер не поддерживает уведомления' };
  }
  return { supported: true, reason: null };
}

/**
 * Web Push требует HTTPS (NFR-2, NOTIF-1). Исключение — localhost/127.0.0.1,
 * где браузеры разрешают push без TLS.
 */
export function isPushSecureContext(): boolean {
  if (typeof window === 'undefined') return false;
  if (window.isSecureContext === false) return false;
  const { protocol, hostname } = window.location;
  return protocol === 'https:' || hostname === 'localhost' || hostname === '127.0.0.1';
}

/** `applicationServerKey` для `pushManager.subscribe` — base64url → Uint8Array. */
export function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = atob(base64);
  const bytes = new Uint8Array(new ArrayBuffer(raw.length));
  for (let index = 0; index < raw.length; index += 1) {
    bytes[index] = raw.charCodeAt(index);
  }
  return bytes;
}

/** Регистрирует (или обновляет) push service worker. */
export function registerPushServiceWorker(): Promise<ServiceWorkerRegistration> {
  return navigator.serviceWorker.register(PUSH_SERVICE_WORKER_URL, { scope: PUSH_SCOPE });
}

/** Уже зарегистрированный worker (без повторной регистрации на старте страницы). */
export async function findPushRegistration(): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator)) return null;
  try {
    return (await navigator.serviceWorker.getRegistration(PUSH_SCOPE)) ?? null;
  } catch {
    return null;
  }
}

/** Текущая подписка устройства, если она есть. */
export async function getCurrentPushSubscription(): Promise<PushSubscription | null> {
  const registration = await findPushRegistration();
  if (!registration) return null;
  try {
    return await registration.pushManager.getSubscription();
  } catch {
    return null;
  }
}

function bytesToBase64Url(bytes: ArrayBuffer | null): string {
  if (!bytes || bytes.byteLength === 0) return '';
  let binary = '';
  const view = new Uint8Array(bytes);
  for (let index = 0; index < view.length; index += 1) {
    binary += String.fromCharCode(view[index] as number);
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

/**
 * `PushSubscription` → тело `POST /api/push/subscriptions`.
 * `toJSON().keys` есть не во всех браузерах, поэтому оставлен fallback на `getKey`.
 */
export function readSubscriptionPayload(subscription: PushSubscription): PushSubscriptionRequest {
  const json = typeof subscription.toJSON === 'function' ? subscription.toJSON() : null;
  const p256dh = json?.keys?.p256dh ?? bytesToBase64Url(subscription.getKey?.('p256dh') ?? null);
  const auth = json?.keys?.auth ?? bytesToBase64Url(subscription.getKey?.('auth') ?? null);
  return {
    endpoint: subscription.endpoint,
    keys: { p256dh, auth },
    user_agent: typeof navigator !== 'undefined' ? navigator.userAgent : undefined,
  };
}

/** Id подписки на этом устройстве (для `DELETE /api/push/subscriptions/{id}`). */
export function readStoredSubscriptionId(): number | null {
  try {
    const raw = window.localStorage.getItem(PUSH_STORAGE_KEY);
    if (!raw) return null;
    const parsed = Number(raw);
    return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
  } catch {
    return null;
  }
}

export function storeSubscriptionId(id: number): void {
  try {
    window.localStorage.setItem(PUSH_STORAGE_KEY, String(id));
  } catch {
    // Приватный режим может запрещать localStorage — подписка всё равно работает.
  }
}

export function clearStoredSubscriptionId(): void {
  try {
    window.localStorage.removeItem(PUSH_STORAGE_KEY);
  } catch {
    // ignore
  }
}
