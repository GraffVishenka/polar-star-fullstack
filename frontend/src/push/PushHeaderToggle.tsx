/**
 * Компактный переключатель push в шапке `AppLayout` (Ф9).
 *
 * Состояние берёт из `PushContext`, поэтому синхронизирован с карточкой
 * уведомлений в `/profile`. Скрыт, если браузер не умеет push; при запрете
 * разрешения — выключен с подсказкой (инструкция живёт в профиле).
 */
import { BellOutlined } from '@ant-design/icons';
import { Flex, Switch, Tooltip, Typography } from 'antd';
import { usePush } from './PushProvider';

export function PushHeaderToggle() {
  const { status, isBusy, secureContext, enable, disable } = usePush();

  // Нет Push API / ещё определяем состояние — не занимаем место в шапке.
  if (status === 'loading' || status === 'unsupported') return null;

  const checked = status === 'subscribed';
  const denied = status === 'denied';
  const disabled = denied || !secureContext || isBusy;

  const title = denied
    ? 'Уведомления запрещены в настройках браузера — инструкция в профиле'
    : !secureContext
      ? 'Push-уведомления работают только по HTTPS (кроме localhost)'
      : checked
        ? 'Уведомления включены — выключить для этого устройства'
        : 'Включить push-уведомления на этом устройстве';

  return (
    <Tooltip title={title}>
      <Flex align="center" gap={6} data-testid="header-push">
        <BellOutlined style={{ color: checked ? '#1677ff' : '#8c8c8c' }} aria-hidden />
        <Typography.Text type="secondary" style={{ fontSize: 12 }} className="header-push__label">
          Push
        </Typography.Text>
        <Switch
          size="small"
          checked={checked}
          disabled={disabled}
          loading={isBusy}
          aria-label="Push-уведомления"
          data-testid="header-push-toggle"
          onChange={(next) => {
            if (next) void enable();
            else void disable();
          }}
        />
      </Flex>
    </Tooltip>
  );
}
