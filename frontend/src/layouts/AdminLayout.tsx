/**
 * AdminLayout — каркас админ-панели (доступ только ADMIN, см. RequireRole в routes).
 *
 * Боковое меню на десктопе, сворачивается до иконок на планшете; на телефоне
 * разделы доступны через тот же вертикальный список в свёрнутом Sider.
 */
import {
  AppstoreOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  LineChartOutlined,
  RollbackOutlined,
  SettingOutlined,
  TeamOutlined,
} from '@ant-design/icons';
import { Button, Flex, Layout, Menu, Typography } from 'antd';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useAuth } from '@/auth/AuthContext';

const { Sider, Content } = Layout;

const adminMenu = [
  {
    key: '/admin/users',
    icon: <TeamOutlined />,
    label: <Link to="/admin/users">Пользователи</Link>,
  },
  {
    key: '/admin/dictionaries',
    icon: <DatabaseOutlined />,
    label: <Link to="/admin/dictionaries">Справочники</Link>,
  },
  {
    key: '/admin/settings',
    icon: <SettingOutlined />,
    label: <Link to="/admin/settings">Настройки</Link>,
  },
  {
    key: '/admin/buildings',
    icon: <AppstoreOutlined />,
    label: <Link to="/admin/buildings">Здания и склады</Link>,
  },
  {
    key: '/admin/stats',
    icon: <LineChartOutlined />,
    label: <Link to="/admin/stats">Статистика</Link>,
  },
  { key: '/admin/trash', icon: <DeleteOutlined />, label: <Link to="/admin/trash">Корзина</Link> },
];

export function AdminLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user } = useAuth();

  const selectedKey =
    adminMenu.find((item) => location.pathname.startsWith(item.key))?.key ?? '/admin/users';

  return (
    <Layout className="admin-layout">
      <Sider className="admin-sider" breakpoint="lg" collapsedWidth={64} width={232} theme="light">
        <Flex vertical gap={12} className="admin-sider__inner">
          <div className="admin-sider__brand">
            <Typography.Text strong>Админ-панель</Typography.Text>
            <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
              {user?.full_name ?? user?.login}
            </Typography.Text>
          </div>
          <Menu
            mode="inline"
            selectedKeys={[selectedKey]}
            items={adminMenu}
            className="admin-menu"
            style={{ borderInlineEnd: 'none' }}
          />
          <Button
            type="link"
            icon={<RollbackOutlined />}
            onClick={() => navigate('/tickets')}
            className="admin-sider__back"
          >
            К заявкам
          </Button>
        </Flex>
      </Sider>
      <Content className="admin-content">
        <div className="admin-content__inner">
          <Outlet />
        </div>
      </Content>
    </Layout>
  );
}
