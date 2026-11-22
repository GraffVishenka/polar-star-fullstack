/**
 * Общая настройка Vitest + jsdom: jest-dom, MSW-сервер, полифилы antd,
 * сброс мок-базы и состояния API-клиента между тестами.
 */
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterAll, afterEach, beforeAll } from 'vitest';
import { __resetApiClientForTests } from '@/api/client';
import { registerToastApi } from '@/app/toast';
import { clearRefreshSession } from '@/mocks/auth';
import { resetDb } from '@/mocks/db';
import { server } from '@/mocks/server';
import { ANON_BUILDING_COOKIE } from '@/hooks/useAnonBuilding';

// --- Полифилы, без которых antd падает в jsdom -----------------------------
if (!window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

class IntersectionObserverStub {
  readonly root = null;
  readonly rootMargin = '';
  readonly thresholds: number[] = [];
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
  takeRecords(): [] {
    return [];
  }
}

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
}
if (!globalThis.IntersectionObserver) {
  globalThis.IntersectionObserver =
    IntersectionObserverStub as unknown as typeof IntersectionObserver;
}
if (!window.scrollTo) {
  Object.defineProperty(window, 'scrollTo', { writable: true, value: () => {} });
}

beforeAll(() => {
  // Любой необработанный запрос — ошибка теста: MSW обязан покрывать контракт.
  server.listen({ onUnhandledRequest: 'error' });
});

afterEach(() => {
  cleanup();
  server.resetHandlers();
  resetDb();
  clearRefreshSession();
  __resetApiClientForTests();
  registerToastApi(null);
  document.cookie = `${ANON_BUILDING_COOKIE}=; Path=/; Max-Age=0`;
  window.localStorage.clear();
  window.sessionStorage.clear();
});

afterAll(() => {
  server.close();
});
