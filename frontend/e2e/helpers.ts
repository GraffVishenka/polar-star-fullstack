/**
 * Общие шаги E2E: вход, выбор здания, выбор опции antd Select, ожидание скачивания.
 */
import { stat } from 'node:fs/promises';
import { expect, type Locator, type Page } from '@playwright/test';

export const ADMIN_LOGIN = 'admin';
export const ADMIN_PASSWORD = 'admin12345';
export const USER_LOGIN = 'user';
export const USER_PASSWORD = 'user12345';

/** Вход через реальную форму; после успеха — доска заявок. */
export async function login(
  page: Page,
  loginName: string = USER_LOGIN,
  password: string = USER_PASSWORD,
): Promise<void> {
  await page.goto('/login');
  await page.getByTestId('login-input').fill(loginName);
  await page.getByTestId('password-input').fill(password);
  await page.getByTestId('login-submit').click();
  await expect(page.getByTestId('tickets-page')).toBeVisible({ timeout: 20_000 });
}

/** Выбор опции в antd Select по тексту (видимая опция, не a11y-дубль). */
export async function pickOption(page: Page, select: Locator, text: string): Promise<void> {
  await select.click();
  const option = page
    .locator('.ant-select-dropdown:not(.ant-select-dropdown-hidden) .ant-select-item-option')
    .filter({ hasText: text })
    .first();
  await expect(option).toBeVisible();
  await option.click();
}

/** Выбор здания на `/`: следующий заход на `/` уже редиректит по cookie. */
export async function chooseBuilding(page: Page, buildingId: number): Promise<void> {
  await page.goto('/?change=1');
  await expect(page.getByTestId('building-select-page')).toBeVisible();
  await page.getByTestId(`building-card-${buildingId}`).click();
  await expect(page.getByTestId('building-dashboard-page')).toBeVisible();
}

/** Открывает карточку заявки на доске (drawer). */
export async function openTicket(page: Page, ticketId: number): Promise<void> {
  await page.getByTestId(`ticket-card-${ticketId}`).click();
  await expect(page.getByTestId('ticket-drawer-content')).toBeVisible();
}

/**
 * Ждёт скачивание, проверяет, что файл не пустой, и возвращает имя из
 * `Content-Disposition`. Ошибка (например, 500) не создаёт файл — тест упадёт
 * по таймауту события download, что и требуется по DOC-3.
 */
export async function readDownload(
  page: Page,
  action: () => Promise<void>,
): Promise<{ filename: string; size: number }> {
  const [download] = await Promise.all([page.waitForEvent('download'), action()]);
  const path = await download.path();
  expect(path, 'Playwright не сохранил файл скачивания').toBeTruthy();
  const info = await stat(path!);
  expect(info.size, 'Файл пустой').toBeGreaterThan(0);
  return { filename: download.suggestedFilename(), size: info.size };
}

/** Номер заявки из ссылки/текста `/t/{token}`. */
export function extractTokenPath(text: string): string {
  const match = /\/t\/[A-Za-z0-9_-]+/.exec(text);
  if (!match) throw new Error(`Не нашли ссылку /t/{token} в «${text}»`);
  return match[0];
}
