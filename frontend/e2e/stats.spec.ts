/**
 * Статистика (Ф10/Ф11): личная и админская, смена периода, Excel.
 * Навигация — по ссылкам SPA (MSW-сессия живёт в памяти страницы).
 */
import { test, expect } from './fixtures';
import { login, pickOption, readDownload } from './helpers';

test.describe('Статистика', () => {
  test('USER смотрит личную статистику, меняет период и качает Excel', async ({ page }) => {
    await login(page, 'user', 'user12345');
    await page.locator('a[href="/stats"]').first().click();
    await expect(page.getByTestId('stats-page')).toBeVisible();

    await expect(page.getByTestId('metric-value-total_done')).toContainText(/\d/);
    await expect(page.getByTestId('stats-metrics')).toBeVisible();

    const quarterResponse = page.waitForResponse(
      (response) => response.url().includes('/api/stats/me?') && response.ok(),
    );
    await page.getByTestId('stats-period').getByText('Квартал').click();
    await quarterResponse;
    await expect(page.getByTestId('stats-period')).toContainText('Квартал');

    const { filename } = await readDownload(page, () => page.getByTestId('export-button').click());
    expect(filename).toMatch(/^stat_.*\.xlsx$/);
  });

  test('ADMIN видит сводку, фильтрует по сотруднику/зданию и качает Excel', async ({ page }) => {
    await login(page, 'admin', 'admin12345');
    await page.locator('a[href="/admin/users"]').click();
    await page.locator('a[href="/admin/stats"]').click();
    await expect(page.getByTestId('admin-stats-page')).toBeVisible();

    // Агрегатный режим: сводка по сотрудникам из `employees[]`.
    await expect(page.getByTestId('employee-summary')).toBeVisible();
    await expect(page.getByTestId('admin-stats-user')).toBeVisible();

    await pickOption(page, page.getByTestId('admin-stats-user'), 'Иванов');
    await expect(page.getByTestId('metric-value-total_done')).toContainText(/\d/);
    await expect(page.getByTestId('employee-summary')).toHaveCount(0);

    const { filename } = await readDownload(page, () => page.getByTestId('export-button').click());
    expect(filename).toMatch(/^stat_.*\.xlsx$/);
  });
});
