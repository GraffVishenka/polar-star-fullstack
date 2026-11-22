/**
 * Настройки и пользователи (Ф6/Ф11, ADMIN): deadline_days, статус, создание
 * пользователя. Навигация — по ссылкам SPA, чтобы не терять MSW-сессию.
 */
import { test, expect } from './fixtures';
import { login, pickOption } from './helpers';

test.describe('Настройки и пользователи', () => {
  test('ADMIN меняет срок дедлайна и подпись статуса', async ({ page }) => {
    await login(page, 'admin', 'admin12345');
    await page.locator('a[href="/admin/users"]').click();
    await page.locator('a[href="/admin/settings"]').click();
    await expect(page.getByTestId('settings-page')).toBeVisible();

    await page.getByTestId('settings-deadline-days').fill('5');
    await page.getByTestId('status-label-new').fill('Новая E2E');
    await page.getByTestId('settings-save').click();

    await expect(page.getByText('Настройки сохранены')).toBeVisible();
    await expect(page.getByTestId('settings-error')).toHaveCount(0);

    // Значения остались в форме после сохранения (SPA, без перезагрузки).
    await expect(page.getByTestId('settings-deadline-days')).toHaveValue('5');
    await expect(page.getByTestId('status-label-new')).toHaveValue('Новая E2E');
  });

  test('ADMIN создаёт пользователя в выбранном здании', async ({ page }) => {
    await login(page, 'admin', 'admin12345');
    await page.locator('a[href="/admin/users"]').click();
    await expect(page.getByTestId('admin-users-page')).toBeVisible();

    await page.getByTestId('user-create').click();
    await expect(page.getByTestId('user-login-input')).toBeVisible();

    await page.getByTestId('user-login-input').fill('e2e_user');
    await page.getByTestId('user-fullname-input').fill('E2E Пользователь');
    await pickOption(page, page.getByTestId('user-role-select'), 'Сотрудник');
    await pickOption(page, page.getByTestId('user-building-select'), 'Центральный офис');
    await page.getByTestId('user-password-input').fill('E2ePassw0rd!');
    await page.getByTestId('user-form-modal').getByRole('button', { name: 'Создать' }).click();

    await expect(page.getByTestId('user-login-input')).toBeHidden();
    await expect(page.getByText('Пользователь e2e_user создан')).toBeVisible();

    await page.getByTestId('users-search').fill('e2e_user');
    await expect(page.getByText('E2E Пользователь')).toBeVisible();
  });
});
