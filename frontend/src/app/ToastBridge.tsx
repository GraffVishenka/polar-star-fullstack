/**
 * Мост между React-деревом antd и модульными вызовами `toast.*`/уведомлений
 * об ошибках API. Рендерится внутри `<App>` (antd), поэтому имеет доступ к
 * message/notification с текущей темой и локалью.
 */
import { useEffect } from 'react';
import { App } from 'antd';
import { describeApiError, setErrorNotifier } from '@/api/errors';
import { registerToastApi } from './toast';

export function ToastBridge() {
  const { message, notification } = App.useApp();

  useEffect(() => {
    registerToastApi({
      success: (title, description) => {
        void message.success(description ? `${title}: ${description}` : title);
      },
      info: (title, description) => {
        void message.info(description ? `${title}: ${description}` : title);
      },
      warning: (title, description) => {
        void message.warning(description ? `${title}: ${description}` : title);
      },
      error: (title, description) => {
        if (description) {
          notification.error({ message: title, description, duration: 8, placement: 'topRight' });
        } else {
          void message.error(title);
        }
      },
    });
    return () => registerToastApi(null);
  }, [message, notification]);

  // 5xx/сеть — автоматический тост с `request_id` (NFR-4).
  useEffect(() => {
    setErrorNotifier((error) => {
      const { title, description } = describeApiError(error);
      notification.error({
        message: title,
        description,
        duration: 8,
        placement: 'topRight',
        key: `api-error-${error.status}-${error.requestId ?? 'no-request-id'}`,
      });
    });
    return () => setErrorNotifier(null);
  }, [notification]);

  return null;
}
