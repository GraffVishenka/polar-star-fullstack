/**
 * `/admin/dictionaries` — виды проблем и типы оборудования (Ф6).
 *
 * Справочники нельзя удалить из UI: значение используется в исторических
 * заявках и оборудовании, поэтому вместо удаления — деактивация. В таблице
 * видно, сколько действующих записей ссылается на значение (`usage_count`),
 * чтобы администратор понимал последствия деактивации.
 */
import { useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Flex,
  Form,
  Input,
  InputNumber,
  Modal,
  Space,
  Switch,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { TableColumnsType } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  createEquipmentType,
  createProblemType,
  fetchEquipmentTypes,
  fetchProblemTypes,
  updateEquipmentType,
  updateProblemType,
  type DictionaryItemPayload,
} from '@/api/admin';
import type { EquipmentType, ProblemType } from '@/api/types';
import { queryKeys } from '@/app/queryClient';
import { toast } from '@/app/toast';
import { EmptyState } from '@/components/EmptyState';
import { PageHeader } from '@/components/PageHeader';
import { adminErrorMessage } from '../shared';

type DictionaryKind = 'problem' | 'equipment';
type DictionaryItem = ProblemType | EquipmentType;

interface DictionaryConfig {
  title: string;
  singular: string;
  addLabel: string;
  queryKey: readonly unknown[];
  list: () => Promise<DictionaryItem[]>;
  create: (body: DictionaryItemPayload & { name: string }) => Promise<DictionaryItem>;
  update: (id: number, body: DictionaryItemPayload) => Promise<DictionaryItem>;
}

const DICTIONARIES: Record<DictionaryKind, DictionaryConfig> = {
  problem: {
    title: 'Виды проблем',
    singular: 'вид проблемы',
    addLabel: 'Добавить вид проблемы',
    queryKey: queryKeys.problemTypes,
    list: fetchProblemTypes,
    create: createProblemType,
    update: updateProblemType,
  },
  equipment: {
    title: 'Типы оборудования',
    singular: 'тип оборудования',
    addLabel: 'Добавить тип оборудования',
    queryKey: queryKeys.equipmentTypes,
    list: fetchEquipmentTypes,
    create: createEquipmentType,
    update: updateEquipmentType,
  },
};

interface DictionaryFormValues {
  name: string;
  sort_order?: number | null;
}

interface DictionaryFormModalProps {
  open: boolean;
  mode: 'create' | 'edit';
  config: DictionaryConfig;
  item: DictionaryItem | null;
  pending: boolean;
  error: unknown;
  onCancel: () => void;
  onSubmit: (values: DictionaryFormValues) => void;
}

function DictionaryFormModal({
  open,
  mode,
  config,
  item,
  pending,
  error,
  onCancel,
  onSubmit,
}: DictionaryFormModalProps) {
  const [form] = Form.useForm<DictionaryFormValues>();

  return (
    <Modal
      open={open}
      title={mode === 'create' ? config.addLabel : `Изменить: ${item?.name ?? ''}`}
      okText="Сохранить"
      cancelText="Отмена"
      confirmLoading={pending}
      onOk={() => void form.submit()}
      onCancel={onCancel}
      destroyOnHidden
      data-testid="dictionary-form-modal"
    >
      {error ? (
        <Alert
          type="error"
          showIcon
          message={adminErrorMessage(error, 'Не удалось сохранить справочник')}
          data-testid="dictionary-form-error"
          style={{ marginBottom: 12 }}
        />
      ) : null}
      <Form<DictionaryFormValues>
        form={form}
        layout="vertical"
        preserve={false}
        initialValues={{ name: item?.name ?? '', sort_order: item?.sort_order ?? null }}
        onFinish={(values) => onSubmit(values)}
      >
        <Form.Item
          name="name"
          label="Название"
          rules={[
            { required: true, message: 'Укажите название' },
            { max: 100, message: 'Не больше 100 символов' },
          ]}
        >
          <Input
            placeholder="Например, Освещение"
            data-testid="dictionary-name-input"
            autoComplete="off"
          />
        </Form.Item>
        <Form.Item
          name="sort_order"
          label="Порядок сортировки"
          extra="Меньшее значение — выше в списках. Пусто — в конец (шаг 10)."
        >
          <InputNumber
            min={0}
            max={100_000}
            step={10}
            style={{ width: '100%' }}
            data-testid="dictionary-sort-input"
            placeholder="Например, 10"
          />
        </Form.Item>
      </Form>
    </Modal>
  );
}

