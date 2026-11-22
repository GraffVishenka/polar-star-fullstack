/**
 * ErrorBoundary — страховка от падения рендера.
 *
 * Показывает понятный экран с `request_id`, если ошибка пришла от API,
 * и позволяет повторить рендер без перезагрузки страницы.
 */
import { Component, type ErrorInfo, type ReactNode } from 'react';
import { Button, Result, Space, Typography } from 'antd';
import { ApiError } from '@/api/errors';

export interface ErrorBoundaryProps {
  children: ReactNode;
  /** Свой экран ошибки (например, внутри модального окна). */
  fallback?: (error: Error, reset: () => void) => ReactNode;
  /** Колбэк для логирования/метрик. */
  onError?: (error: Error, info: ErrorInfo) => void;
}

interface ErrorBoundaryState {
  error: Error | null;
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // NFR-4: в проде сюда добавляется отправка в систему логов.
    console.error('[ErrorBoundary]', error, info.componentStack);
    this.props.onError?.(error, info);
  }

  reset = (): void => {
    this.setState({ error: null });
  };

  render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;
    if (this.props.fallback) return this.props.fallback(error, this.reset);

    const apiError = error instanceof ApiError ? error : null;
    const status = apiError?.status && apiError.status >= 400 ? apiError.status : 500;

    return (
      <Result
        status={status === 404 ? '404' : status === 403 ? '403' : 'error'}
        title="Что-то пошло не так"
        subTitle={
          <Space direction="vertical" size={4}>
            <Typography.Text>
              {apiError ? apiError.userMessage : 'Страница не смогла отрисоваться.'}
            </Typography.Text>
            {apiError?.requestId ? (
              <Typography.Text type="secondary" copyable>
                Код запроса: {apiError.requestId}
              </Typography.Text>
            ) : null}
          </Space>
        }
        extra={
          <Space wrap>
            <Button type="primary" onClick={this.reset}>
              Повторить
            </Button>
            <Button onClick={() => window.location.reload()}>Перезагрузить страницу</Button>
          </Space>
        }
        data-testid="error-boundary"
      />
    );
  }
}
