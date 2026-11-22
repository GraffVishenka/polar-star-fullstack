/**
 * `/admin/kb` — модерация базы знаний (Ф7, §3.5 KB-1/KB-3/KB-7, только ADMIN).
 *
 * Задачи раздела: включить/выключить видимость корневого раздела для анонимов
 * (`is_hidden_for_anon`, у потомков наследуется) и удалить раздел в корзину
 * с подтверждением и показом числа потомков.
 *
 * Полноценный редактор дерева/статей делает fe-2 в `pages/kb` — здесь
 * намеренно простая таблица-дерево, без drag-n-drop и TipTap.
 */
import { useCallback, useMemo, useState } from 'react';
import { DeleteOutlined, LockOutlined, ReloadOutlined } from '@ant-design/icons';
import { Alert, App, Button, Card, Flex, Skeleton, Switch, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { deleteKbNode, fetchKbTree, setKbNodeVisibility } from '@/api/kb';
import { queryKeys } from '@/app/queryClient';
import { describeInfraErrorMessage } from '../errors';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { EmptyState } from '@/components/EmptyState';
import { PageHeader } from '@/components/PageHeader';
import { toKbAdminRows, type KbAdminRow } from './kbTreeUtils';

export function AdminKbPage() {
  const { message } = App.useApp();
  const client = useQueryClient();

  const treeQuery = useQuery({
    queryKey: queryKeys.kbTree,
    queryFn: () => fetchKbTree(),
  });

  const [deleteTarget, setDeleteTarget] = useState<KbAdminRow | null>(null);

  const rows = useMemo(() => toKbAdminRows(treeQuery.data ?? []), [treeQuery.data]);

  const invalidateKb = useCallback(() => {
    void client.invalidateQueries({ queryKey: queryKeys.kbTree });
    // Скрытие раздела меняет публичное дерево анонима (KB-7).
    void client.invalidateQueries({ queryKey: queryKeys.publicKbTree });
  }, [client]);

  const visibilityMutation = useMutation({
    mutationFn: ({ id, hidden }: { id: number; hidden: boolean }) =>
      setKbNodeVisibility(id, hidden),
    onSuccess: (_node, variables) => {
      invalidateKb();
      message.success(
        variables.hidden
          ? 'Раздел скрыт от анонимов — потомки тоже не видны (KB-7)'
          : 'Раздел снова виден анонимам',
      );
    },
    onError: (cause) => message.error(describeInfraErrorMessage(cause)),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteKbNode(id),
  });

  const handleVisibility = useCallback(
    (row: KbAdminRow, nextVisible: boolean) => {
      visibilityMutation.mutate({ id: row.id, hidden: !nextVisible });
    },
    [visibilityMutation],
  );

  const confirmDelete = useCallback(async () => {
    if (!deleteTarget) return;
    const row = deleteTarget;
    try {
      const response = await deleteMutation.mutateAsync(row.id);
      invalidateKb();
      message.success(
        `Раздел «${row.title}» удалён в корзину · потомков удалено: ${response.descendants_deleted}`,
      );
    } catch (cause) {
      message.error(describeInfraErrorMessage(cause));
    } finally {
      setDeleteTarget(null);
    }
  }, [deleteMutation, deleteTarget, invalidateKb, message]);

  const columns: ColumnsType<KbAdminRow> = [
    {
      title: 'Раздел',
      dataIndex: 'title',
      key: 'title',
      render: (_value, row) => (
        <Flex vertical gap={2}>
          <Flex align="center" gap={6} wrap>
            <Typography.Text strong data-testid={`kb-title-${row.id}`}>
              {row.title}
            </Typography.Text>
            {row.is_hidden_effective ? (
              <Tag icon={<LockOutlined />} color="default" data-testid={`kb-hidden-${row.id}`}>
                скрыт от анонимов
              </Tag>
            ) : null}
          </Flex>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            #{row.id} · потомков: {row.descendants}
            {row.isRoot ? ' · корневой раздел' : ' · вложенный'}
          </Typography.Text>
        </Flex>
      ),
    },
    {
      title: 'Доступ анонимам',
      key: 'visibility',
      width: 210,
      render: (_value, row) =>
        row.isRoot ? (
          <Flex align="center" gap={8}>
            <Switch
              checked={!row.is_hidden_for_anon}
              checkedChildren="Виден"
              unCheckedChildren="Скрыт"
              aria-label={`Доступ анонимам: ${row.title}`}
              data-testid={`kb-visibility-${row.id}`}
              onChange={(checked) => handleVisibility(row, checked)}
            />
          </Flex>
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            наследуется от корня
          </Typography.Text>
        ),
    },
    {
      title: '',
      key: 'actions',
      width: 100,
      align: 'right',
      render: (_value, row) => (
        <Button
          size="small"
          type="text"
          danger
          icon={<DeleteOutlined />}
          aria-label={`Удалить раздел ${row.title}`}
          onClick={() => setDeleteTarget(row)}
          data-testid={`kb-delete-${row.id}`}
        >
          Удалить
        </Button>
      ),
    },
  ];

  return (
    <Flex vertical gap={16} data-testid="admin-kb-page">
      <PageHeader
        title="Модерация базы знаний"
        description="Видимость настраивается у корневых разделов и наследуется потомками (KB-1, KB-7). Удаление — в корзину, только ADMIN (KB-3)."
        extra={
          <Button
            icon={<ReloadOutlined />}
            loading={treeQuery.isFetching}
            onClick={() => void treeQuery.refetch()}
            data-testid="kb-refresh"
          >
            Обновить
          </Button>
        }
      />

      <Alert
        type="info"
        showIcon
        message="Как это работает"
        description="Скрытый корневой раздел не виден анонимам, включая все вложенные статьи (KB-7); авторизованные сотрудники видят его как обычно. Удаление раздела каскадно убирает потомков в корзину, но восстановится только сам раздел — потомки нет (TRASH-5)."
        data-testid="kb-rules"
      />

      {treeQuery.isError ? (
        <Alert
          type="error"
          showIcon
          message="Не удалось загрузить дерево базы знаний"
          action={
            <Button size="small" onClick={() => void treeQuery.refetch()}>
              Повторить
            </Button>
          }
          data-testid="kb-error"
        />
      ) : treeQuery.isLoading ? (
        <Skeleton active paragraph={{ rows: 6 }} />
      ) : (
        <Card size="small" data-testid="kb-table">
          <Table<KbAdminRow>
            rowKey="id"
            size="small"
            columns={columns}
            dataSource={rows}
            pagination={false}
            expandable={{ defaultExpandAllRows: true }}
            loading={treeQuery.isFetching}
            locale={{
              emptyText: (
                <EmptyState
                  title="Разделов пока нет"
                  description="Создать разделы можно в редакторе базы знаний."
                />
              ),
            }}
          />
        </Card>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title={`Удалить раздел «${deleteTarget?.title ?? ''}»?`}
        description={
          deleteTarget
            ? `Раздел и его потомки (${deleteTarget.descendants}) будут перемещены в корзину.`
            : undefined
        }
        warning="Удаление необратимо для потомков: при восстановлении вернётся только сам раздел (TRASH-5)."
        okText="Удалить в корзину"
        danger
        loading={deleteMutation.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={confirmDelete}
      />
    </Flex>
  );
}
