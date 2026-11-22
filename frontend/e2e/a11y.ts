/** axe-core: проверка доступности без critical-нарушений (Ф11). */
import AxeBuilder from '@axe-core/playwright';
import { expect, type Page } from '@playwright/test';

export async function expectNoCriticalA11y(page: Page, context: string): Promise<void> {
  const results = await new AxeBuilder({ page })
    .withTags(['wcag2a', 'wcag2aa', 'best-practice'])
    .analyze();

  const critical = results.violations.filter((violation) => violation.impact === 'critical');
  const summary = critical
    .map(
      (violation) =>
        `${violation.id} (${violation.impact}): ${violation.help}\n  ${violation.nodes
          .slice(0, 3)
          .map((node) => node.target.join(' '))
          .join('\n  ')}`,
    )
    .join('\n');

  expect(critical, `${context}: critical-нарушения axe:\n${summary}`).toEqual([]);
}
