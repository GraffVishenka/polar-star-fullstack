/** MSW в браузере (только dev): `pnpm dev` работает без бэкенда. */
import { setupWorker } from 'msw/browser';
import { handlers } from './handlers';

export const worker = setupWorker(...handlers);

export async function startMockServiceWorker(): Promise<void> {
  await worker.start({
    // Неавторизованные запросы к статике и HMR не должны ругаться в консоли.
    onUnhandledRequest: 'bypass',
    serviceWorker: { url: '/mockServiceWorker.js' },
    quiet: false,
  });
}
