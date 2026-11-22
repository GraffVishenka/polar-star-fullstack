/** Индикатор загрузки на весь экран: bootstrap сессии, ленивые страницы. */
import { Flex, Spin, Typography } from 'antd';

export interface FullPageLoaderProps {
  /** Текст под спиннером. */
  text?: string;
  /** Не занимать всю высоту (например, внутри модального окна). */
  inline?: boolean;
}

export function FullPageLoader({ text = 'Загрузка…', inline = false }: FullPageLoaderProps) {
  return (
    <Flex
      align="center"
      justify="center"
      vertical
      gap={16}
      style={{ minHeight: inline ? 160 : '60vh', padding: 24 }}
      data-testid="full-page-loader"
      role="status"
      aria-live="polite"
    >
      <Spin size="large" />
      <Typography.Text type="secondary">{text}</Typography.Text>
    </Flex>
  );
}
