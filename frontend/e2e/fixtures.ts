/**
 * Базовый `test` с защитой от ошибок в консоли/на странице (Ф11).
 *
 * `pageerror` — всегда падение теста. `console.error` — падение, кроме известных
 * dev-предупреждений библиотек (antd/React Router/ResizeObserver), которые не
 * являются ошибками приложения.
 */
import { test as base, expect } from '@playwright/test';

const IGNORED_CONSOLE_ERRORS = [
  /Warning: \[antd:/,
  /There may be circular references/,
  /antd v5 support React is 16/,
  /findDOMNode is deprecated/,
  /React Router Future Flag/,
  /ResizeObserver loop/,
  /Download the React DevTools/,
  // Анонимный bootstrap: `/auth/me` и `/auth/refresh` штатно отвечают 401.
  /Failed to load resource: the server responded with a status of 401 \(Unauthorized\)/,
  /Failed to load resource: the server responded with a status of 404 \(Not Found\)/,
];

export const test = base.extend({
  page: async ({ page }, provide) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => {
      errors.push(`pageerror: ${error.message}`);
    });
    page.on('console', (message) => {
      if (message.type() !== 'error') return;
      const text = message.text();
      if (IGNORED_CONSOLE_ERRORS.some((pattern) => pattern.test(text))) return;
      errors.push(`console.error: ${text}`);
    });

    await provide(page);

    expect(errors, `Ошибки браузера:\n${errors.join('\n')}`).toEqual([]);
  },
});

export { expect };
