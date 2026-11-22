/**
 * Окончательное удаление из корзины (TRASH-3).
 *
 * Действие необратимо, поэтому кнопка активируется только после ввода слова
 * `УДАЛИТЬ`: это защищает от случайного клика и совпадает с требованием
 * «окончательное удаление с подтверждением в UI».
 */
import { useEffect, useState } from 'react';
import { Alert, Input, Modal, Typography } from 'antd';
import type { TrashItem } from '@/api/types';
import { TRASH_PURGE_CONFIRMATION, TRASH_TYPE_LABELS } from '../trashUtils';

export interface PurgeTrashDialogProps {
  open: boolean;
  item: TrashItem | null;
  loading?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

export function PurgeTrashDialog({
  open,
  item,
  loading = false,
  onCancel,
  onConfirm,
}: PurgeTrashDialogProps) {
  const [value, setValue] = useState('');

  useEffect(() => {
    if (open) setValue('');
  }, [open, item?.id, item?.type]);

  const confirmed = value.trim().toUpperCase() === TRASH_PURGE_CONFIRMATION;

  return (
    <Modal
      open={open}
      title="Удалить навсегда?"
      okText="Удалить навсегда"
      cancelText="Отмена"
      okButtonProps={{ danger: true, disabled: !confirmed, 'data-testid': 'purge-submit' }}
      cancelButtonProps={{ 'data-testid': 'purge-cancel' }}
      confirmLoading={loading}
      onOk={onConfirm}
      onCancel={onCancel}
      maskClosable={false}
    >
      <div data-testid="purge-dialog">
        <Alert
          type="error"
          showIcon
          message="Действие необратимо"
          description="Элемент, его комментарии и вложения будут удалены без возможности восстановления (TRASH-3)."
          style={{ marginBottom: 12 }}
        />
        {item ? (
          <Typography.Paragraph style={{ fontSize: 13 }}>
            {TRASH_TYPE_LABELS[item.type]} · <Typography.Text strong>{item.title}</Typography.Text>
            {item.description ? ` — ${item.description}` : ''}
          </Typography.Paragraph>
        ) : null}
        <Typography.Paragraph style={{ fontSize: 13, marginBottom: 4 }}>
          Для подтверждения введите слово <Typography.Text code>УДАЛИТЬ</Typography.Text>:
        </Typography.Paragraph>
        <Input
          value={value}
          onChange={(event) => setValue(event.target.value)}
          placeholder="УДАЛИТЬ"
          aria-label="Подтверждение окончательного удаления"
          data-testid="purge-confirm-input"
        />
      </div>
    </Modal>
  );
}
