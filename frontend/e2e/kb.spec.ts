/**
 * База знаний (Ф3/Ф7/Ф11): создание подраздела, вставка изображения между
 * абзацами, DnD-перемещение (dnd-kit). Скрытая секция проверяется в public.spec.
 */
import { test, expect } from './fixtures';
import { login } from './helpers';
import type { Page } from '@playwright/test';

async function createNode(page: Page, parent: 'root' | 'child', title: string) {
  await page.getByTestId(parent === 'root' ? 'kb-add-root' : 'kb-add-subsection').click();
  await expect(page.getByTestId('kb-node-title-input')).toBeVisible();
  await page.getByTestId('kb-node-title-input').fill(title);
  await page.locator('.ant-modal-content').getByRole('button', { name: 'Создать' }).click();
  await expect(page.getByTestId('kb-node-title-input')).toBeHidden();
  await expect(page).toHaveURL(/\/kb\/\d+/);
  return Number(/\/kb\/(\d+)/.exec(page.url())![1]);
}

test.describe('База знаний', () => {
  test('создаёт подраздел и вставляет изображение между абзацами', async ({ page }) => {
    await login(page, 'admin', 'admin12345');
    await page.locator('a[href="/kb"]').first().click();
    await expect(page.getByTestId('kb-page')).toBeVisible();

    const rootId = await createNode(page, 'root', 'E2E раздел');
    expect(rootId).toBeGreaterThan(0);
    const childId = await createNode(page, 'child', 'E2E подраздел');
    expect(childId).toBeGreaterThan(0);

    await page.getByTestId('kb-edit').click();
    await expect(page.getByTestId('kb-editor')).toBeVisible();

    const editor = page.getByTestId('kb-editor-body').locator('.ProseMirror');
    await editor.click();
    await page.keyboard.type('Первый абзац E2E');
    await page.keyboard.press('Enter');
    await page.keyboard.type('Второй абзац E2E');

    // Курсор — в конец первого абзаца, затем вставляем картинку (между абзацами).
    await page.keyboard.press('ArrowUp');
    await page.keyboard.press('End');
    await page.getByTestId('kb-image-input').setInputFiles('e2e/fixtures/photo.png');
    await expect(editor.locator('img')).toBeVisible();

    await page.getByTestId('kb-editor-save').click();
    await expect(page.getByTestId('kb-article-content')).toBeVisible();

    const html = await page.getByTestId('kb-article-content').innerHTML();
    const first = html.indexOf('Первый абзац E2E');
    const image = html.indexOf('<img');
    const second = html.indexOf('Второй абзац E2E');
    expect(first).toBeGreaterThan(-1);
    expect(image).toBeGreaterThan(first);
    expect(second).toBeGreaterThan(image);
  });

  test('перемещает раздел DnD и сохраняет нового родителя', async ({ page }) => {
    await login(page, 'admin', 'admin12345');
    await page.locator('a[href="/kb"]').first().click();
    await expect(page.getByTestId('kb-page')).toBeVisible();

    const rootId = await createNode(page, 'root', 'E2E DnD корень');
    const childId = await createNode(page, 'child', 'E2E DnD ребёнок');

    // Перетаскиваем ребёнка на корневой раздел дерева: dnd-kit работает на
    // pointer-событиях, поэтому ведём мышь по шагам.
    const handle = page.getByTestId(`kb-drag-${childId}`);
    const target = page.getByTestId('kb-node-1');
    await expect(handle).toBeVisible();
    const from = await handle.boundingBox();
    const to = await target.boundingBox();
    expect(from && to).toBeTruthy();

    const moveResponse = page.waitForResponse(
      (response) => response.url().includes(`/api/kb/nodes/${childId}/move`) && response.ok(),
    );
    await page.mouse.move(from!.x + from!.width / 2, from!.y + from!.height / 2);
    await page.mouse.down();
    await page.mouse.move(from!.x + 30, from!.y + 40, { steps: 10 });
    await page.mouse.move(to!.x + 20, to!.y + to!.height / 2, { steps: 20 });
    await page.mouse.up();
    await moveResponse;

    await expect(page.getByTestId('kb-move-error')).toHaveCount(0);

    const tree = await page.evaluate(async () => {
      const response = await fetch('/api/kb/tree', {
        headers: { Authorization: 'Bearer mock-access-token-1' },
      });
      return response.json();
    });
    const find = (
      nodes: Array<{ id: number; parent_id: number | null; children?: unknown[] }>,
      id: number,
    ): { parent_id: number | null } | null => {
      for (const node of nodes) {
        if (node.id === id) return node;
        const found = find((node.children ?? []) as never[], id);
        if (found) return found;
      }
      return null;
    };
    const moved = find(tree, childId);
    expect(moved).not.toBeNull();
    expect(moved?.parent_id).toBeNull();
    expect(rootId).toBeGreaterThan(0);
  });
});
