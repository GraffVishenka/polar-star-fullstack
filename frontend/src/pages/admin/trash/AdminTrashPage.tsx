/**
 * `/admin/trash` — корзина (Ф7, §3.10 TRASH-1..TRASH-5, только ADMIN).
 *
 * Вкладки «Заявки / База знаний / Оборудование», серверная пагинация, дата
 * удаления, автор и краткое описание, дата автоочистки (90 дней).
 *
 * Восстановление (TRASH-2) показывает предупреждения из `TrashItem.warnings`:
 * деактивированное здание заявки, невосстановимые потомки БЗ, конфликт
 * регистрационного номера. Окончательное удаление (TRASH-3) требует ввода
 * слова-подтверждения.
 */
import { useCallback, useMemo, useState } from 'react';
import {
  DeleteOutlined,
  ReloadOutlined,
  RollbackOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { Alert, App, Button, Card, Flex, Table, Tabs, Tag, Tooltip, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { fetchTrash, purgeTrashItem, restoreTrashItem } from '@/api/admin';
import type { TrashItem, TrashType } from '@/api/types';
import { queryKeys } from '@/app/queryClient';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { EmptyState } from '@/components/EmptyState';
import { PageHeader } from '@/components/PageHeader';
import { PurgeTrashDialog } from './components/PurgeTrashDialog';
import {
  TRASH_RETENTION_DAYS,
  TRASH_TYPE_EMPTY,
  TRASH_TYPE_LABELS,
  autoCleanupDate,
  daysUntilCleanup,
  describeTrashError,
  formatDeletedAt,
} from './trashUtils';

const PAGE_SIZE = 20;
const TRASH_TYPES: TrashType[] = ['ticket', 'kb', 'equipment'];

export function AdminTrashPage() {
  const { message } = App.useApp();
  const client = useQueryClient();

  const [type, setType] = useState<TrashType>('ticket');
  const [page, setPage] = useState(1);
  const [restoreTarget, setRestoreTarget] = useState<TrashItem | null>(null);
  const [purgeTarget, setPurgeTarget] = useState<TrashItem | null>(null);

  const trashQuery = useQuery({
    queryKey: queryKeys.trash(type, page),
    queryFn: () => fetchTrash({ type, page, size: PAGE_SIZE }),
  });

  const items = useMemo(() => trashQuery.data?.items ?? [], [trashQuery.data]);
  const total = trashQuery.data?.total ?? 0;

  const invalidateTrash = useCallback(() => {
    void client.invalidateQueries({ queryKey: ['admin', 'trash'] });
    // Восстановление/удаление затрагивает списки доменов и публичный периметр.
    void client.invalidateQueries({ queryKey: ['tickets'] });
    void client.invalidateQueries({ queryKey: ['public', 'tickets'] });
    void client.invalidateQueries({ queryKey: queryKeys.kbTree });
    void client.invalidateQueries({ queryKey: queryKeys.publicKbTree });
    void client.invalidateQueries({ queryKey: ['equipment'] });
  }, [client]);

  const restoreMutation = useMutation({
    mutationFn: (item: TrashItem) => restoreTrashItem(item.type, item.id),
    onSuccess: (_result, item) => {
      invalidateTrash();
      message.success(`Восстановлено: ${item.title}`);
    },
    onError: (cause) => message.error(describeTrashError(cause)),
  });

  const purgeMutation = useMutation({
    mutationFn: (item: TrashItem) => purgeTrashItem(item.type, item.id),
    onSuccess: (_result, item) => {
      invalidateTrash();
      message.success(`Удалено навсегда: ${item.title}`);
    },
    onError: (cause) => message.error(describeTrashError(cause)),
  });

  const confirmRestore = useCallback(async () => {
    if (!restoreTarget) return;
    const item = restoreTarget;
    try {
      await restoreMutation.mutateAsync(item);
    } finally {
      setRestoreTarget(null);
    }
  }, [restoreMutation, restoreTarget]);

  const confirmPurge = useCallback(async () => {
    if (!purgeTarget) return;
    const item = purgeTarget;
    try {
      await purgeMutation.mutateAsync(item);
    } finally {
      setPurgeTarget(null);
    }
  }, [purgeMutation, purgeTarget]);

  const columns: ColumnsType<TrashItem> = [
    {
      title: 'Элемент',
      dataIndex: 'title',
      key: 'title',
      render: (_value, record) => (
        <Flex vertical gap={4}>
          <Typography.Text strong>{record.title}</Typography.Text>
          {record.description ? (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {record.description}
            </Typography.Text>
          ) : null}
          {record.warnings.map((warning) => (
            <Typography.Text
              key={warning}
              type="warning"
              style={{ fontSize: 12 }}
              data-testid={`trash-warning-${record.type}-${record.id}`}
            >
              <WarningOutlined /> {warning}
            </Typography.Text>
          ))}
        </Flex>
      ),
    },
    {
      title: 'Удалено',
      dataIndex: 'deleted_at',
      key: 'deleted_at',
      width: 160,
      render: (value: string) => formatDeletedAt(value),
    },
    {
      title: 'Кто удалил',
      dataIndex: 'deleted_by_name',
      key: 'deleted_by_name',
      width: 200,
      render: (value: string) => value || '—',
    },
    {
      title: 'Автоочистка',
      key: 'cleanup',
      width: 180,
      render: (_value, record) => {
        const daysLeft = daysUntilCleanup(record.deleted_at);
        return (
          <Flex vertical gap={2}>
            <Typography.Text data-testid={`trash-cleanup-${record.type}-${record.id}`}>
              {autoCleanupDate(record.deleted_at)}
            </Typography.Text>
            <Tag color={daysLeft <= 7 ? 'red' : 'default'} style={{ width: 'fit-content' }}>
              через {daysLeft} дн.
            </Tag>
          </Flex>
        );
      },
    },
    {
      title: '',
      key: 'actions',
      width: 230,
      align: 'right',
      render: (_value, record) => (
        <Flex gap={4} justify="flex-end" wrap>
          <Tooltip title="Вернуть элемент из корзины (TRASH-2)">
            <Button
              size="small"
              icon={<RollbackOutlined />}
              onClick={() => setRestoreTarget(record)}
              data-testid={`trash-restore-${record.type}-${record.id}`}
            >
              Восстановить
            </Button>
          </Tooltip>
          <Tooltip title="Удалить без возможности восстановления (TRASH-3)">
            <Button
              size="small"
              danger
              icon={<DeleteOutlined />}
              onClick={() => setPurgeTarget(record)}
              data-testid={`trash-purge-${record.type}-${record.id}`}
            >
              Навсегда
            </Button>
          </Tooltip>
        </Flex>
      ),
    },
  ];

  return (
    <Flex vertical gap={16} data-testid="admin-trash-page">
      <PageHeader
        title="Корзина"
        description={`Удалённые заявки, разделы БЗ и оборудование. Элементы старше ${TRASH_RETENTION_DAYS} дней удаляются автоматически (TRASH-4).`}
        extra={
          <Button
            icon={<ReloadOutlined />}
            loading={trashQuery.isFetching}
            onClick={() => void trashQuery.refetch()}
            data-testid="trash-refresh"
          >
            Обновить
          </Button>
        }
      />

      <Alert
        type="info"
        showIcon
        message="Восстановление и окончательное удаление"
        description="Заявка восстанавливается со своим статусом и зданием; если здание деактивировано — предупреждаем, публично она будет недоступна. У раздела БЗ восстанавливается только сам узел, потомки — нет (TRASH-5). Окончательное удаление необратимо (TRASH-3)."
        data-testid="trash-rules"
      />

      <Card size="small" data-testid="trash-card">
        <Tabs
          activeKey={type}
          onChange={(key) => {
            setType(key as TrashType);
            setPage(1);
          }}
          items={TRASH_TYPES.map((trashType) => ({
            key: trashType,
            label: TRASH_TYPE_LABELS[trashType],
          }))}
        />

        {trashQuery.isError ? (
          <Alert
            type="error"
            showIcon
            message="Не удалось загрузить корзину"
            action={
              <Button size="small" onClick={() => void trashQuery.refetch()}>
                Повторить
              </Button>
            }
            data-testid="trash-error"
          />
        ) : (
          <Table<TrashItem>
            rowKey={(record) => `${record.type}-${record.id}`}
            size="small"
            columns={columns}
            dataSource={items}
            loading={trashQuery.isFetching}
            scroll={{ x: 900 }}
            pagination={{
              current: page,
              pageSize: PAGE_SIZE,
              total,
              showSizeChanger: false,
              showTotal: (count) => `Всего элементов: ${count}`,
              onChange: setPage,
            }}
            locale={{
              emptyText: (
                <EmptyState
                  compact
                  title={TRASH_TYPE_EMPTY[type]}
                  description="Удалённые элементы появятся здесь и будут храниться 90 дней."
                />
              ),
            }}
          />
        )}
      </Card>

      <ConfirmDialog
        open={restoreTarget !== null}
        title={`Восстановить «${restoreTarget?.title ?? ''}»?`}
        description="Элемент вернётся в рабочие списки и исчезнет из корзины (TRASH-2)."
        warning={restoreTarget?.warnings.length ? restoreTarget.warnings.join(' · ') : undefined}
        okText="Восстановить"
        loading={restoreMutation.isPending}
        onCancel={() => setRestoreTarget(null)}
        onConfirm={confirmRestore}
      />

      <PurgeTrashDialog
        open={purgeTarget !== null}
        item={purgeTarget}
        loading={purgeMutation.isPending}
        onCancel={() => setPurgeTarget(null)}
        onConfirm={confirmPurge}
      />
    </Flex>
  );
}
