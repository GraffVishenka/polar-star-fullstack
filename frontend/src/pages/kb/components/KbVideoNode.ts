/**
 * Узел TipTap для видео между абзацами (KB-5).
 *
 * Бэкенд разрешает в статье `<video><source src="/api/files/{id}"></video>`;
 * в схеме StarterKit такого узла нет. Объявляем его сами как атомарный блочный
 * узел, который сериализуется ровно в ту разметку, что принимает санитайзер.
 *
 * Почему `Image.extend`, а не `Node.create`: в TipTap 3 `@tiptap/core` не
 * объявлен в package.json фронта напрямую (и `@tiptap/react` не реэкспортирует
 * `Node` — имя перекрыто локальным импортом), поэтому берём за основу уже
 * подключённый блочный узел и полностью переопределяем его поведение:
 * ни аттрибутов картинки, ни команд/правил вставки от `Image` не остаётся.
 */
import Image from '@tiptap/extension-image';

export interface KbVideoAttributes {
  src: string | null;
  title: string | null;
  type: string;
}

export const KbVideo = Image.extend({
  name: 'kbVideo',
  group: 'block',
  inline: false,
  atom: true,
  draggable: true,
  selectable: true,

  // Опции Image (allowBase64/resize) наследуются, но не используются:
  // видео всегда загружается на сервер (KB-5), base64 в статью не попадает.

  // Ничего от Image: только собственные атрибуты.
  addAttributes() {
    return {
      src: { default: null },
      title: { default: null },
      type: { default: 'video/mp4' },
    };
  },

  addCommands() {
    return {};
  },

  addInputRules() {
    return [];
  },

  addPasteRules() {
    return [];
  },

  addKeyboardShortcuts() {
    return {};
  },

  parseHTML() {
    return [
      {
        tag: 'video',
        getAttrs: (element) => {
          const el = element as HTMLElement;
          const source = el.querySelector('source');
          return {
            src: el.getAttribute('src') ?? source?.getAttribute('src') ?? null,
            title: el.getAttribute('title'),
            type: source?.getAttribute('type') ?? 'video/mp4',
          };
        },
      },
    ];
  },

  renderHTML({ HTMLAttributes }) {
    const attrs = HTMLAttributes as unknown as KbVideoAttributes;
    return [
      'video',
      {
        controls: 'controls',
        preload: 'metadata',
        ...(attrs.title ? { title: attrs.title } : {}),
      },
      ['source', { src: attrs.src ?? '', type: attrs.type || 'video/mp4' }],
    ];
  },
});