function DictionarySection({ kind }: { kind: DictionaryKind }) {
  const config = DICTIONARIES[kind];
  const queryClient = useQueryClient();
  const [modal, setModal] = useState<{ mode: 'create' | 'edit'; item: DictionaryItem | null } | null>(
    null,
  );
  const [pageError, setPageError] = useState<string | null>(null);

  const query = useQuery({ queryKey: config.queryKey, queryFn: config.list });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: config.queryKey });
    void queryClient.invalidateQueries({ queryKey: queryKeys.settings });
  };

  const saveMutation = useMutation({
    mutationFn: (input: { mode: 'create' | 'edit'; item: DictionaryItem | null; values: DictionaryFormValues }) => {
      const payload: DictionaryItemPayload & { name: string } = {
        name: (input.values.name ?? '').trim(),
        sort_order:
          input.values.sort_order === null || input.values.sort_order === undefined
            ? undefined
            : Number(input.values.sort_order),
      };
      return input.mode === 'create'
        ? config.create(payload)
        : config.update(input.item?.id as number, payload);
    },
    onSuccess: () => {
      setModal(null);
      toast.success('Справочник сохранён');
    },
    onSettled: invalidate,
  });

  const toggleMutation = useMutation({
    mutationFn: (input: { item: DictionaryItem; is_active: boolean }) =>
      config.update(input.item.id, { is_active: input.is_active }),
    onError: (error) => setPageError(adminErrorMessage(error, 'Не удалось изменить справочник')),
    onSuccess: (_result, input) => {
      setPageError(null);
      toast.success(
        input.is_active ? 'Справочник активирован' : 'Справочник деактивирован и скрыт из форм',
      );
    },
    onSettled: invalidate,
  });

  const columns: TableColumnsType<DictionaryItem> = [
    {
      title: 'Название',
      dataIndex: 'name',
      key: 'name',
      render: (name: string, item) => (
        <Space size={6} wrap>
          <Typography.Text strong={item.is_active !== false} type={item.is_active === false ? 'secondary' : undefined}>
            {name}
          </Typography.Text>
          {item.is_active === false ? (
            <Tag data-testid={`dictionary-inactive-${kind}-${item.id}`}>неактивен</Tag>
          ) : null}
        </Space>
      ),
    },
    {
      title: 'Порядок',
      dataIndex: 'sort_order',
      key: 'sort_order',
      width: 100,
      render: (value: number | undefined) => (
        <Typography.Text type="secondary">{value ?? '—'}</Typography.Text>
      ),
    },
    {
      title: 'Используется',
      dataIndex: 'usage_count',
      key: 'usage_count',
      width: 190,
      render: (value: number | undefined, item) => {
        const count = value ?? 0;
        return count > 0 ? (
          <Tooltip title="Значение останется в старых записях даже после деактивации">
            <Tag color="blue" data-testid={`dictionary-usage-${kind}-${item.id}`}>
              {count === 1 ? '1 запись' : `${count} записей`}
            </Tag>
          </Tooltip>
        ) : (
          <Typography.Text type="secondary" data-testid={`dictionary-usage-${kind}-${item.id}`}>
            не используется
          </Typography.Text>
        );
      },
    },
    {
      title: 'Активен',
      dataIndex: 'is_active',
      key: 'is_active',
      width: 120,
      render: (isActive: boolean | undefined, item) => (
        <Switch
          checked={isActive !== false}
          checkedChildren="да"
          unCheckedChildren="нет"
          loading={toggleMutation.isPending && toggleMutation.variables?.item.id === item.id}
          onChange={(checked) => toggleMutation.mutate({ item, is_active: checked })}
          data-testid={`dictionary-active-${kind}-${item.id}`}
        />
      ),
    },
    {
      title: 'Действия',
      key: 'actions',
      width: 170,
      render: (_value, item) => (
        <Space size={4} wrap>
          <Button
            size="small"
            onClick={() => setModal({ mode: 'edit', item })}
            data-testid={`dictionary-edit-${kind}-${item.id}`}
          >
            Изменить
          </Button>
          <Tooltip title="Удаление справочника запрещено: деактивируйте его — старые записи сохранят значение">
            <Button
              size="small"
              danger
              icon={<DeleteOutlined />}
              disabled
              aria-label={`Удаление запрещено: ${item.name}`}
              data-testid={`dictionary-delete-${kind}-${item.id}`}
            />
          </Tooltip>
        </Space>
      ),
    },
  ];

  return (
    <Flex vertical gap={12}>
      <Flex justify="space-between" align="center" gap={8} wrap>
        <Typography.Text type="secondary" style={{ fontSize: 13 }}>
          Справочники не удаляются: деактивированные значения остаются в старых записях.
        </Typography.Text>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => {
            saveMutation.reset();
            setModal({ mode: 'create', item: null });
          }}
          data-testid={`dictionary-add-${kind}`}
        >
          Добавить
        </Button>
      </Flex>

      {pageError ? (
        <Alert
          type="error"
          showIcon
          closable
          message={pageError}
          data-testid="dictionary-error"
          onClose={() => setPageError(null)}
        />
      ) : null}

      {query.error ? (
        <Alert
          type="error"
          showIcon
          message={adminErrorMessage(query.error, 'Не удалось загрузить справочник')}
          data-testid="dictionary-load-error"
        />
      ) : null}

      <Table<DictionaryItem>
        rowKey="id"
        size="middle"
        columns={columns}
        dataSource={query.data ?? []}
        loading={query.isLoading}
        pagination={false}
        data-testid={`dictionary-table-${kind}`}
        locale={{
          emptyText: (
            <EmptyState
              compact
              title="Справочник пуст"
              description="Добавьте первое значение — оно появится в формах."
            />
          ),
        }}
      />

      <DictionaryFormModal
        key={modal?.mode === 'edit' ? `edit-${modal.item?.id}` : 'create'}
        open={modal !== null}
        mode={modal?.mode ?? 'create'}
        config={config}
        item={modal?.item ?? null}
        pending={saveMutation.isPending}
        error={saveMutation.error}
        onCancel={() => {
          setModal(null);
          saveMutation.reset();
        }}
        onSubmit={(values) =>
          saveMutation.mutate({
            mode: modal?.mode ?? 'create',
            item: modal?.item ?? null,
            values,
          })
        }
      />
    </Flex>
  );
}

export function DictionariesPage() {
  return (
    <Flex vertical gap={16} data-testid="dictionaries-page">
      <PageHeader
        title="Справочники"
        description="Виды проблем и типы оборудования. Значения из истории не удаляются — только деактивируются."
      />
      <Alert
        type="info"
        showIcon
        message="Удаление запрещено"
        description="Деактивированный справочник исчезает из новых форм, но остаётся в уже созданных заявках и оборудовании. Поэтому кнопка «Удалить» недоступна."
        data-testid="dictionaries-info"
      />
      <Card size="small">
        <Tabs
          defaultActiveKey="problem"
          data-testid="dictionaries-tabs"
          items={[
            {
              key: 'problem',
              label: 'Виды проблем',
              children: <DictionarySection kind="problem" />,
            },
            {
              key: 'equipment',
              label: 'Типы оборудования',
              children: <DictionarySection kind="equipment" />,
            },
          ]}
        />
      </Card>
    </Flex>
  );
}
