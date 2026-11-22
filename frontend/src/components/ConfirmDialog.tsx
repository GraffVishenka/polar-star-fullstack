/**
 * ConfirmDialog — подтверждение опасных действий.
 *
 * Используется для «В корзину» (TICKET-12), окончательного удаления (TRASH-3),
 * удаления узла БЗ (KB-3) и сброса исполнителя при смене здания (TICKET-9).
 * Кнопка «ОК» блокируется на время асинхронного действия, повторные клики
 * не создают дублей запросов.
 */
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { Modal, Typography } from 'antd';

export interface ConfirmDialogProps {
  open: boolean;
  title: ReactNode;
  description?: ReactNode;
  /** Дополнительное предупреждение (например, о сбросе исполнителя). */
  warning?: ReactNode;
  okText?: string;
  cancelText?: string;
  /** Красная кнопка подтверждения. */
  danger?: boolean;
  /** Внешняя блокировка (например, мутация в родителе). */
  loading?: boolean;
  onConfirm: () => void | Promise<void>;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  description,
  warning,
  okText = 'Подтвердить',
  cancelText = 'Отмена',
  danger = false,
  loading = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const [pending, setPending] = useState(false);

  useEffect(() => {
    if (!open) setPending(false);
  }, [open]);

  const handleConfirm = useCallback(async () => {
    setPending(true);
    try {
      await onConfirm();
    } finally {
      setPending(false);
    }
  }, [onConfirm]);

  return (
    <Modal
      open={open}
      title={title}
      okText={okText}
      cancelText={cancelText}
      okButtonProps={{ danger, 'data-testid': 'confirm-dialog-ok' }}
      cancelButtonProps={{ 'data-testid': 'confirm-dialog-cancel' }}
      confirmLoading={loading || pending}
      maskClosable={false}
      closable={!pending}
      keyboard={!pending}
      onOk={() => void handleConfirm()}
      onCancel={onCancel}
    >
      <div data-testid="confirm-dialog-body">
        {description ? (
          <Typography.Paragraph style={{ marginBottom: warning ? 12 : 0 }}>
            {description}
          </Typography.Paragraph>
        ) : null}
        {warning ? (
          <Typography.Text type="warning" data-testid="confirm-dialog-warning">
            {warning}
          </Typography.Text>
        ) : null}
      </div>
    </Modal>
  );
}
