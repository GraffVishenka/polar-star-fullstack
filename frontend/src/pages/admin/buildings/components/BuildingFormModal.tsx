/**
 * Создание и редактирование здания (EQ-1, только ADMIN).
 *
 * Поля: название (1–200) и адрес (необязательный, ≤500). Активность меняется
 * отдельным переключателем в таблице — здесь её намеренно нет, чтобы случайно
 * не спрятать здание вместе с правкой названия.
 */
import { useEffect, useState } from 'react';
import { Alert, Form, Input, Modal } from 'antd';
import type { Building } from '@/api/types';
import { describeInfraError } from '../../errors';
import { useCreateBuilding, useUpdateBuilding } from '../useBuildingsAdmin';

interface FormValues {
  name: string;
  address?: string;
}

export interface BuildingFormModalProps {
  open: boolean;
  /** `null` — создание. */
  building: Building | null;
  onClose: () => void;
  onSaved: (building: Building) => void;
}

export function BuildingFormModal({ open, building, onClose, onSaved }: BuildingFormModalProps) {
  const [form] = Form.useForm<FormValues>();
  const [error, setError] = useState<string | null>(null);
  const createMutation = useCreateBuilding();
  const updateMutation = useUpdateBuilding();
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (!open) return;
    setError(null);
    createMutation.reset();
    updateMutation.reset();
    form.setFieldsValue({ name: building?.name ?? '', address: building?.address ?? '' });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- сброс только при открытии
  }, [open, building?.id]);

  const submit = () => {
    setError(null);
    form
      .validateFields()
      .then(async (values) => {
        const body = {
          name: values.name.trim(),
          address: values.address?.trim() ? values.address.trim() : null,
        };
        try {
          const saved = building
            ? await updateMutation.mutateAsync({ id: building.id, body })
            : await createMutation.mutateAsync(body);
          onSaved(saved);
          onClose();
        } catch (cause) {
          setError(describeInfraError(cause).message);
        }
      })
      .catch(() => undefined);
  };

  return (
    <Modal
      open={open}
      title={building ? 'Редактирование здания' : 'Новое здание'}
      okText={building ? 'Сохранить' : 'Создать'}
      cancelText="Отмена"
      confirmLoading={pending}
      okButtonProps={{ 'data-testid': 'building-form-submit' }}
      cancelButtonProps={{ 'data-testid': 'building-form-cancel' }}
      onOk={submit}
      onCancel={onClose}
      maskClosable={false}
    >
      <div data-testid="building-form">
        {error ? (
          <Alert
            type="error"
            showIcon
            message={error}
            style={{ marginBottom: 12 }}
            data-testid="building-form-error"
          />
        ) : null}
        <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
          <Form.Item
            label="Название"
            name="name"
            rules={[
              { required: true, message: 'Укажите название здания' },
              { max: 200, message: 'Не больше 200 символов' },
            ]}
          >
            <Input placeholder="Например: Центральный офис" maxLength={200} showCount />
          </Form.Item>
          <Form.Item
            label="Адрес"
            name="address"
            rules={[{ max: 500, message: 'Не больше 500 символов' }]}
            style={{ marginBottom: 0 }}
          >
            <Input placeholder="ул. Ленина, 24" maxLength={500} />
          </Form.Item>
        </Form>
      </div>
    </Modal>
  );
}
