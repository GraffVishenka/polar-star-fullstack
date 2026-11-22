/**
 * `public/sw.js` — обработчики push и notificationclick (Ф9, NOTIF-4).
 *
 * Service worker нельзя импортировать как модуль, поэтому тест читает файл и
 * выполняет его в песочнице с подменённым `self`: проверяем формат payload
 * (title/body/url/tag/icon, лимиты), фокус существующей вкладки и fallback
 * `clients.openWindow`.
 */
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it, vi } from 'vitest';

type PushHandler = (event: Record<string, unknown>) => void;

interface LoadedWorker {
  handlers: Record<string, PushHandler>;
  showNotification: ReturnType<typeof vi.fn>;
  openWindow: ReturnType<typeof vi.fn>;
  matchAll: ReturnType<typeof vi.fn>;
}

function loadServiceWorker(): LoadedWorker {
  const handlers: Record<string, PushHandler> = {};
  const showNotification = vi.fn(async () => undefined);
  const openWindow = vi.fn(async () => null);
  const matchAll = vi.fn(async () => [] as unknown[]);

  const selfStub = {
    addEventListener: (type: string, handler: PushHandler) => {
      handlers[type] = handler;
    },
    registration: { showNotification },
    clients: { matchAll, openWindow },
    skipWaiting: vi.fn(),
    location: { origin: 'http://localhost:5173' },
  };

  const source = readFileSync(resolve(process.cwd(), 'public/sw.js'), 'utf8');
  // Выполняем worker в изоляции: он общается с браузером только через `self`.
  new Function('self', source)(selfStub);

  return { handlers, showNotification, openWindow, matchAll };
}

function runPush(worker: LoadedWorker, payload: unknown, asText = false): Promise<unknown> {
  let settled: Promise<unknown> | undefined;
  worker.handlers.push?.({
    data: asText
      ? {
          json: () => {
            throw new Error('not json');
          },
          text: () => String(payload),
        }
      : { json: () => payload },
    waitUntil: (promise: Promise<unknown>) => {
      settled = promise;
    },
  });
  return settled ?? Promise.resolve();
}

function runClick(worker: LoadedWorker, url?: string): Promise<unknown> {
  let settled: Promise<unknown> | undefined;
  worker.handlers.notificationclick?.({
    notification: { close: vi.fn(), data: url ? { url } : {} },
    waitUntil: (promise: Promise<unknown>) => {
      settled = promise;
    },
  });
  return settled ?? Promise.resolve();
}

describe('service worker: push', () => {
  it('показывает уведомление с title/body/url/tag/icon (NOTIF-4)', async () => {
    const worker = loadServiceWorker();

    await runPush(worker, {
      title: 'Новая заявка',
      body: 'Центральный офис · каб. 204',
      url: '/tickets/12',
      tag: 'ticket-12',
      icon: '/icons/ticket.png',
    });

    expect(worker.showNotification).toHaveBeenCalledWith(
      'Новая заявка',
      expect.objectContaining({
        body: 'Центральный офис · каб. 204',
        tag: 'ticket-12',
        icon: '/icons/ticket.png',
        data: { url: '/tickets/12' },
      }),
    );
  });

  it('обрезает body до 120 символов и подставляет заголовок по умолчанию', async () => {
    const worker = loadServiceWorker();

    await runPush(worker, { body: 'я'.repeat(300) });

    const [title, options] = worker.showNotification.mock.calls[0] as [string, { body: string }];
    expect(title).toBe('Сервисный центр');
    expect(options.body).toHaveLength(120);
    expect(options.body).toBe('я'.repeat(120));
  });

  it('принимает текстовый payload (без JSON) и ведёт на /tickets по умолчанию', async () => {
    const worker = loadServiceWorker();

    await runPush(worker, 'Просто текст уведомления', true);

    expect(worker.showNotification).toHaveBeenCalledWith(
      'Сервисный центр',
      expect.objectContaining({ body: 'Просто текст уведомления', data: { url: '/tickets' } }),
    );
  });
});

describe('service worker: notificationclick', () => {
  it('фокусирует уже открытую вкладку с нужным адресом', async () => {
    const worker = loadServiceWorker();
    const focus = vi.fn(async () => undefined);
    worker.matchAll.mockResolvedValueOnce([
      { url: 'http://localhost:5173/tickets/12', focus, navigate: vi.fn() },
    ]);

    await runClick(worker, '/tickets/12');

    expect(focus).toHaveBeenCalledTimes(1);
    expect(worker.openWindow).not.toHaveBeenCalled();
  });

  it('фокусирует вкладку приложения и переводит её на нужный экран', async () => {
    const worker = loadServiceWorker();
    const navigate = vi.fn(async () => null);
    const focus = vi.fn(async () => ({ navigate }));
    worker.matchAll.mockResolvedValueOnce([
      { url: 'http://localhost:5173/tickets', focus, navigate },
    ]);

    await runClick(worker, '/tickets/42');

    expect(navigate).toHaveBeenCalledWith('http://localhost:5173/tickets/42');
    expect(worker.openWindow).not.toHaveBeenCalled();
  });

  it('открывает новую вкладку, если приложение не открыто', async () => {
    const worker = loadServiceWorker();
    worker.matchAll.mockResolvedValueOnce([]);

    await runClick(worker, '/tickets/42');

    expect(worker.openWindow).toHaveBeenCalledWith('http://localhost:5173/tickets/42');
  });
});
