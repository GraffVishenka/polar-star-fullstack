/**
 * axe-core на ключевых авторизованных страницах (Ф11): без critical-нарушений.
 * Публичные страницы проверяются в public.spec.ts.
 */
import { test, expect } from './fixtures';
import { expectNoCriticalA11y } from './a11y';
import { login } from './helpers';

test.describe('Доступность (axe)', () => {
  test('вход, доска заявок и личная статистика USER', async ({ page }) => {
    await page.goto('/login');
    await expect(page.getByTestId('login-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/login');

    await login(page, 'user', 'user12345');
    await expectNoCriticalA11y(page, '/tickets');

    await page.locator('a[href="/stats"]').first().click();
    await expect(page.getByTestId('stats-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/stats');
  });

  test('админская статистика и настройки', async ({ page }) => {
    await login(page, 'admin', 'admin12345');

    await page.locator('a[href="/admin/users"]').click();
    await page.locator('a[href="/admin/stats"]').click();
    await expect(page.getByTestId('admin-stats-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/admin/stats');

    await page.locator('a[href="/admin/settings"]').click();
    await expect(page.getByTestId('settings-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/admin/settings');
  });
});
