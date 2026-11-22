/**
 * Дизайн-система: тема Ant Design 5 под внутренний сервисный инструмент.
 * Русская локаль подключается в `Providers` (`antd/locale/ru_RU`).
 */
import type { ThemeConfig } from 'antd';

export const themeConfig: ThemeConfig = {
  token: {
    colorPrimary: '#1677ff',
    colorInfo: '#1677ff',
    colorSuccess: '#52c41a',
    colorWarning: '#fa8c16',
    colorError: '#cf1322',
    borderRadius: 8,
    fontSize: 14,
    colorBgLayout: '#f5f7fa',
    fontFamily:
      '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
    wireframe: false,
  },
  components: {
    Layout: {
      headerBg: '#ffffff',
      headerHeight: 60,
      headerPadding: '0 16px',
      footerBg: '#ffffff',
      footerPadding: '16px 24px',
      bodyBg: '#f5f7fa',
      siderBg: '#ffffff',
    },
    Card: {
      borderRadiusLG: 12,
    },
    Menu: {
      itemBorderRadius: 8,
      horizontalItemSelectedColor: '#1677ff',
    },
    Table: {
      headerBg: '#fafafa',
    },
    Tag: {
      defaultBg: '#f0f2f5',
    },
  },
};
