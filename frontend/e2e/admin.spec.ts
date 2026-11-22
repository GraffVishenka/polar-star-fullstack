/**
 * ADMIN (Ф2/Ф11): дедлайн, смена здания заявки со сбросом исполнителя (TICKET-9),
 * удаление в корзину и восстановление (TRASH).
 */
import { test, expect } from './fixtures';
import { login, openTicket, pickOption } from './helpers';

test.describe('Администратор', () => {
  test('меняет дедлайн, здание со сбросом исполнителя и восстанавливает заявку из корзины', async ({
    page,
  }) => {
    await login(page, 'admin', 'admin12345');
    await openTicket(page, 2);

    // Дедлайн (поле ADMIN): 31.12.2026.
    const deadlineInput = page.getByTestId('drawer-deadline');
    await deadlineInput.fill('31.12.2026');
    await deadlineInput.press('Enter');
    await expect(deadlineInput).toHaveValue('31.12.2026');

    // Смена здания: исполнитель из старого здания снимается автоматически.
    await pickOption(page, page.getByTestId('drawer-building-select'), 'Производственный корпус');
    const changeBuildingDialog = page.getByRole('dialog', { name: 'Сменить здание заявки?' });
    await expect(changeBuildingDialog).toBeVisible();
    await expect(changeBuildingDialog.getByTestId('confirm-dialog-warning')).toContainText(
      'будет снят',
    );
    await changeBuildingDialog.getByTestId('confirm-dialog-ok').click();
    await expect(page.getByTestId('executor-reset-notice')).toBeVisible();

    // Удаление в корзину и восстановление (TRASH-2).
    await page.getByTestId('delete-ticket').click();
    const deleteDialog = page.getByRole('dialog', { name: 'Удалить заявку №2 в корзину?' });
    await expect(deleteDialog).toBeVisible();
    await deleteDialog.getByTestId('confirm-dialog-ok').click();
    await expect(page.getByTestId('ticket-drawer')).toBeHidden();

    await page.locator('a[href="/admin/users"]').click();
    await page.locator('a[href="/admin/trash"]').click();
    await expect(page.getByTestId('admin-trash-page')).toBeVisible();
    const restore = page.getByTestId('trash-restore-ticket-2');
    await expect(restore).toBeVisible();
    await restore.click();
    const restoreDialog = page.getByRole('dialog', { name: /Восстановить/ });
    await expect(restoreDialog).toBeVisible();
    await restoreDialog.getByTestId('confirm-dialog-ok').click();
    await expect(restore).toHaveCount(0);
  });
});
