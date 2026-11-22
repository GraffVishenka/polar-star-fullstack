/**
 * `/public/kb` — публичная база знаний (PUB-7, KB-7).
 *
 * Доступна без входа: дерево без скрытых корневых разделов и статья в режиме
 * чтения. HTML санитизируется (KB-4): `<script>`, `on*` и `iframe` вырезаются,
 * медиа из БЗ переписывается на публичный эндпоинт файлов.
 */
import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { ReadOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Col, Flex, Row, Skeleton, Typography } from 'antd';
import DOMPurify from 'dompurify';
import { Link, useSearchParams } from 'react-router-dom';
import { fetchPublicKbNode, fetchPublicKbTree } from '@/api/public';
import type { KbTreeNode } from '@/api/types';
import { queryKeys } from '@/app/queryClient';
import { EmptyState } from '@/components/EmptyState';
import { TreeNav } from '@/components/TreeNav';
import type { TreeNavItem } from '@/components/TreeNav';
import { PublicNotFound } from './components/PublicNotFound';
import { isApiError } from '@/api/errors';
import dayjs from 'dayjs';

/** KB-5: `/api/files/{id}` требует авторизации, публично — `/api/public/files/{id}`. */
export function toPublicMediaHtml(html: string): string {
  return html.replace(/((?:src|href)=["'])\/api\/files\//g, '$1/api/public/files/');
}

/** KB-4: allowlist тегов и атрибутов; DOMPurify вырезает script, on*-атрибуты и iframe. */
export function sanitizeKbHtml(html: string): string {
  return DOMPurify.sanitize(toPublicMediaHtml(html), {
    ALLOWED_TAGS: [
      'h1',
      'h2',
      'h3',
      'h4',
      'p',
      'br',
      'hr',
      'strong',
      'b',
      'em',
      'i',
      'u',
      's',
      'ul',
      'ol',
      'li',
      'a',
      'img',
      'video',
      'source',
      'code',
      'pre',
      'blockquote',
      'table',
      'thead',
      'tbody',
      'tr',
      'th',
      'td',
    ],
    ALLOWED_ATTR: [
      'href',
      'src',
      'alt',
      'title',
      'controls',
      'width',
      'height',
      'target',
      'rel',
      'colspan',
      'rowspan',
    ],
  });
}

function toTreeItems(nodes: KbTreeNode[]): TreeNavItem[] {
  return nodes.map((node) => ({
    id: node.id,
    title: node.title,
    isHidden: node.is_hidden_effective,
    children: node.children?.length ? toTreeItems(node.children) : undefined,
  }));
}

export function PublicKbPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const rawNodeId = Number(searchParams.get('node'));
  const selectedId = Number.isSafeInteger(rawNodeId) && rawNodeId > 0 ? rawNodeId : null;

  const treeQuery = useQuery({
    queryKey: queryKeys.publicKbTree,
    queryFn: fetchPublicKbTree,
    staleTime: 60_000,
  });

  const nodeQuery = useQuery({
    queryKey: queryKeys.publicKbNode(selectedId ?? 0),
    queryFn: () => fetchPublicKbNode(selectedId!),
    enabled: selectedId !== null,
    retry: false,
  });

  const treeItems = useMemo(() => toTreeItems(treeQuery.data ?? []), [treeQuery.data]);

  const selectNode = (item: TreeNavItem) => {
    const next = new URLSearchParams(searchParams);
    next.set('node', String(item.id));
    setSearchParams(next, { replace: true });
  };

  if (treeQuery.isLoading) {
    return (
      <Flex vertical gap={16}>
        <Skeleton active title paragraph={{ rows: 2 }} />
        <Skeleton active paragraph={{ rows: 6 }} />
      </Flex>
    );
  }

  if (treeQuery.isError) {
    return (
      <Alert
        type="error"
        showIcon
        message="Не удалось загрузить базу знаний"
        action={
          <Button size="small" onClick={() => void treeQuery.refetch()}>
            Повторить
          </Button>
        }
        data-testid="public-kb-error"
      />
    );
  }

  const nodeError = nodeQuery.error;
  const nodeNotFound =
    selectedId !== null && nodeQuery.isError && isApiError(nodeError) && nodeError.status === 404;

  return (
    <Flex vertical gap={16} data-testid="public-kb-page">
      <Flex vertical gap={4}>
        <Typography.Title level={1} style={{ margin: 0, fontSize: 'clamp(22px, 5vw, 30px)' }}>
          <ReadOutlined /> База знаний
        </Typography.Title>
        <Typography.Text type="secondary">
          Публичные инструкции. Скрытые разделы доступны только сотрудникам после входа.
        </Typography.Text>
      </Flex>

      <Row gutter={[16, 16]}>
        <Col xs={24} md={8} lg={7}>
          <Card size="small" title="Разделы" data-testid="public-kb-tree">
            <TreeNav
              nodes={treeItems}
              selectedId={selectedId}
              onSelect={selectNode}
              defaultExpandDepth={2}
              showExpandControls
              emptyText="Публичных разделов пока нет"
              ariaLabel="Разделы базы знаний"
            />
          </Card>
        </Col>

        <Col xs={24} md={16} lg={17}>
          {nodeNotFound ? (
            <PublicNotFound
              title="Раздел не найден"
              description="Возможно, раздел скрыт или удалён. Вернитесь к списку разделов."
            />
          ) : null}

          {selectedId === null ? (
            <Card data-testid="public-kb-placeholder">
              <EmptyState
                title="Выберите раздел"
                description="Слева — дерево публичных инструкций. Выберите раздел, чтобы прочитать статью."
              />
            </Card>
          ) : null}

          {selectedId !== null && nodeQuery.isLoading ? (
            <Card>
              <Skeleton active paragraph={{ rows: 8 }} />
            </Card>
          ) : null}

          {nodeQuery.isError && !nodeNotFound ? (
            <Alert
              type="error"
              showIcon
              message="Не удалось открыть раздел"
              action={
                <Button size="small" onClick={() => void nodeQuery.refetch()}>
                  Повторить
                </Button>
              }
            />
          ) : null}

          {nodeQuery.data ? (
            <Card
              data-testid="public-kb-article"
              title={nodeQuery.data.title}
              extra={
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {dayjs(nodeQuery.data.updated_at).format('DD.MM.YYYY')}
                </Typography.Text>
              }
            >
              <div
                className="kb-article"
                // KB-4: HTML приходит санитизированным сервером и повторно — на клиенте.
                dangerouslySetInnerHTML={{ __html: sanitizeKbHtml(nodeQuery.data.content_html) }}
              />
              {nodeQuery.data.updated_by_name ? (
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  Обновил: {nodeQuery.data.updated_by_name}
                </Typography.Text>
              ) : null}
            </Card>
          ) : null}
        </Col>
      </Row>

      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        Сотрудникам доступны закрытые разделы после <Link to="/login">входа</Link>.
      </Typography.Text>
    </Flex>
  );
}
