/**
 * Создание и переименование склада/кабинета (EQ-1, только ADMIN).
 *
 * Имя уникально в рамках здания: `409 CONFLICT` подсвечивается прямо у поля,
 * чтобы администратор сразу видел причину, а не общий «конфликт данных».
 */
import { useEffect, useState } from 'react';
import { Alert, Form, Input, Modal, Typography } from 'antd';
import type { Warehouse } from '@/api/types';
import { describeInfraFieldError } from '../../errors';
import { useCreateWarehouse, useUpdateWarehouse } from '../useBuildingsAdmin';

interface FormValues {
  name: string;
}

export interface WarehouseFormModalProps {
  open: boolean;
  buildingId: number | null;
  buildingName: string | null;
  /** `null` — создание. */
  warehouse: Warehouse | null;
  onClose: () => void;
  onSaved: (warehouse: Warehouse) => void;
}

export function WarehouseFormModal({
  open,
  buildingId,
  buildingName,
  warehouse,
  onClose,
  onSaved,
}: WarehouseFormModalProps) {
  const [form] = Form.useForm<FormValues>();
  const [error, setError] = useState<string | null>(null);
  const createMutation = useCreateWarehouse();
  const updateMutation = useUpdateWarehouse();
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (!open) return;
    setError(null);
    createMutation.reset();
    updateMutation.reset();
    form.setFieldsValue({ name: warehouse?.name ?? '' });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- сброс только при открытии
  }, [open, warehouse?.id]);

  const submit = () => {
    setError(null);
    form
      .validateFields()
      .then(async (values) => {
        const name = values.name.trim();
        try {
          if (warehouse) {
            const saved = await updateMutation.mutateAsync({ id: warehouse.id, body: { name } });
            onSaved(saved);
          } else {
            const saved = await createMutation.mutateAsync({
              building_id: buildingId as number,
              name,
            });
            onSaved(saved);
          }
          onClose();
        } catch (cause) {
          const message = describeInfraFieldError(cause);
          setError(message);
          // Ошибка уникальности имени приходит как 409 CONFLICT — показываем у поля.
          form.setFields([{ name: 'name', errors: message ? [message] : [] }]);
        }
      })
      .catch(() => undefined);
  };

  return (
    <Modal
      open={open}
      title={warehouse ? 'Переименование помещения' : 'Новое помещение'}
      okText={warehouse ? 'Сохранить' : 'Создать'}
      cancelText="Отмена"
      confirmLoading={pending}
      okButtonProps={{ 'data-testid': 'warehouse-form-submit' }}
      cancelButtonProps={{ 'data-testid': 'warehouse-form-cancel' }}
      onOk={submit}
      onCancel={onClose}
      maskClosable={false}
    >
      <div data-testid="warehouse-form">
        {error ? (
          <Alert
            type="error"
            showIcon
            message={error}
            style={{ marginBottom: 12 }}
            data-testid="warehouse-form-error"
          />
        ) : null}
        <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
          Здание: <Typography.Text strong>{buildingName ?? '—'}</Typography.Text>
          <br />
          Имя помещения уникально в рамках здания (EQ-1).
        </Typography.Paragraph>
        <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
          <Form.Item
            label="Название помещения"
            name="name"
            rules={[
              { required: true, message: 'Укажите название помещения' },
              { max: 200, message: 'Не больше 200 символов' },
            ]}
            style={{ marginBottom: 0 }}
          >
            <Input placeholder="Например: Склад ИТ" maxLength={200} showCount />
          </Form.Item>
        </Form>
      </div>
    </Modal>
  );
}
