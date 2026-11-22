/** Тесты санитизации HTML статьи БЗ (KB-4, KB-5). */
import { describe, expect, it } from 'vitest';
import {
  extractKbMediaIds,
  kbImageHtml,
  kbVideoHtml,
  sanitizeKbHtml,
} from './sanitize';

describe('sanitizeKbHtml', () => {
  it('вырезает <script> и обработчики on*, оставляя разметку статьи', () => {
    const dirty =
      '<h2>Заголовок</h2><p>Текст</p><script>window.__xss = true</script>' +
      '<img src="/api/files/1" onerror="alert(1)" alt="схема">';
    const clean = sanitizeKbHtml(dirty);

    expect(clean).not.toContain('<script');
    expect(clean).not.toContain('__xss');
    expect(clean).not.toContain('onerror');
    expect(clean).toContain('<h2>Заголовок</h2>');
    expect(clean).toContain('<img src="/api/files/1"');
  });

  it('удаляет javascript:-ссылки и укрепляет оставшиеся', () => {
    const clean = sanitizeKbHtml(
      '<p><a href="javascript:alert(1)">плохая</a> <a href="https://example.org">хорошая</a></p>',
    );
    expect(clean).not.toContain('javascript:');
    expect(clean).toContain('href="https://example.org"');
    expect(clean).toContain('rel="noopener noreferrer nofollow"');
    expect(clean).toContain('target="_blank"');
  });

  it('сохраняет медиа между абзацами и таблицы, убирает iframe/style', () => {
    const html =
      '<p>до</p><img src="/api/files/1" alt="фото">' +
      '<video controls><source src="/api/files/2" type="video/mp4"></video>' +
      '<table><thead><tr><th>Поле</th></tr></thead><tbody><tr><td>Кабинет</td></tr></tbody></table>' +
      '<iframe src="https://evil.example"></iframe><p style="color:red">после</p>';
    const clean = sanitizeKbHtml(html);

    expect(clean).toContain('<img src="/api/files/1"');
    expect(clean).toContain('<video controls="">');
    expect(clean).toContain('<source src="/api/files/2"');
    expect(clean).toContain('<table>');
    expect(clean).toContain('<td>Кабинет</td>');
    expect(clean).not.toContain('iframe');
    expect(clean).not.toContain('style=');
  });

  it('возвращает пустую строку для пустого ввода', () => {
    expect(sanitizeKbHtml('')).toBe('');
  });
});

describe('медиа-ссылки статьи', () => {
  it('собирает уникальные id файлов из /api/files/{id}', () => {
    const html =
      '<img src="/api/files/7"><video><source src="/api/files/7"></video><img src="/api/files/12">';
    expect(extractKbMediaIds(html).sort((a, b) => a - b)).toEqual([7, 12]);
    expect(extractKbMediaIds('<p>без медиа</p>')).toEqual([]);
  });

  it('формирует HTML вставки между абзацами (KB-5)', () => {
    expect(kbImageHtml(3, 'Схема "А"')).toBe(
      '<img src="/api/files/3" alt="Схема А" loading="lazy">',
    );
    expect(kbVideoHtml(4, 'Инструкция')).toBe(
      '<video controls preload="metadata" title="Инструкция"><source src="/api/files/4" type="video/mp4"></video>',
    );
  });
});
