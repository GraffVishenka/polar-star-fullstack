/**
 * USER (Ф2/Ф11): видимость только своего здания (TICKET-7), «Назначить себя»,
 * комментарий с вложением, смена статуса.
 */
import { test, expect } from './fixtures';
import { login, openTicket } from './helpers';

test.describe('Сотрудник (USER)', () => {
  test('видит только заявки своего здания; чужая заявка — 404 на API', async ({ page }) => {
    await login(page, 'user', 'user12345');

    await expect(page.getByTestId('ticket-card-1')).toBeVisible();
    // Заявки других зданий не попадают на доску USER (TICKET-7).
    await expect(page.getByTestId('ticket-card-5')).toHaveCount(0);
    await expect(page.getByTestId('building-select')).toContainText('Центральный офис');

    // Серверная проверка: своя заявка доступна, чужая — 404 (без утечки данных).
    const statuses = await page.evaluate(async () => {
      const headers = { Authorization: 'Bearer mock-access-token-2' };
      const own = await fetch('/api/tickets/1', { headers });
      const foreign = await fetch('/api/tickets/5', { headers });
      return { own: own.status, foreign: foreign.status };
    });
    expect(statuses.own).toBe(200);
    expect(statuses.foreign).toBe(404);
  });

  test('назначает себя, добавляет комментарий с фото и меняет статус', async ({ page }) => {
    await login(page, 'user', 'user12345');
    await openTicket(page, 1);

    // «Назначить себя» доступно, пока исполнитель не назначен.
    const assignSelf = page.getByTestId('assign-self');
    if (await assignSelf.isVisible().catch(() => false)) {
      await assignSelf.click();
      await expect(page.getByTestId('drawer-executor-name')).toContainText('Иванов');
    }

    // Комментарий с вложением: загрузка идёт через MSW multipart.
    await page.getByTestId('file-uploader-input').setInputFiles('e2e/fixtures/photo.png');
    await expect(page.getByTestId(/^file-uploader-item-/)).toBeVisible();
    await page.getByTestId('comment-input').fill('E2E: комментарий с фото');
    await page.getByTestId('comment-submit').click();
    await expect(page.getByText('E2E: комментарий с фото')).toBeVisible();

    // Смена статуса: «Новая» → «В работе» (TICKET-12).
    await page.getByTestId('drawer-status').getByText('В работе').click();
    await expect(page.getByTestId('drawer-status')).toContainText('В работе');
    await expect(page.getByTestId('ticket-card-1')).toHaveAttribute('data-status', 'in_progress');
  });
});
