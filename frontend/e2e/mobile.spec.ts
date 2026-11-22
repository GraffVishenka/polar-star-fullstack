/**
 * Адаптив 375px (Ф11, NFR-8): публичные страницы без горизонтального скролла
 * страницы (внутренний скролл таблиц/дерева допустим), ключевые действия доступны.
 */
import { test, expect, type Page } from './fixtures';

async function expectNoPageOverflow(page: Page, label: string): Promise<void> {
  const metrics = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  expect(
    metrics.scrollWidth,
    `${label}: горизонтальный скролл страницы (${metrics.scrollWidth} > ${metrics.innerWidth})`,
  ).toBeLessThanOrEqual(metrics.innerWidth + 1);
}

test.describe('Адаптив 375px', () => {
  test('публичные страницы помещаются в ширину экрана', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByTestId('building-select-page')).toBeVisible();
    await expectNoPageOverflow(page, '/');

    await page.getByTestId('building-card-1').click();
    await expect(page.getByTestId('building-dashboard-page')).toBeVisible();
    await expectNoPageOverflow(page, '/b/1');

    await page.goto('/t/demo-token-1');
    await expect(page.getByTestId('public-ticket-page')).toBeVisible();
    await expectNoPageOverflow(page, '/t/{token}');

    await page.goto('/public/kb');
    await expect(page.getByTestId('public-kb-page')).toBeVisible();
    await expectNoPageOverflow(page, '/public/kb');
  });

  test('карточки зданий доступны на 375px и форма создания открывается', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByTestId('building-card-1')).toBeVisible();

    const first = await page.getByTestId('building-card-1').boundingBox();
    expect(first).toBeTruthy();
    // Карточки не выходят за пределы экрана (сетка antd сама выбирает колонки).
    expect(first!.width).toBeLessThanOrEqual(375);

    await page.getByTestId('building-card-1').click();
    await expect(page.getByTestId('building-dashboard-page')).toBeVisible();
    await page.getByTestId('create-ticket-button').click();
    await expect(page.getByTestId('public-create-submit')).toBeVisible();
    const modal = await page.locator('.ant-modal-content').boundingBox();
    expect(modal!.width).toBeLessThanOrEqual(375);
  });
});
