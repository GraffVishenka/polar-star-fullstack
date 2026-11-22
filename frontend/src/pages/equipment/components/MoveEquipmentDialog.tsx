/**
 * Диалог перемещения единицы оборудования (EQ-3).
 *
 * Сценарий: здание → склад/кабинет → комментарий. Сервер проверяет правила:
 *  - тот же склад → `409 SAME_WAREHOUSE` (подсвечиваем ошибку в диалоге);
 *  - неактивное здание → `422 BUILDING_INACTIVE` для USER (в списке таких
 *    зданий нет, но ошибку сервера всё равно показываем понятным текстом).
 * После успеха карточка и журнал обновляются инвалидацией запросов — без F5.
 */
import { useEffect, useState } from 'react';
import { Alert, Form, Input, Modal, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { moveEquipment } from '@/api/equipment';
import type { Building, Equipment, Warehouse } from '@/api/types';
import { describeEquipmentError } from '../errors';
import { EntityName } from './InactiveTag';
import { LocationFields } from './LocationFields';

interface FormValues {
  building_id: number;
  to_warehouse_id: number;
  comment?: string;
}

export interface MoveEquipmentDialogProps {
  open: boolean;
  equipment: Equipment | null;
  buildings: Building[];
  warehouses: Warehouse[];
  isAdmin: boolean;
  onClose: () => void;
  onMoved: (equipment: Equipment) => void;
}

export function MoveEquipmentDialog({
  open,
  equipment,
  buildings,
  warehouses,
  isAdmin,
  onClose,
  onMoved,
}: MoveEquipmentDialogProps) {
  const [form] = Form.useForm<FormValues>();
  const [error, setError] = useState<string | null>(null);

  const currentBuilding = buildings.find((building) => building.id === equipment?.building_id);
  const currentBuildingSelectable = isAdmin || currentBuilding?.is_active !== false;

  const mutation = useMutation({
    mutationFn: (values: FormValues) =>
      moveEquipment(equipment?.id as number, {
        to_warehouse_id: values.to_warehouse_id,
        comment: values.comment?.trim() || undefined,
      }),
    onSuccess: (updated) => {
      onMoved(updated);
      onClose();
    },
    onError: (cause) => setError(describeEquipmentError(cause, 'move')),
  });

  // Каждое открытие начинается с текущего здания и пустых полей склада/комментария.
  useEffect(() => {
    if (!open) return;
    setError(null);
    mutation.reset();
    form.setFieldsValue({
      building_id: currentBuildingSelectable ? equipment?.building_id : undefined,
      to_warehouse_id: undefined,
      comment: undefined,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- сброс только при открытии
  }, [open, equipment?.id]);

  const submit = () => {
    setError(null);
    form
      .validateFields()
      .then((values) => mutation.mutate(values))
      .catch(() => undefined);
  };

  return (
    <Modal
      open={open}
      title="Перемещение оборудования"
      okText="Переместить"
      cancelText="Отмена"
      confirmLoading={mutation.isPending}
      okButtonProps={{ 'data-testid': 'move-submit' }}
      cancelButtonProps={{ 'data-testid': 'move-cancel' }}
      onOk={submit}
      onCancel={onClose}
      maskClosable={false}
    >
      <div data-testid="move-dialog">
        {equipment ? (
          <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
            <Typography.Text strong>{equipment.name}</Typography.Text> ·{' '}
            {equipment.registration_number}
            <br />
            Сейчас:{' '}
            <EntityName name={equipment.building_name} inactive={!equipment.is_building_active} />,
            «{equipment.warehouse_name}»
          </Typography.Paragraph>
        ) : null}

        {error ? (
          <Alert
            type="error"
            showIcon
            message={error}
            style={{ marginBottom: 12 }}
            data-testid="move-error"
          />
        ) : null}

        <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
          <LocationFields buildings={buildings} warehouses={warehouses} isAdmin={isAdmin} />
          <Form.Item label="Комментарий" name="comment" style={{ marginBottom: 0 }}>
            <Input.TextArea
              rows={2}
              maxLength={500}
              showCount
              placeholder="Например: перенос на склад ИТ по акту"
              data-testid="move-comment"
            />
          </Form.Item>
        </Form>
      </div>
    </Modal>
  );
}
