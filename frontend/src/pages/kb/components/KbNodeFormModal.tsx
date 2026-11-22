/**
 * Модалка создания раздела/подраздела (Ф3, KB-1, KB-2).
 *
 * Поля: заголовок (1–200) и родитель. Родитель по умолчанию — выбранный узел
 * (для «Добавить подраздел») или корень дерева (для «Добавить раздел»).
 * Ошибки сервера (400 «родитель не найден», 409 и пр.) показываются текстом
 * в самой модалке, поля при этом не сбрасываются.
 */
import { useEffect } from 'react';
import { Alert, Form, Input, Modal, TreeSelect, Typography } from 'antd';
import type { KbTreeNode } from '@/api/types';
import { kbErrorMessage } from '../hooks';

export interface KbNodeFormValues {
  title: string;
  parent_id: number | null;
}

export interface KbNodeFormModalProps {
  open: boolean;
  /** `root` — новый корневой раздел, `child` — подраздел выбранного узла. */
  mode: 'root' | 'child';
  tree: KbTreeNode[];
  defaultParentId: number | null;
  pending?: boolean;
  /** Ошибка последней попытки сохранения (ApiError или строка). */
  error?: unknown;
  onCancel: () => void;
  onSubmit: (values: KbNodeFormValues) => void | Promise<void>;
}

export interface KbTreeSelectNode {
  value: number;
  title: string;
  children?: KbTreeSelectNode[];
}

/** Дерево для TreeSelect: вложенность любая, подписи — заголовки разделов. */
export function buildKbTreeSelectData(nodes: KbTreeNode[]): KbTreeSelectNode[] {
  return nodes.map((node) => ({
    value: node.id,
    title: node.title,
    children: node.children.length ? buildKbTreeSelectData(node.children) : undefined,
  }));
}

export function KbNodeFormModal({
  open,
  mode,
  tree,
  defaultParentId,
  pending = false,
  error,
  onCancel,
  onSubmit,
}: KbNodeFormModalProps) {
  const [form] = Form.useForm<KbNodeFormValues>();

  useEffect(() => {
    if (!open) return;
    form.setFieldsValue({ title: '', parent_id: mode === 'child' ? defaultParentId : null });
  }, [open, mode, defaultParentId, form]);

  const treeData = buildKbTreeSelectData(tree);

  return (
    <Modal
      open={open}
      title={mode === 'root' ? 'Новый раздел' : 'Новый подраздел'}
      okText="Создать"
      cancelText="Отмена"
      confirmLoading={pending}
      onOk={() => void form.submit()}
      onCancel={onCancel}
      destroyOnHidden
      data-testid="kb-node-modal"
    >
      <Form<KbNodeFormValues>
        form={form}
        layout="vertical"
        preserve={false}
        onFinish={(values) => void onSubmit(values)}
        data-testid="kb-node-form"
      >
        <Form.Item
          name="title"
          label="Заголовок"
          rules={[
            { required: true, message: 'Укажите заголовок раздела' },
            { max: 200, message: 'Не больше 200 символов' },
            {
              validator: (_rule, value: string) =>
                typeof value === 'string' && value.trim().length > 0
                  ? Promise.resolve()
                  : Promise.reject(new Error('Заголовок не может состоять из пробелов')),
            },
          ]}
        >
          <Input
            placeholder="Например, порядок работы с заявками"
            maxLength={200}
            showCount
            autoFocus
            data-testid="kb-node-title-input"
          />
        </Form.Item>

        <Form.Item name="parent_id" label="Родительский раздел">
          <TreeSelect
            allowClear
            treeDefaultExpandAll
            placeholder="Корень базы знаний"
            treeData={treeData}
            data-testid="kb-node-parent-select"
          />
        </Form.Item>

        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Вложенность не ограничена: подраздел можно создать на любом уровне.
        </Typography.Text>

        {error ? (
          <Alert
            type="error"
            showIcon
            style={{ marginTop: 12 }}
            message={kbErrorMessage(error, 'Не удалось создать раздел')}
            data-testid="kb-node-form-error"
          />
        ) : null}
      </Form>
    </Modal>
  );
}
