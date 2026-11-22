/**
 * Настройки push-уведомлений в `/profile` (Ф9, NOTIF-1..NOTIF-5).
 *
 * Состояния переключателя: не поддерживается, разрешение не запрошено,
 * разрешено без подписки, подписано, запрещено (с инструкцией), ошибка.
 * Разрешение запрашивается только по кнопке (NOTIF-5), а не при загрузке.
 *
 * Тестовая отправка требует серверного эндпоинта (Б8) — пока его нет,
 * кнопка выключена с пояснением, чтобы не обещать несуществующую функцию.
 */
import { BellOutlined, SendOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Flex, List, Space, Tag, Tooltip, Typography } from 'antd';
import { usePush } from '@/push/PushProvider';

const DENIED_STEPS = [
  'Chrome / Edge: значок замка в адресной строке → «Настройки сайта» → «Уведомления» → «Разрешить».',
  'Firefox: значок замка → «Дополнительные настройки» → «Разрешения» → «Отправлять уведомления» → «Разрешить».',
  'Safari: «Safari» → «Настройки для сайта» → «Уведомления» → «Разрешить» (macOS 13+ / iOS 16.4+).',
  'После изменения настроек перезагрузите страницу и включите уведомления снова.',
];

function statusText(
  status: ReturnType<typeof usePush>['status'],
  supportReason: string | null,
): string {
  switch (status) {
    case 'loading':
      return 'Проверяем поддержку уведомлений…';
    case 'unsupported':
      return `Push-уведомления недоступны: ${supportReason ?? 'браузер не поддерживает Push API'}.`;
    case 'denied':
      return 'Уведомления запрещены для этого сайта в настройках браузера.';
    case 'default':
      return 'Разрешение на уведомления не запрошено.';
    case 'granted':
      return 'Разрешение выдано, но устройство ещё не подписано.';
    case 'subscribed':
      return 'Подписка активна: уведомления приходят на это устройство.';
    case 'error':
      return 'Не удалось включить уведомления.';
    default:
      return '';
  }
}

export function PushSettingsCard() {
  const {
    status,
    supportReason,
    error,
    isBusy,
    secureContext,
    subscriptionId,
    canEnable,
    enable,
    disable,
  } = usePush();

  const denied = status === 'denied';
  const subscribed = status === 'subscribed';

  return (
    <Card
      size="small"
      title={
        <span>
          <BellOutlined /> Уведомления
        </span>
      }
      data-testid="profile-push"
    >
      <Typography.Paragraph style={{ marginBottom: 8 }}>
        <Tag color={subscribed ? 'green' : 'default'} data-testid="push-state-tag">
          {subscribed ? 'подписано' : status === 'denied' ? 'запрещено' : 'не подписано'}
        </Tag>
        <Typography.Text data-testid="push-status" aria-live="polite">
          {statusText(status, supportReason)}
        </Typography.Text>
      </Typography.Paragraph>

      {subscribed && subscriptionId !== null ? (
        <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>
          Подписка #{subscriptionId} на этом устройстве
        </Typography.Text>
      ) : null}

      {error ? (
        <Alert
          type="error"
          showIcon
          message={error}
          style={{ marginBottom: 12 }}
          data-testid="push-error"
        />
      ) : null}

      {!secureContext ? (
        <Alert
          type="warning"
          showIcon
          message="Уведомления работают только по HTTPS"
          description="Кроме localhost: браузеры разрешают Web Push только на защищённом соединении (NOTIF-1)."
          style={{ marginBottom: 12 }}
          data-testid="push-insecure-hint"
        />
      ) : null}

      {denied ? (
        <Alert
          type="info"
          showIcon
          message="Как включить уведомления в браузере"
          description={
            <List
              size="small"
              data-testid="push-denied-instructions"
              dataSource={DENIED_STEPS}
              renderItem={(item) => <List.Item>{item}</List.Item>}
            />
          }
          style={{ marginBottom: 12 }}
        />
      ) : null}

      <Space wrap>
        {subscribed ? (
          <Button danger loading={isBusy} onClick={() => void disable()} data-testid="push-disable">
            Отключить уведомления
          </Button>
        ) : (
          <Button
            type="primary"
            loading={isBusy}
            disabled={!canEnable || !secureContext || denied || status === 'unsupported'}
            onClick={() => void enable()}
            data-testid="push-enable"
          >
            Включить уведомления
          </Button>
        )}

        <Tooltip title="Серверная тестовая отправка появится после подключения API уведомлений (Б8, NOTIF-3). Сейчас уведомления приходят по реальным событиям.">
          <Button icon={<SendOutlined />} disabled data-testid="push-test-notification">
            Отправить тестовое уведомление
          </Button>
        </Tooltip>
      </Space>

      <Flex vertical gap={4} style={{ marginTop: 12 }}>
        <Typography.Text type="secondary" style={{ fontSize: 12 }} data-testid="push-test-hint">
          Тестовая отправка станет доступна, когда бэкенд добавит эндпоинт (Б8). Проверить
          уведомления можно реальной заявкой или назначением исполнителя.
        </Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Уведомления получают только сотрудники и администраторы; у каждого устройства своя
          подписка, отключение действует на текущем устройстве.
        </Typography.Text>
      </Flex>
    </Card>
  );
}
