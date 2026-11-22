/**
 * Оборудование (Ф4/Ф7/Ф11): перемещение между складами и журнал, массовое
 * перемещение, ведомость помещения (Excel).
 *
 * Кнопка «Ведомость помещения» в карточке оборудования пока — заглушка
 * (`inventory-stub`): `InventoryExportButton` из Ф8 ждёт интеграции fe-3/lead.
 * Поэтому скачивание проверяем на том же эндпоинте через fetch в контексте
 * страницы (MSW), с проверкой `Content-Disposition` и непустого blob.
 */
import { test, expect } from './fixtures';
import { login, pickOption } from './helpers';

test.describe('Оборудование', () => {
  test('перемещает единицу между складами и показывает журнал', async ({ page }) => {
    await login(page, 'admin', 'admin12345');
    await page.locator('a[href="/equipment"]').first().click();
    await expect(page.getByTestId('equipment-page')).toBeVisible();

    await page.getByTestId('open-equipment-3').click();
    await expect(page.getByTestId('equipment-drawer')).toBeVisible();

    await page.getByTestId('open-move-dialog').click();
    await expect(page.getByTestId('move-dialog')).toBeVisible();

    // Целевое здание и склад: «Центральный офис» → «Склад ИТ».
    await pickOption(page, page.getByTestId('move-building'), 'Центральный офис');
    await pickOption(page, page.getByTestId('move-warehouse'), 'Склад ИТ');
    await page.getByTestId('move-comment').fill('E2E: перемещение на склад ИТ');
    await page.getByTestId('move-submit').click();

    await expect(page.getByTestId('move-dialog')).toBeHidden();
    await expect(page.getByTestId('equipment-location')).toContainText('Склад ИТ');
    await expect(page.getByTestId('equipment-journal')).toBeVisible();
    await expect(page.getByTestId('movement-list')).toContainText('Склад ИТ');
  });

  test('массово перемещает две единицы и показывает результат', async ({ page }) => {
    await login(page, 'admin', 'admin12345');
    await page.locator('a[href="/equipment"]').first().click();
    await expect(page.getByTestId('equipment-page')).toBeVisible();

    // Кликаем по чекбоксам строк (не по «Select all» в заголовке).
    const rows = page.locator('.ant-table-tbody tr.ant-table-row');
    await expect(rows.first()).toBeVisible();
    await rows.nth(0).locator('.ant-checkbox-wrapper').click();
    await rows.nth(1).locator('.ant-checkbox-wrapper').click();
    await expect(page.getByTestId('selection-bar')).toBeVisible();

    await page.getByTestId('bulk-move-open').click();
    await expect(page.getByTestId('bulk-dialog')).toBeVisible();

    await pickOption(page, page.getByTestId('move-building'), 'Центральный офис');
    await pickOption(page, page.getByTestId('move-warehouse'), 'Кабинет 101');
    await page.getByTestId('bulk-next').click();
    await expect(page.getByTestId('bulk-confirm-summary')).toBeVisible();
    await page.getByTestId('bulk-confirm').click();

    await expect(page.getByTestId('bulk-result')).toBeVisible();
    await page.getByTestId('bulk-done').click();
    await expect(page.getByTestId('bulk-dialog')).toBeHidden();
  });

  test('ведомость помещения отдаётся как xlsx-attachment (EQ-8)', async ({ page }) => {
    await login(page, 'admin', 'admin12345');

    const info = await page.evaluate(async () => {
      const response = await fetch('/api/warehouses/1/inventory.xlsx', {
        headers: { Authorization: 'Bearer mock-access-token-1' },
      });
      const blob = await response.blob();
      return {
        status: response.status,
        disposition: response.headers.get('content-disposition'),
        size: blob.size,
      };
    });

    expect(info.status).toBe(200);
    expect(info.disposition).toContain('inventory_1.xlsx');
    expect(info.size).toBeGreaterThan(0);
  });
});
