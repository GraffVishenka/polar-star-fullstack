/**
 * AppLayout — каркас рабочего пространства сотрудника.
 *
 * Шапка: разделы (Заявки, БЗ, Оборудование, Статистика), здание пользователя,
 * меню профиля и выход. Отдельный баннер для AUTH-9: у USER не назначено здание,
 * заявки недоступны (сервер вернёт 409 BUILDING_NOT_ASSIGNED).
 */
import { useCallback, useState } from 'react';
import {
  BankOutlined,
  BarChartOutlined,
  LaptopOutlined,
  LogoutOutlined,
  ReadOutlined,
  ScheduleOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Alert, Avatar, Button, Dropdown, Flex, Layout, Menu, Space, Typography } from 'antd';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { APP_NAME } from '@/app/config';
import { useAuth } from '@/auth/AuthContext';
import { BuildingBadge } from '@/components/BuildingBadge';

const { Header, Content, Footer } = Layout;

export function AppLayout() {
  const { user, isAdmin, logout } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const [loggingOut, setLoggingOut] = useState(false);

  const menuItems = [
    { key: '/tickets', icon: <ScheduleOutlined />, label: <Link to="/tickets">Заявки</Link> },
    { key: '/kb', icon: <ReadOutlined />, label: <Link to="/kb">База знаний</Link> },
    {
      key: '/equipment',
      icon: <LaptopOutlined />,
      label: <Link to="/equipment">Оборудование</Link>,
    },
    { key: '/stats', icon: <BarChartOutlined />, label: <Link to="/stats">Статистика</Link> },
    ...(isAdmin
      ? [
          {
            key: '/admin',
            icon: <SettingOutlined />,
            label: <Link to="/admin/users">Админ-панель</Link>,
          },
        ]
      : []),
  ];

  const selectedKey =
    menuItems.find((item) => location.pathname.startsWith(item.key))?.key ?? '/tickets';

  const handleLogout = useCallback(async () => {
    setLoggingOut(true);
    try {
      await logout();
      navigate('/', { replace: true });
    } finally {
      setLoggingOut(false);
    }
  }, [logout, navigate]);

  const userMenuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: <Link to="/profile">Профиль и статистика</Link>,
    },
    { type: 'divider' as const },
    {
      key: 'logout',
      icon: <LogoutOutlined />,
      danger: true,
      label: 'Выйти',
      onClick: () => void handleLogout(),
    },
  ];

  const buildingMissing = user?.role === 'USER' && !user.building_id;

  return (
    <Layout className="app-shell">
      <Header className="app-header">
        <Flex align="center" gap={16} className="app-header__inner">
          <Link to="/tickets" className="brand brand--compact">
            <BankOutlined className="brand__icon" />
            <Typography.Text strong className="brand__title">
              {APP_NAME}
            </Typography.Text>
          </Link>

          <Menu
            mode="horizontal"
            selectedKeys={[selectedKey]}
            items={menuItems}
            className="app-header__menu"
          />

          <Space size={12} className="app-header__right">
            {user ? (
              <BuildingBadge
                buildingId={user.building_id}
                buildingName={user.building_name}
                allBuildings={user.role === 'ADMIN'}
                size="small"
              />
            ) : null}
            <Dropdown menu={{ items: userMenuItems }} trigger={['click']} placement="bottomRight">
              <Button type="text" loading={loggingOut} data-testid="user-menu">
                <Space size={8}>
                  <Avatar size={28} icon={<UserOutlined />} />
                  <Typography.Text className="app-header__user-name">
                    {user?.full_name ?? user?.login}
                  </Typography.Text>
                </Space>
              </Button>
            </Dropdown>
          </Space>
        </Flex>
      </Header>

      {buildingMissing ? (
        <Alert
          type="warning"
          banner
          showIcon
          message="Здание не назначено, обратитесь к администратору"
          description="Список заявок и создание заявок недоступны, пока сотруднику не назначено здание."
          data-testid="building-missing-banner"
        />
      ) : null}

      <Content className="app-content">
        <div className="app-content__inner">
          <Outlet />
        </div>
      </Content>

      <Footer className="app-footer">
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          © {new Date().getFullYear()} {APP_NAME}. Дедлайны и просрочка рассчитываются на сервере.
        </Typography.Text>
      </Footer>
    </Layout>
  );
}
