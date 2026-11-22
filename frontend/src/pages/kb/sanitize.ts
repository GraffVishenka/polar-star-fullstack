/**
 * Санитизация HTML статьи БЗ (KB-4, KB-5).
 *
 * Бэкенд уже санитизирует `content_html` через bluemonday, но клиент обязан
 * защищаться повторно: контент мог быть сохранён старым кодом, а XSS в
 * редакторе статьи — прямой путь к краже access-токена (он живёт в памяти
 * этого же документа). Поэтому здесь строгий allowlist тегов и атрибутов:
 * заголовки, списки, ссылки http/https, `<img>`, `<video>/<source>`, code/pre,
 * таблицы. `<script>`, `on*`, `<iframe>`, `style` вырезаются.
 */
import DOMPurify from 'dompurify';

/** Теги, которые разрешены в статье (KB-4). */
export const KB_ALLOWED_TAGS: string[] = [
  // структура текста
  'p',
  'br',
  'hr',
  'div',
  'span',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'strong',
  'b',
  'em',
  'i',
  'u',
  's',
  'sub',
  'sup',
  'mark',
  'blockquote',
  'pre',
  'code',
  // списки
  'ul',
  'ol',
  'li',
  // ссылки
  'a',
  // медиа между абзацами (KB-5)
  'img',
  'video',
  'source',
  'figure',
  'figcaption',
  // таблицы
  'table',
  'caption',
  'colgroup',
  'col',
  'thead',
  'tbody',
  'tfoot',
  'tr',
  'th',
  'td',
];

/** Разрешённые атрибуты. Стили и обработчики (`on*`) не входят сознательно. */
export const KB_ALLOWED_ATTR: string[] = [
  'href',
  'target',
  'rel',
  'title',
  'src',
  'alt',
  'width',
  'height',
  'class',
  'controls',
  'preload',
  'poster',
  'type',
  'colspan',
  'rowspan',
  'scope',
  'start',
];

/** Явно запрещённые теги — даже если DOMPurify когда-то изменит дефолты. */
export const KB_FORBID_TAGS: string[] = [
  'script',
  'style',
  'iframe',
  'object',
  'embed',
  'form',
  'input',
  'button',
  'link',
  'meta',
  'base',
];

/**
 * Ссылки открываем в новой вкладке и без доступа к `window.opener`
 * (обратная табуляция). Делается хуком, а не правкой HTML регулярками.
 */
const linkHardeningHook = (() => {
  let installed = false;
  return (): void => {
    if (installed) return;
    installed = true;
    DOMPurify.addHook('afterSanitizeAttributes', (node) => {
      if (node.nodeName !== 'A') return;
      const element = node as unknown as Element;
      if (!element.getAttribute('href')) return;
      element.setAttribute('target', '_blank');
      element.setAttribute('rel', 'noopener noreferrer nofollow');
    });
  };
})();

/** Очищает HTML статьи до безопасного подмножества (KB-4). */
export function sanitizeKbHtml(html: string): string {
  if (!html) return '';
  linkHardeningHook();
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: KB_ALLOWED_TAGS,
    ALLOWED_ATTR: KB_ALLOWED_ATTR,
    FORBID_TAGS: KB_FORBID_TAGS,
    // `data-*` и `aria-*` не нужны, а вместе с ними не пройдут и «носители»
    // обработчиков вроде data-onclick.
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
    // Разрешаем только относительные URL и http/https (javascript:/data: — нет).
    ALLOWED_URI_REGEXP: /^(?:(?:https?|mailto):|\/(?!\/)|#)/i,
  });
}

/** Идентификаторы файлов, на которые ссылается статья (`/api/files/{id}`). */
export function extractKbMediaIds(html: string): number[] {
  const ids = new Set<number>();
  const pattern = /\/api\/files\/(\d+)/g;
  let match: RegExpExecArray | null = pattern.exec(html);
  while (match) {
    const id = Number(match[1]);
    if (Number.isSafeInteger(id)) ids.add(id);
    match = pattern.exec(html);
  }
  return [...ids];
}

/** HTML вставки изображения между абзацами (KB-5) — для превью и тестов. */
export function kbImageHtml(fileId: number, alt = ''): string {
  const safeAlt = alt.replace(/[<>&"]/g, '');
  return `<img src="/api/files/${fileId}" alt="${safeAlt}" loading="lazy">`;
}

/** HTML вставки видео между абзацами: `<video><source ...></video>` (KB-5). */
export function kbVideoHtml(fileId: number, title = ''): string {
  const safeTitle = title.replace(/[<>&"]/g, '');
  return `<video controls preload="metadata" title="${safeTitle}"><source src="/api/files/${fileId}" type="video/mp4"></video>`;
}
