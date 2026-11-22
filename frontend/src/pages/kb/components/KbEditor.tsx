/**
 * Редактор статьи БЗ на TipTap (Ф3, KB-4, KB-5).
 *
 * Возможности: заголовки, списки, ссылки (только http/https), code/pre,
 * цитаты, отмена/повтор, вставка изображения и видео **между абзацами**
 * через загрузку файла в `POST /api/files` со `scope=kb`.
 *
 * Загрузка медиа идёт мимо редактора: сначала файл попадает на сервер,
 * затем в документ вставляется `<img src="/api/files/{id}">` или
 * `<video><source src="/api/files/{id}"></video>` — так сервер может проверить
 * ссылку (KB-5, `400 INVALID_MEDIA_REF`) и не хранить base64.
 */
import { useCallback, useEffect, useRef, useState } from 'react';
import {
  BoldOutlined,
  CloseOutlined,
  CodeOutlined,
  ItalicOutlined,
  LinkOutlined,
  OrderedListOutlined,
  PictureOutlined,
  RedoOutlined,
  SaveOutlined,
  StrikethroughOutlined,
  UndoOutlined,
  UnorderedListOutlined,
  VideoCameraOutlined,
} from '@ant-design/icons';
import { Alert, Button, Flex, Input, Modal, Space, Tag, Tooltip, Typography } from 'antd';
import { EditorContent, useEditor, type Editor } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import Image from '@tiptap/extension-image';
import Placeholder from '@tiptap/extension-placeholder';
import type { FileMeta } from '@/api/types';
import { uploadFile as defaultUploadFile, validateUploadFile } from '@/api/upload';
import type { UploadHandler } from '@/components/FileUploader';
import { kbErrorMessage } from '../hooks';
import { KbVideo } from './KbVideoNode';

export interface KbEditorValues {
  title: string;
  content_html: string;
}

/**
 * Не скроллим страницу к курсору при программном `focus()`: визуально это
 * дёргает статью при вставке медиа, а в jsdom `getClientRects` у текстовых
 * узлов не реализован (тесты падали бы на служебной ошибке ProseMirror).
 */
const NO_SCROLL = { scrollIntoView: false } as const;

export interface KbEditorProps {
  /** Исходные значения — с них считается «грязное» состояние. */
  initialTitle: string;
  initialHtml: string;
  pending?: boolean;
  /** Ошибка сохранения с сервера (показывается над формой). */
  error?: unknown;
  /** Подмена загрузчика в тестах (по умолчанию `uploadFile`, scope=kb). */
  upload?: UploadHandler;
  onDirtyChange?: (dirty: boolean) => void;
  onSave: (values: KbEditorValues) => void | Promise<void>;
  onCancel: () => void;
}

/** Вставка загруженного файла в позицию курсора (KB-5). */
export function insertKbMedia(editor: Editor, file: FileMeta): void {
  const src = `/api/files/${file.id}`;
  const isImage = file.media_kind === 'image' || file.content_type.startsWith('image/');
  if (isImage) {
    editor
      .chain()
      .focus(null, NO_SCROLL)
      .setImage({ src, alt: file.filename, title: file.filename })
      .run();
    return;
  }
  editor
    .chain()
    .focus(null, NO_SCROLL)
    .insertContent({
      type: 'kbVideo',
      attrs: { src, title: file.filename, type: file.content_type || 'video/mp4' },
    })
    .run();
}

interface ToolButtonProps {
  label: string;
  icon: React.ReactNode;
  active?: boolean;
  disabled?: boolean;
  testId: string;
  onClick: () => void;
}

function ToolButton({ label, icon, active = false, disabled, testId, onClick }: ToolButtonProps) {
  return (
    <Tooltip title={label}>
      <Button
        size="small"
        type={active ? 'primary' : 'text'}
        icon={icon}
        disabled={disabled}
        aria-label={label}
        aria-pressed={active}
        data-testid={testId}
        onClick={onClick}
      />
    </Tooltip>
  );
}

