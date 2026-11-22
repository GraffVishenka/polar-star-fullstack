/**
 * Playwright E2E (Ф11).
 *
 * Запуск: `pnpm e2e` (или `pnpm e2e:ui`). Dev-сервер поднимается автоматически
 * с `VITE_ENABLE_MSW=true` — тесты идут против MSW, реальный API не нужен.
 *
 * Проекты:
 *  - `chromium` — десктоп 1440×900, все сценарии;
 *  - `mobile-375` — 375×812, только `mobile.spec.ts` (адаптив).
 *
 * `trace: on-first-retry`, скриншот/видео — при падении.
 */
import { defineConfig, devices } from '@playwright/test';

const PORT = Number(process.env.E2E_PORT ?? 5173);
const BASE_URL = `http://127.0.0.1:${PORT}`;

/**
 * Локально на этой машине Application Control блокирует скачанный Chromium
 * (`spawn UNKNOWN`), поэтому по умолчанию используем установленный Edge.
 * В CI — обычный Playwright Chromium. Переопределение: `E2E_CHANNEL=chromium`.
 */
const browserChannel = process.env.E2E_CHANNEL ?? (process.env.CI ? undefined : 'msedge');
const channelOverride = browserChannel ? { channel: browserChannel } : {};

export default defineConfig({
  testDir: './e2e',
  outputDir: './test-results',
  // MSW-база живёт в памяти страницы; параллельные контексты независимы, но
  // dev-сервер и Chromium стабильнее с одним воркером.
  fullyParallel: false,
  workers: 1,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 2 : 0,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  reporter: [['list'], ['html', { open: 'never', outputFolder: 'playwright-report' }]],
  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    // Видео отключено: ffmpeg из Playwright блокируется Application Control на
    // машинах разработки; трейс и скриншоты дают достаточно диагностики.
    video: 'off',
    locale: 'ru-RU',
    timezoneId: 'Asia/Yekaterinburg',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        ...channelOverride,
        viewport: { width: 1440, height: 900 },
      },
      testIgnore: /mobile\.spec\.ts/,
    },
    {
      name: 'mobile-375',
      use: {
        ...devices['Desktop Chrome'],
        ...channelOverride,
        viewport: { width: 375, height: 812 },
        hasTouch: true,
      },
      testMatch: /mobile\.spec\.ts/,
    },
  ],
  webServer: {
    // process.execPath — Node, которым запущен Playwright (в CI и локально он есть).
    command: `"${process.execPath}" node_modules/vite/bin/vite.js --host 127.0.0.1 --port ${PORT} --strictPort`,
    url: BASE_URL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    env: { VITE_ENABLE_MSW: 'true' },
  },
});
