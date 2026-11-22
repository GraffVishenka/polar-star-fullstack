/** Заголовок страницы: заголовок, описание, хлебные крошки и действия справа. */
import type { ReactNode } from 'react';
import { Breadcrumb, Flex, Typography } from 'antd';

export interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  /** Действия справа (кнопки создания/фильтров/выгрузки). */
  extra?: ReactNode;
  /** Хлебные крошки (для БЗ и админки). */
  breadcrumbs?: Array<{ title: ReactNode; href?: string }>;
  /** Мобильная раскладка: действия переносятся под заголовок. */
  stackOnMobile?: boolean;
}

export function PageHeader({
  title,
  description,
  extra,
  breadcrumbs,
  stackOnMobile = true,
}: PageHeaderProps) {
  return (
    <Flex vertical gap={8} style={{ marginBottom: 16 }} data-testid="page-header">
      {breadcrumbs?.length ? (
        <Breadcrumb items={breadcrumbs.map((item) => ({ title: item.title, href: item.href }))} />
      ) : null}
      <Flex
        align={stackOnMobile ? 'flex-start' : 'center'}
        justify="space-between"
        gap={12}
        wrap
        vertical={false}
        style={{ flexDirection: 'row' }}
      >
        <div style={{ minWidth: 0 }}>
          <Typography.Title level={3} style={{ margin: 0, fontSize: 'clamp(20px, 4vw, 26px)' }}>
            {title}
          </Typography.Title>
          {description ? (
            <Typography.Text type="secondary" style={{ fontSize: 13 }}>
              {description}
            </Typography.Text>
          ) : null}
        </div>
        {extra ? (
          <Flex gap={8} wrap>
            {extra}
          </Flex>
        ) : null}
      </Flex>
    </Flex>
  );
}
