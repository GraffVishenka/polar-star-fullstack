/**
 * Массовое перемещение оборудования (EQ-4, до 200 единиц).
 *
 * Три шага: выбор здания/склада → подтверждение → итог операции.
 * Ошибки сервера: `404` (одной из единиц уже нет — откат, обновите список) и
 * `422` (некорректный набор: пусто, >200, неактивное здание для USER).
 * После успеха список и журнал обновляются инвалидацией — без перезагрузки.
 */
import { useEffect, useState } from 'react';
import { Alert, Button, Flex, Form, Input, Modal, Typography } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { bulkMoveEquipment } from '@/api/equipment';
import type { Building, EquipmentBulkMoveResponse, Warehouse } from '@/api/types';
import { describeEquipmentError } from '../errors';
import { LocationFields } from './LocationFields';

interface FormValues {
  building_id: number;
  to_warehouse_id: number;
  comment?: string;
}

type Stage = 'form' | 'confirm' | 'result';

export interface BulkMoveDialogProps {
  open: boolean;
  equipmentIds: number[];
  buildings: Building[];
  warehouses: Warehouse[];
  isAdmin: boolean;
  onClose: () => void;
  onCompleted: (result: EquipmentBulkMoveResponse) => void;
}

export function BulkMoveDialog({
  open,
  equipmentIds,
  buildings,
  warehouses,
  isAdmin,
  onClose,
  onCompleted,
}: BulkMoveDialogProps) {
  const [form] = Form.useForm<FormValues>();
  const [stage, setStage] = useState<Stage>('form');
  const [values, setValues] = useState<FormValues | null>(null);
  const [result, setResult] = useState<EquipmentBulkMoveResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setStage('form');
    setValues(null);
    setResult(null);
    setError(null);
    form.resetFields();
  }, [open, form]);

  const mutation = useMutation({
    mutationFn: (payload: FormValues) =>
      bulkMoveEquipment({
        equipment_ids: equipmentIds,
        to_warehouse_id: payload.to_warehouse_id,
        comment: payload.comment?.trim() || undefined,
      }),
    onSuccess: (response) => {
      setResult(response);
      setStage('result');
      onCompleted(response);
    },
    onError: (cause) => {
      setError(describeEquipmentError(cause, 'bulk'));
      setStage('form');
    },
  });

  const goToConfirm = () => {
    setError(null);
    form
      .validateFields()
      .then((next) => {
        setValues(next);
        setStage('confirm');
      })
      .catch(() => undefined);
  };

  const confirm = () => {
    if (values) mutation.mutate(values);
  };

  const targetBuilding = buildings.find((building) => building.id === values?.building_id);
  const targetWarehouse = warehouses.find((warehouse) => warehouse.id === values?.to_warehouse_id);

  const footer =
    stage === 'result' ? (
      <Button type="primary" onClick={onClose} data-testid="bulk-done">
        Готово
      </Button>
    ) : stage === 'confirm' ? (
      <Flex gap={8} justify="flex-end">
        <Button onClick={() => setStage('form')} data-testid="bulk-back">
          Назад
        </Button>
        <Button
          type="primary"
          loading={mutation.isPending}
          onClick={confirm}
          data-testid="bulk-confirm"
        >
          Переместить {equipmentIds.length}
        </Button>
      </Flex>
    ) : (
      <Flex gap={8} justify="flex-end">
        <Button onClick={onClose} data-testid="bulk-cancel">
          Отмена
        </Button>
        <Button type="primary" onClick={goToConfirm} data-testid="bulk-next">
          Продолжить
        </Button>
      </Flex>
    );

  return (
    <Modal
      open={open}
      title={`Массовое перемещение (${equipmentIds.length})`}
      footer={footer}
      onCancel={onClose}
      maskClosable={false}
    >
      <div data-testid="bulk-dialog">
        {error ? (
          <Alert
            type="error"
            showIcon
            message={error}
            style={{ marginBottom: 12 }}
            data-testid="bulk-error"
          />
        ) : null}

        {stage === 'form' ? (
          <>
            <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
              Выбрано единиц: <Typography.Text strong>{equipmentIds.length}</Typography.Text>. Все
              перемещения выполняются одной операцией и попадают в журнал (EQ-4).
            </Typography.Paragraph>
            <Form form={form} layout="vertical" requiredMark={false}>
              <LocationFields buildings={buildings} warehouses={warehouses} isAdmin={isAdmin} />
              <Form.Item label="Комментарий" name="comment" style={{ marginBottom: 0 }}>
                <Input.TextArea
                  rows={2}
                  maxLength={500}
                  showCount
                  placeholder="Например: инвентаризация склада"
                  data-testid="bulk-comment"
                />
              </Form.Item>
            </Form>
          </>
        ) : null}

        {stage === 'confirm' ? (
          <Alert
            type="warning"
            showIcon
            message={`Переместить ${equipmentIds.length} ед. в «${targetWarehouse?.name ?? '—'}»?`}
            description={
              <span>
                Здание: {targetBuilding?.name ?? '—'}. Для каждой единицы будет создана запись в
                журнале перемещений; отменить операцию нельзя.
              </span>
            }
            data-testid="bulk-confirm-summary"
          />
        ) : null}

        {stage === 'result' && result ? (
          <Alert
            type="success"
            showIcon
            message={`Перемещено единиц: ${result.moved_count}`}
            description={`Помещение: "${result.to_warehouse_name}". Журнал перемещений обновлён.`}
            data-testid="bulk-result"
          />
        ) : null}
      </div>
    </Modal>
  );
}
