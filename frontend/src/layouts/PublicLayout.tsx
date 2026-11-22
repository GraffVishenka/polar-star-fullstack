/**
 * PublicLayout — публичный периметр (PUB-1..PUB-4).
 *
 * Шапка: название приложения, ссылки «Выбор здания» и «База знаний»,
 * переключатель зданий (только на дашборде `/b/:id`) и вход для сотрудников.
 * Футер нейтральный: не раскрывает внутренние сущности (NFR-2).
 */
import { useCallback, useMemo } from 'react';
import { BankOutlined, LoginOutlined, ReadOutlined } from '@ant-design/icons';
import { Button, Flex, Layout, Menu, Space, Typography } from 'antd';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { APP_DESCRIPTION } from '@/app/config';
import { useAuth } from '@/auth/AuthContext';
import { BuildingSelect } from '@/components/BuildingSelect';
import { useAnonBuilding } from '@/hooks/useAnonBuilding';
import { usePublicSettings } from '@/hooks/usePublicSettings';

const { Header, Content, Footer } = Layout;

export function PublicLayout() {
  const location = useLocation();
  const navigate = useNavigate();
  const { isAuthenticated } = useAuth();
  const { buildings, isLoading, selectedBuilding, selectBuilding } = useAnonBuilding();
  // PUB-1: имя приложения настраивается в админке; при недоступности API — фолбэк.
  const { appName } = usePublicSettings();

  const currentBuildingId = useMemo(() => {
    const match = /^\/b\/(\d+)/.exec(location.pathname);
    if (match?.[1]) return Number(match[1]);
    return selectedBuilding?.id ?? null;
  }, [location.pathname, selectedBuilding?.id]);

  const showSwitcher = location.pathname.startsWith('/b/');

  const handleBuildingChange = useCallback(
    (buildingId: number | null) => {
      if (buildingId === null) {
        navigate('/b/all');
        return;
      }
      selectBuilding(buildingId);
      navigate(`/b/${buildingId}`);
    },
    [navigate, selectBuilding],
  );

  return (
    <Layout className="public-shell">
      <Header className="public-header">
        <Flex align="center" justify="space-between" gap={12} wrap className="public-header__inner">
          <Link to="/" className="brand" aria-label={`${appName} — на главную`}>
            <BankOutlined className="brand__icon" />
            <Space direction="vertical" size={0}>
              <Typography.Text strong className="brand__title">
                {appName}
              </Typography.Text>
              <Typography.Text type="secondary" className="brand__subtitle">
                {APP_DESCRIPTION}
              </Typography.Text>
            </Space>
          </Link>

          <Flex align="center" gap={12} wrap className="public-header__actions">
            {showSwitcher ? (
              <BuildingSelect
                buildings={buildings.map(({ id, name }) => ({ id, name }))}
                value={currentBuildingId}
                onChange={handleBuildingChange}
                loading={isLoading}
                allowAll
                size="middle"
                className="public-header__switcher"
              />
            ) : null}
            <Menu
              mode="horizontal"
              selectable={false}
              className="public-header__menu"
              items={[
                {
                  key: 'kb',
                  icon: <ReadOutlined />,
                  label: <Link to="/public/kb">База знаний</Link>,
                },
              ]}
            />
            {isAuthenticated ? (
              <Button type="primary" href="/tickets">
                В личный кабинет
              </Button>
            ) : (
              <Button icon={<LoginOutlined />} href="/login">
                Войти
              </Button>
            )}
          </Flex>
        </Flex>
      </Header>

      <Content className="public-content">
        <div className="public-content__inner">
          <Outlet />
        </div>
      </Content>

      <Footer className="public-footer">
        <Flex vertical align="center" gap={4}>
          <Typography.Text type="secondary" style={{ fontSize: 13 }}>
            {appName} · {APP_DESCRIPTION}
          </Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            © {new Date().getFullYear()} Внутренний сервис. Сотрудникам — вход по логину.
          </Typography.Text>
        </Flex>
      </Footer>
    </Layout>
  );
}