export function KbEditor({
  initialTitle,
  initialHtml,
  pending = false,
  error,
  upload,
  onDirtyChange,
  onSave,
  onCancel,
}: KbEditorProps) {
  const [title, setTitle] = useState(initialTitle);
  const [html, setHtml] = useState(initialHtml);
  const [localError, setLocalError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  const [linkHref, setLinkHref] = useState('');
  const [linkError, setLinkError] = useState<string | null>(null);
  const imageInputRef = useRef<HTMLInputElement>(null);
  const videoInputRef = useRef<HTMLInputElement>(null);

  const uploader = upload ?? defaultUploadFile;

  const editor = useEditor(
    {
      extensions: [
        StarterKit.configure({
          heading: { levels: [2, 3, 4] },
          link: {
            openOnClick: false,
            autolink: true,
            HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
          },
        }),
        Image.configure({ allowBase64: false, inline: false }),
        KbVideo,
        Placeholder.configure({ placeholder: 'Текст статьи: шаги, ссылки, фото и видео…' }),
      ],
      content: initialHtml,
      editorProps: {
        attributes: {
          class: 'kb-editor__prosemirror',
          'aria-label': 'Текст статьи',
          role: 'textbox',
          'aria-multiline': 'true',
        },
        // Не скроллим к выделению на каждый программный focus(): в jsdom это
        // падает на getClientRects, а в браузере дёргает страницу при вставке медиа.
        handleScrollToSelection: () => false,
      },
      onUpdate: ({ editor: instance }) => {
        setHtml(instance.getHTML());
      },
    },
    [],
  );

  const dirty = title !== initialTitle || html !== initialHtml;

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  const handleMediaFile = useCallback(
    async (file: File, expected: 'image' | 'video') => {
      setLocalError(null);
      const validation = validateUploadFile(file);
      if (!validation.ok) {
        setLocalError(validation.error ?? 'Файл не подходит');
        return;
      }
      if (validation.kind !== expected) {
        setLocalError(expected === 'image' ? 'Выберите изображение' : 'Выберите видео');
        return;
      }
      if (!editor) return;
      setUploading(true);
      try {
        // KB-5: файл сначала на сервер, затем в статью — только id, без base64.
        const meta = await uploader(file, { scope: 'kb' });
        insertKbMedia(editor, meta);
        setHtml(editor.getHTML());
      } catch (cause) {
        setLocalError(kbErrorMessage(cause, 'Не удалось загрузить файл'));
      } finally {
        setUploading(false);
      }
    },
    [editor, uploader],
  );

  const openLinkDialog = () => {
    if (!editor) return;
    setLinkHref((editor.getAttributes('link').href as string | undefined) ?? '');
    setLinkError(null);
    setLinkOpen(true);
  };

  const applyLink = () => {
    if (!editor) return;
    const href = linkHref.trim();
    if (!href) {
      editor.chain().focus(null, NO_SCROLL).unsetLink().run();
      setLinkOpen(false);
      return;
    }
    // KB-4: allowlist ссылок — только http/https, иначе сервер их вырежет.
    if (!/^https?:\/\/\S+$/i.test(href)) {
      setLinkError('Ссылка должна начинаться с http:// или https://');
      return;
    }
    editor.chain().focus(null, NO_SCROLL).extendMarkRange('link').setLink({ href }).run();
    setLinkOpen(false);
  };

  const handleSave = () => {
    if (!editor) return;
    const trimmed = title.trim();
    if (!trimmed) {
      setLocalError('Укажите заголовок статьи');
      return;
    }
    void onSave({ title: trimmed, content_html: editor.getHTML() });
  };

  const busy = pending || uploading;

  return (
    <Flex vertical gap={12} data-testid="kb-editor">
      <Flex vertical gap={4}>
        <Typography.Text strong>Заголовок</Typography.Text>
        <Input
          value={title}
          onChange={(event) => setTitle(event.target.value)}
          maxLength={200}
          showCount
          disabled={pending}
          data-testid="kb-editor-title"
          aria-label="Заголовок статьи"
        />
      </Flex>

      <Flex
        align="center"
        gap={8}
        wrap
        className="kb-editor__toolbar"
        data-testid="kb-editor-toolbar"
      >
        <ToolButton
          label="Жирный"
          icon={<BoldOutlined />}
          active={editor?.isActive('bold')}
          disabled={!editor || busy}
          testId="kb-tool-bold"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleBold().run()}
        />
        <ToolButton
          label="Курсив"
          icon={<ItalicOutlined />}
          active={editor?.isActive('italic')}
          disabled={!editor || busy}
          testId="kb-tool-italic"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleItalic().run()}
        />
        <ToolButton
          label="Зачёркнутый"
          icon={<StrikethroughOutlined />}
          active={editor?.isActive('strike')}
          disabled={!editor || busy}
          testId="kb-tool-strike"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleStrike().run()}
        />
        <ToolButton
          label="Заголовок 2"
          icon={<span style={{ fontWeight: 700 }}>H2</span>}
          active={editor?.isActive('heading', { level: 2 })}
          disabled={!editor || busy}
          testId="kb-tool-h2"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleHeading({ level: 2 }).run()}
        />
        <ToolButton
          label="Заголовок 3"
          icon={<span style={{ fontWeight: 700 }}>H3</span>}
          active={editor?.isActive('heading', { level: 3 })}
          disabled={!editor || busy}
          testId="kb-tool-h3"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleHeading({ level: 3 }).run()}
        />
        <ToolButton
          label="Маркированный список"
          icon={<UnorderedListOutlined />}
          active={editor?.isActive('bulletList')}
          disabled={!editor || busy}
          testId="kb-tool-ul"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleBulletList().run()}
        />
        <ToolButton
          label="Нумерованный список"
          icon={<OrderedListOutlined />}
          active={editor?.isActive('orderedList')}
          disabled={!editor || busy}
          testId="kb-tool-ol"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleOrderedList().run()}
        />
        <ToolButton
          label="Блок кода"
          icon={<CodeOutlined />}
          active={editor?.isActive('codeBlock')}
          disabled={!editor || busy}
          testId="kb-tool-code"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).toggleCodeBlock().run()}
        />
        <ToolButton
          label="Ссылка"
          icon={<LinkOutlined />}
          active={editor?.isActive('link')}
          disabled={!editor || busy}
          testId="kb-tool-link"
          onClick={openLinkDialog}
        />
        <ToolButton
          label="Вставить изображение"
          icon={<PictureOutlined />}
          disabled={!editor || busy}
          testId="kb-tool-image"
          onClick={() => imageInputRef.current?.click()}
        />
        <ToolButton
          label="Вставить видео"
          icon={<VideoCameraOutlined />}
          disabled={!editor || busy}
          testId="kb-tool-video"
          onClick={() => videoInputRef.current?.click()}
        />
        <ToolButton
          label="Отменить"
          icon={<UndoOutlined />}
          disabled={!editor || busy}
          testId="kb-tool-undo"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).undo().run()}
        />
        <ToolButton
          label="Повторить"
          icon={<RedoOutlined />}
          disabled={!editor || busy}
          testId="kb-tool-redo"
          onClick={() => editor?.chain().focus(null, NO_SCROLL).redo().run()}
        />

        <input
          ref={imageInputRef}
          type="file"
          accept="image/*"
          hidden
          data-testid="kb-image-input"
          onChange={(event) => {
            const file = event.target.files?.[0];
            if (file) void handleMediaFile(file, 'image');
            event.target.value = '';
          }}
        />
        <input
          ref={videoInputRef}
          type="file"
          accept="video/*"
          hidden
          data-testid="kb-video-input"
          onChange={(event) => {
            const file = event.target.files?.[0];
            if (file) void handleMediaFile(file, 'video');
            event.target.value = '';
          }}
        />
      </Flex>

      {uploading ? (
        <Typography.Text type="secondary" style={{ fontSize: 12 }} data-testid="kb-media-uploading">
          Загрузка файла…
        </Typography.Text>
      ) : null}

      {localError ? (
        <Alert
          type="error"
          showIcon
          closable
          message={localError}
          data-testid="kb-editor-local-error"
          onClose={() => setLocalError(null)}
        />
      ) : null}

      {error ? (
        <Alert
          type="error"
          showIcon
          message={kbErrorMessage(error, 'Не удалось сохранить статью')}
          data-testid="kb-editor-server-error"
        />
      ) : null}

      <div className="kb-editor__body" data-testid="kb-editor-body">
        <EditorContent editor={editor} />
      </div>

      <Flex align="center" justify="space-between" gap={8} wrap>
        <Space size={8}>
          {dirty ? (
            <Tag color="orange" data-testid="kb-editor-dirty">
              Есть несохранённые изменения
            </Tag>
          ) : (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              Изменений нет
            </Typography.Text>
          )}
        </Space>
        <Space size={8}>
          <Button icon={<CloseOutlined />} onClick={onCancel} data-testid="kb-editor-cancel">
            Отмена
          </Button>
          <Button
            type="primary"
            icon={<SaveOutlined />}
            loading={pending}
            disabled={!editor || uploading}
            onClick={handleSave}
            data-testid="kb-editor-save"
          >
            Сохранить
          </Button>
        </Space>
      </Flex>

      <Modal
        open={linkOpen}
        title="Ссылка"
        okText="Применить"
        cancelText="Отмена"
        onOk={applyLink}
        onCancel={() => setLinkOpen(false)}
        destroyOnHidden
      >
        <Input
          value={linkHref}
          onChange={(event) => {
            setLinkHref(event.target.value);
            setLinkError(null);
          }}
          placeholder="https://example.org"
          onPressEnter={applyLink}
          data-testid="kb-link-input"
          aria-label="Адрес ссылки"
        />
        {linkError ? (
          <Typography.Text type="danger" style={{ fontSize: 12 }}>
            {linkError}
          </Typography.Text>
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            Разрешены только ссылки http/https (KB-4).
          </Typography.Text>
        )}
      </Modal>
    </Flex>
  );
}
