/**
 * Карточка учётной записи в `/profile` (Ф9): ФИО, логин, роль, здание.
 *
 * AUTH-9: у USER без здания показываем предупреждение — заявки и уведомления
 * по зданию работать не будут, пока администратор не назначит здание.
 */
import { Alert, Card, Descriptions, Tag, Typography } from 'antd';
import { BankOutlined, UserOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import type { AuthUser } from '@/api/types';

export interface ProfileAccountCardProps {
  user: AuthUser | null;
}

function formatCreatedAt(value?: string): string {
  if (!value) return '—';
  const parsed = dayjs(value);
  return parsed.isValid() ? parsed.format('DD.MM.YYYY') : value;
}

export function ProfileAccountCard({ user }: ProfileAccountCardProps) {
  const isAdmin = user?.role === 'ADMIN';
  const buildingMissing = !isAdmin && Boolean(user) && !user?.building_id;
  const buildingName = user?.building_name ?? (isAdmin ? 'Все здания' : 'Не назначено');

  return (
    <Card
      size="small"
      title={
        <span>
          <UserOutlined /> Учётная запись
        </span>
      }
      data-testid="profile-account"
    >
      <Descriptions
        size="small"
        column={1}
        items={[
          { key: 'full_name', label: 'ФИО', children: user?.full_name ?? '—' },
          {
            key: 'login',
            label: 'Логин',
            children: <Typography.Text code>{user?.login ?? '—'}</Typography.Text>,
          },
          {
            key: 'role',
            label: 'Роль',
            children: (
              <Tag color={isAdmin ? 'gold' : 'blue'} data-testid="profile-role">
                {isAdmin ? 'Администратор' : 'Сотрудник'}
              </Tag>
            ),
          },
          {
            key: 'building',
            label: 'Здание',
            children: (
              <Typography.Text data-testid="profile-building">
                <BankOutlined /> {buildingName}
              </Typography.Text>
            ),
          },
          { key: 'created_at', label: 'Создан', children: formatCreatedAt(user?.created_at) },
        ]}
      />

      {buildingMissing ? (
        <Alert
          type="warning"
          showIcon
          style={{ marginTop: 12 }}
          message="Здание не назначено, обратитесь к администратору"
          description="Пока здание не назначено, заявки и уведомления по зданию недоступны (AUTH-9)."
          data-testid="profile-building-missing"
        />
      ) : null}
    </Card>
  );
}
