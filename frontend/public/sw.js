/*
 * Push service worker (Ф9, NOTIF-4).
 *
 * Назначение: показывать уведомления, присланные Web Push, и открывать нужный
 * экран по клику. Файл лежит в `public/`, поэтому обновляется вместе с фронтом
 * и не кэширует API: обработчика `fetch` здесь нет вообще — SPA и запросы к
 * `/api` идут мимо service worker (не ломаем ни MSW, ни nginx).
 *
 * Payload (NOTIF-4): `{title, body, url, tag, icon}`; title ≤ 60, body ≤ 120.
 * Scope регистрации выбирает клиент (`src/push/pushClient.ts`): в dev с MSW —
 * `/push/`, чтобы не перетирать mockServiceWorker; в production — `/`.
 */

/* eslint-disable */

const DEFAULT_TITLE = 'Сервисный центр';
const DEFAULT_URL = '/tickets';
const DEFAULT_ICON = '/favicon.svg';
const MAX_TITLE = 60;
const MAX_BODY = 120;

function clampString(value, maxLength) {
  return typeof value === 'string' ? value.slice(0, maxLength) : '';
}

function readPayload(event) {
  if (!event.data) return {};
  try {
    const parsed = event.data.json();
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    // Текстовая отправка (например, из тестового пуша) — показываем как body.
    try {
      return { body: event.data.text() };
    } catch {
      return {};
    }
  }
}

self.addEventListener('install', () => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('push', (event) => {
  const payload = readPayload(event);
  const title = clampString(payload.title, MAX_TITLE) || DEFAULT_TITLE;
  const body = clampString(payload.body, MAX_BODY);
  const url = typeof payload.url === 'string' && payload.url ? payload.url : DEFAULT_URL;
  const tag = typeof payload.tag === 'string' && payload.tag ? payload.tag : undefined;
  const icon = typeof payload.icon === 'string' && payload.icon ? payload.icon : DEFAULT_ICON;

  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      icon,
      badge: DEFAULT_ICON,
      tag,
      // Одинаковые tag схлопывают уведомления (например, напоминания по заявке).
      renotify: Boolean(tag),
      data: { url },
    }),
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const data = event.notification.data || {};
  const targetUrl = new URL(
    typeof data.url === 'string' && data.url ? data.url : DEFAULT_URL,
    self.location.origin,
  ).href;

  event.waitUntil(
    (async () => {
      const clientList = await self.clients.matchAll({
        type: 'window',
        includeUncontrolled: true,
      });

      // 1. Уже открытая нужная вкладка — фокусируем её.
      for (const client of clientList) {
        if (client.url === targetUrl) {
          await client.focus();
          return;
        }
      }

      // 2. Любая вкладка приложения — фокусируем и переводим на нужный экран.
      const sameOrigin = clientList.find((client) => client.url.startsWith(self.location.origin));
      if (sameOrigin && typeof sameOrigin.navigate === 'function') {
        const focused = await sameOrigin.focus();
        await focused.navigate(targetUrl);
        return;
      }

      // 3. Открываем новую вкладку.
      await self.clients.openWindow(targetUrl);
    })(),
  );
});
