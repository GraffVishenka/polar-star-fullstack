/**
 * DeadlineBadge — дедлайн заявки и признак просрочки.
 *
 * ВАЖНО (TICKET-16 / NFR-7): просрочку считает ТОЛЬКО сервер в настройке
 * `timezone`. Компонент не сравнивает дату с «сегодня», иначе на клиенте
 * получались бы расхождения с бэкендом (часовой пояс, рабочий календарь).
 * Флаг `is_overdue` приходит в ответе API.
 */
import { Space, Tag, Typography } from 'antd';
import { ClockCircleOutlined, WarningOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';

export interface DeadlineBadgeProps {
  /** `deadline` (DATE `YYYY-MM-DD` или ISO datetime) либо null. */
  deadline?: string | null;
  /** Флаг просрочки с сервера (TICKET-16). */
  isOverdue?: boolean;
  /** Дата завершения: у завершённой заявки дедлайн показываем нейтрально. */
  completedAt?: string | null;
  /** Компактный вариант для карточек. */
  size?: 'default' | 'small';
  /** Показать подпись «Дедлайн:» перед датой. */
  withLabel?: boolean;
}

/** DATE/ISO → `DD.MM.YYYY`; некорректная дата → исходная строка. */
export function formatDeadline(deadline: string): string {
  const parsed = dayjs(deadline);
  return parsed.isValid() ? parsed.format('DD.MM.YYYY') : deadline;
}

export function DeadlineBadge({
  deadline,
  isOverdue = false,
  completedAt,
  size = 'default',
  withLabel = false,
}: DeadlineBadgeProps) {
  if (!deadline) {
    return (
      <Typography.Text type="secondary" data-testid="deadline-badge-empty">
        {withLabel ? 'Дедлайн: —' : '—'}
      </Typography.Text>
    );
  }

  const overdue = Boolean(isOverdue) && !completedAt;

  return (
    <Space size={6} wrap data-testid="deadline-badge" data-overdue={overdue ? 'true' : 'false'}>
      <ClockCircleOutlined
        style={{ color: overdue ? '#cf1322' : '#8c8c8c', fontSize: size === 'small' ? 12 : 14 }}
      />
      <Typography.Text
        type={overdue ? 'danger' : undefined}
        style={{ fontSize: size === 'small' ? 12 : 14, whiteSpace: 'nowrap' }}
      >
        {withLabel ? 'Дедлайн: ' : ''}
        {formatDeadline(deadline)}
      </Typography.Text>
      {overdue ? (
        <Tag
          color="error"
          icon={<WarningOutlined />}
          style={{ marginInlineEnd: 0, fontSize: size === 'small' ? 11 : 12 }}
        >
          Просрочено
        </Tag>
      ) : null}
    </Space>
  );
}
