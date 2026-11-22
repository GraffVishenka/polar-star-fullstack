/**
 * `/profile` — профиль сотрудника (Ф9, STATS-4).
 *
 * Разделы: учётная запись (ФИО, логин, роль, здание), push-уведомления
 * (подписка устройства, NOTIF-5) и смена своего пароля (AUTH-5).
 * Личная статистика появится в блоке Ф10.
 */
import { Col, Flex, Row } from 'antd';
import { useAuth } from '@/auth/AuthContext';
import { PageHeader } from '@/components/PageHeader';
import { ProfileAccountCard } from './components/ProfileAccountCard';
import { ProfilePasswordCard } from './components/ProfilePasswordCard';
import { PushSettingsCard } from './components/PushSettingsCard';

export function ProfilePage() {
  const { user, isAdmin } = useAuth();

  return (
    <Flex vertical gap={16} data-testid="profile-page">
      <PageHeader
        title="Профиль"
        description={
          isAdmin
            ? 'Администратор не привязан к зданию и видит данные всех зданий (AUTH-4).'
            : 'Личные данные, уведомления и пароль. Уведомления приходят только по вашему зданию.'
        }
      />

      <Row gutter={[16, 16]}>
        <Col xs={24} xl={13}>
          <Flex vertical gap={16}>
            <ProfileAccountCard user={user} />
            <ProfilePasswordCard />
          </Flex>
        </Col>
        <Col xs={24} xl={11}>
          <PushSettingsCard />
        </Col>
      </Row>
    </Flex>
  );
}
