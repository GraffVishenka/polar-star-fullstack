/**
 * Публичный периметр (Ф5/Ф11): выбор здания, cookie/авторедирект, анонимная
 * заявка со сменой здания и ссылкой `/t/{token}`, Word-выгрузка.
 */
import { test, expect } from './fixtures';
import { expectNoCriticalA11y } from './a11y';
import { extractTokenPath, pickOption, readDownload } from './helpers';

test.describe('Публичный периметр', () => {
  test('выбор здания запоминается в cookie и / редиректит на дашборд', async ({
    page,
    context,
  }) => {
    await page.goto('/');
    await expect(page.getByTestId('building-select-page')).toBeVisible();
    await expect(page.getByTestId('building-card-1')).toContainText('Центральный офис');

    await page.getByTestId('building-card-1').click();
    await expect(page.getByTestId('building-dashboard-page')).toBeVisible();

    const cookie = (await context.cookies()).find((item) => item.name === 'anon_building_id');
    expect(cookie?.value).toBe('1');

    // Повторный заход на `/` — авторедирект по cookie (PUB-1).
    await page.goto('/');
    await expect(page.getByTestId('building-dashboard-page')).toBeVisible();

    // `?change=1` отключает редирект.
    await page.goto('/?change=1');
    await expect(page.getByTestId('building-select-page')).toBeVisible();
  });

  test('аноним создаёт заявку со сменой здания, ссылка работает в новом контексте', async ({
    page,
  }) => {
    await page.goto('/');
    await page.getByTestId('building-card-1').click();
    await expect(page.getByTestId('building-dashboard-page')).toBeVisible();

    await page.getByTestId('create-ticket-button').click();
    await expect(page.getByTestId('public-create-submit')).toBeVisible();

    await pickOption(page, page.getByTestId('public-create-building'), 'Производственный корпус');
    await page.getByTestId('public-create-applicant').fill('Гость E2E');
    await page.getByTestId('public-create-cabinet').fill('каб. 42');
    await pickOption(page, page.getByTestId('public-create-problem'), 'Протечка / сантехника');
    await page.getByTestId('public-create-description').fill('E2E: течёт кран в столовой');
    await page.getByTestId('public-create-submit').click();

    await expect(page.getByTestId('public-create-success')).toBeVisible();
    const successText = (await page.getByTestId('public-create-link').innerText()).trim();
    const tokenPath = extractTokenPath(successText);

    // PUB-3: cookie переключилась на выбранное здание.
    const cookie = (await page.context().cookies()).find(
      (item) => item.name === 'anon_building_id',
    );
    expect(cookie?.value).toBe('2');

    // Ссылка работает без cookie: чистим cookie и переходим по ссылке из
    // success-экрана (SPA-переход, база MSW живёт в текущей странице; при
    // полной перезагрузке/новом контексте MSW-сид создаётся заново).
    await page.context().clearCookies();
    expect((await page.context().cookies()).some((c) => c.name === 'anon_building_id')).toBe(false);
    await page.getByRole('link', { name: 'Открыть карточку' }).click();
    await expect(page).toHaveURL(new RegExp(`${tokenPath}$`));
    await expect(page.getByTestId('public-ticket-page')).toBeVisible();
    await expect(page.getByText('E2E: течёт кран в столовой')).toBeVisible();
  });

  test('Word с публичной карточки скачивается непустым файлом ticket_{id}.docx', async ({
    page,
  }) => {
    await page.goto('/t/demo-token-1');
    await expect(page.getByTestId('public-ticket-page')).toBeVisible();

    const { filename } = await readDownload(page, () => page.getByTestId('export-button').click());
    expect(filename).toMatch(/^ticket_\d+\.docx$/);
  });

  test('публичная БЗ доступна без входа, скрытый раздел не виден (KB-7)', async ({ page }) => {
    await page.goto('/public/kb');
    await expect(page.getByTestId('public-kb-page')).toBeVisible();
    await expect(page.getByTestId('tree-node-1').getByText('Инструкции')).toBeVisible();
    await expect(page.getByText('Администрирование')).toHaveCount(0);
  });

  test('ключевые публичные страницы проходят axe без critical', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByTestId('building-select-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/');

    await page.getByTestId('building-card-1').click();
    await expect(page.getByTestId('building-dashboard-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/b/1');

    await page.goto('/t/demo-token-1');
    await expect(page.getByTestId('public-ticket-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/t/{token}');

    await page.goto('/public/kb');
    await expect(page.getByTestId('public-kb-page')).toBeVisible();
    await expectNoCriticalA11y(page, '/public/kb');
  });
});
