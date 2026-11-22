/** EmptyState — единое пустое состояние списков, таблиц и колонок (TICKET-5, Ф10). */
import type { ReactNode } from 'react';
import { Button, Empty, Flex, Typography } from 'antd';

export interface EmptyStateProps {
  /** Короткий заголовок, например «Заявок пока нет». */
  title: string;
  /** Пояснение, что сделать дальше. */
  description?: ReactNode;
  /** Кнопка действия («Создать заявку», «Сбросить фильтры»). */
  action?: {
    label: string;
    onClick: () => void;
    icon?: ReactNode;
    type?: 'primary' | 'default' | 'link';
  };
  /** Иллюстрация antd Empty. */
  image?: 'default' | 'simple';
  /** Компактный вариант для колонок доски. */
  compact?: boolean;
}

export function EmptyState({
  title,
  description,
  action,
  image = 'simple',
  compact = false,
}: EmptyStateProps) {
  return (
    <Flex
      vertical
      align="center"
      justify="center"
      gap={compact ? 4 : 8}
      style={{ padding: compact ? '16px 8px' : '32px 16px', textAlign: 'center' }}
      data-testid="empty-state"
    >
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        imageStyle={compact ? { height: 40, marginBottom: 4 } : undefined}
        description={
          <Flex vertical gap={4} align="center">
            <Typography.Text strong={!compact} style={{ fontSize: compact ? 13 : 15 }}>
              {title}
            </Typography.Text>
            {description ? (
              <Typography.Text type="secondary" style={{ fontSize: compact ? 12 : 13 }}>
                {description}
              </Typography.Text>
            ) : null}
          </Flex>
        }
      >
        {action ? (
          <Button
            type={action.type ?? 'primary'}
            icon={action.icon}
            onClick={action.onClick}
            data-testid="empty-state-action"
          >
            {action.label}
          </Button>
        ) : null}
      </Empty>
      {/* image оставлен для совместимости API: antd Empty всегда рисует иллюстрацию */}
      <span hidden data-image={image} />
    </Flex>
  );
}
