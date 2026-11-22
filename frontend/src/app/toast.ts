/**
 * Тост-уведомления приложения вне React-дерева.
 *
 * API-клиент и общие компоненты не должны зависеть от antd-контекста, поэтому
 * провайдеры один раз регистрируют реализацию (`ToastBridge`), а код вызывает
 * `toast.error(...)` из любого места. До регистрации вызовы — no-op.
 */
export interface ToastApi {
  success: (title: string, description?: string) => void;
  error: (title: string, description?: string) => void;
  warning: (title: string, description?: string) => void;
  info: (title: string, description?: string) => void;
}

let registered: ToastApi | null = null;

export function registerToastApi(api: ToastApi | null): void {
  registered = api;
}

const call =
  (level: keyof ToastApi) =>
  (title: string, description?: string): void => {
    // Прокидываем ровно те аргументы, с которыми вызвали: так тесты и
    // реализации видят одинаковый вызов (без лишнего undefined).
    if (description === undefined) registered?.[level](title);
    else registered?.[level](title, description);
  };

export const toast: ToastApi = {
  success: call('success'),
  error: call('error'),
  warning: call('warning'),
  info: call('info'),
};
