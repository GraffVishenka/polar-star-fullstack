/**
 * StatusTag — статус заявки (TICKET-1).
 *
 * Коды статусов фиксированы (`new`, `in_progress`, `deferred`, `done`),
 * а название и цвет приходят с сервера (`status_label`, `status_color`) и
 * настраиваются в Admin-панели. Поэтому цвета и подписи — из пропсов,
 * встроенные значения используются только как fallback.
 */
import { Tag } from 'antd';
import { TICKET_STATUS_COLORS, TICKET_STATUS_LABELS } from '@/app/config';
import type { TicketStatus } from '@/api/types';

export interface StatusTagProps {
  /** Код статуса из API. */
  status: TicketStatus;
  /** `status_label` с сервера; пустое значение → русский текст по умолчанию. */
  label?: string | null;
  /** `status_color` с сервера (hex); пустое значение → цвет по умолчанию. */
  color?: string | null;
  /** Компактный вариант для карточек доски. */
  size?: 'default' | 'small';
  className?: string;
}

export function StatusTag({ status, label, color, size = 'default', className }: StatusTagProps) {
  const text = label?.trim() || TICKET_STATUS_LABELS[status] || status;
  const tagColor = color?.trim() || TICKET_STATUS_COLORS[status] || 'default';

  return (
    <Tag
      className={className}
      color={tagColor}
      data-status={status}
      data-testid={`status-tag-${status}`}
      style={{
        marginInlineEnd: 0,
        fontSize: size === 'small' ? 11 : 12,
        lineHeight: size === 'small' ? '16px' : '20px',
        paddingInline: size === 'small' ? 6 : 8,
      }}
    >
      {text}
    </Tag>
  );
}
