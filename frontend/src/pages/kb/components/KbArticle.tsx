/**
 * Статья БЗ: безопасный рендер `content_html` и действия раздела (Ф3, KB-4, KB-7).
 *
 * HTML всегда проходит через `sanitizeKbHtml` (allowlist): `<script>` не
 * исполняется, `on*`-атрибуты и `javascript:`-ссылки вырезаются — даже если
 * сервер сохранил что-то лишнее.
 *
 * Права (PLAN.md §3.1): «Редактировать» доступно любому авторизованному,
 * «Удалить» и тумблер видимости — только ADMIN.
 */
import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Button, Flex, Space, Switch, Tooltip, Typography } from 'antd';
import { DeleteOutlined, EditOutlined, LockOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { KbNode } from '@/api/types';
import { PageHeader } from '@/components/PageHeader';
import { sanitizeKbHtml } from '../sanitize';
import type { KbTreeNode } from '@/api/types';

export interface KbArticleProps {
  node: KbNode;
  /** Путь от корня до узла (хлебные крошки). */
  path: KbTreeNode[];
  isAdmin: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onVisibilityChange: (hidden: boolean) => void;
  visibilityPending?: boolean;
}

function formatDateTime(value: string): string {
  const parsed = dayjs(value);
  return parsed.isValid() ? parsed.format('DD.MM.YYYY, HH:mm') : value;
}

export function KbArticle({
  node,
  path,
  isAdmin,
  onEdit,
  onDelete,
  onVisibilityChange,
  visibilityPending = false,
}: KbArticleProps) {
  // Санитизация — на каждом рендере: кэш не нужен, HTML ≤ 100 000 символов.
  const safeHtml = useMemo(() => sanitizeKbHtml(node.content_html), [node.content_html]);
  const isRoot = node.parent_id === null;

  const breadcrumbs = [
    { title: <Link to="/kb">База знаний</Link> },
    ...path.map((entry) => ({
      title: entry.id === node.id ? node.title : <Link to={`/kb/${entry.id}`}>{entry.title}</Link>,
    })),
  ];

  return (
    <Flex vertical gap={16} data-testid="kb-article">
      <PageHeader
        title={node.title}
        breadcrumbs={breadcrumbs}
        description={`Обновлено ${formatDateTime(node.updated_at)}${
          node.updated_by_name ? ` · ${node.updated_by_name}` : ''
        }`}
        extra={
          <>
            <Button icon={<EditOutlined />} onClick={onEdit} data-testid="kb-edit">
              Редактировать
            </Button>
            {isAdmin ? (
              <Button danger icon={<DeleteOutlined />} onClick={onDelete} data-testid="kb-delete">
                Удалить
              </Button>
            ) : null}
          </>
        }
      />

      <Flex gap={16} wrap align="center" className="kb-article__meta">
        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
          Создано: {formatDateTime(node.created_at)}
        </Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
          Автор: {node.updated_by_name ?? '—'}
        </Typography.Text>
        {node.is_hidden_effective ? (
          <Space size={4} data-testid="kb-article-hidden-badge">
            <LockOutlined style={{ color: '#8c8c8c' }} />
            <Typography.Text type="secondary" style={{ fontSize: 13 }}>
              Скрыт для анонимных посетителей
            </Typography.Text>
          </Space>
        ) : null}
      </Flex>

      {isAdmin ? (
        isRoot ? (
          <Flex
            align="center"
            gap={10}
            className="kb-article__visibility"
            data-testid="kb-visibility-control"
          >
            <Switch
              checked={node.is_hidden_for_anon}
              loading={visibilityPending}
              onChange={(checked) => onVisibilityChange(checked)}
              data-testid="kb-visibility-switch"
              aria-label="Скрыть раздел от анонимных посетителей"
            />
            <Typography.Text>Скрыть раздел от анонимных посетителей</Typography.Text>
            <Tooltip title="KB-7: прямые ссылки на скрытый раздел отдают 404">
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                тумблер только у корневых разделов
              </Typography.Text>
            </Tooltip>
          </Flex>
        ) : (
          <Typography.Text
            type="secondary"
            style={{ fontSize: 12 }}
            data-testid="kb-visibility-inherited"
          >
            Видимость наследуется от корневого раздела «{path[0]?.title ?? '—'}»
          </Typography.Text>
        )
      ) : null}

      {safeHtml ? (
        <div
          className="kb-article__content"
          data-testid="kb-article-content"
          // KB-4: разметка прошла allowlist-санитизацию (DOMPurify).
          dangerouslySetInnerHTML={{ __html: safeHtml }}
        />
      ) : (
        <Alert
          type="info"
          showIcon
          data-testid="kb-article-empty"
          message="Статья пока пуста"
          description="Нажмите «Редактировать», чтобы добавить текст, фото или видео."
        />
      )}
    </Flex>
  );
}
