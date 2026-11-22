/**
 * Заглушка раздела, который реализуют другие агенты (Ф2–Ф10).
 *
 * Файл лежит в `src/pages/placeholders`, чтобы не занимать каталоги
 * `pages/kb`, `pages/equipment`, `pages/admin/*`, `pages/profile` — их
 * владельцы заменят маршрут на реальную страницу без конфликтов слияния.
 */
import { DatabaseOutlined } from '@ant-design/icons';
import { Card, Flex, Result, Tag, Typography } from 'antd';
import { Link } from 'react-router-dom';
import { PageHeader } from '@/components/PageHeader';

export interface PlaceholderPageProps {
  title: string;
  /** Блок PLAN.md, в котором появится экран. */
  block: string;
  /** Агент-владелец раздела. */
  owner: string;
  description?: string;
}

export function PlaceholderPage({ title, block, owner, description }: PlaceholderPageProps) {
  return (
    <Flex vertical gap={16} data-testid="placeholder-page">
      <PageHeader
        title={title}
        description={description ?? 'Раздел в разработке'}
        extra={
          <Tag color="processing">
            {block} · {owner}
          </Tag>
        }
      />
      <Card>
        <Result
          icon={<DatabaseOutlined />}
          title={`${title}: экран готовит ${owner}`}
          subTitle={
            <Flex vertical gap={4} align="center">
              <Typography.Text type="secondary">
                API, MSW-моки и общие компоненты уже доступны — экран подключается к реальному
                контракту без бэкенда.
              </Typography.Text>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Блок {block} · см. PLAN.md §6.2
              </Typography.Text>
            </Flex>
          }
          extra={<Link to="/tickets">Перейти к заявкам</Link>}
        />
      </Card>
    </Flex>
  );
}
