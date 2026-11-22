/**
 * Push-переключатель в шапке (Ф9, NOTIF-5): компактное состояние, синхронизация
 * с карточкой профиля и скрытие при отсутствии поддержки.
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { PushSettingsCard } from '@/pages/profile/components/PushSettingsCard';
import { PushProvider } from '@/push/PushProvider';
import { installPushEnvironment, type PushTestEnvironment } from '@/push/testing/pushEnv';
import { USER_AUTH, authenticateAs, makeAuthValue, renderWithProviders } from '@/test/utils';
import { PushHeaderToggle } from './PushHeaderToggle';

let pushEnv: PushTestEnvironment | null = null;

function renderHeaderAndProfile(push: Parameters<typeof installPushEnvironment>[0] = {}) {
  authenticateAs(USER_AUTH.id);
  pushEnv = installPushEnvironment(push);
  return renderWithProviders(
    <PushProvider>
      <PushHeaderToggle />
      <PushSettingsCard />
    </PushProvider>,
    { route: '/profile', auth: makeAuthValue({ user: USER_AUTH }) },
  );
}

afterEach(() => {
  pushEnv?.cleanup();
  pushEnv = null;
});

describe('PushHeaderToggle', () => {
  beforeEach(() => authenticateAs(USER_AUTH.id));

  it('скрыт, если браузер не поддерживает push', async () => {
    renderHeaderAndProfile({ supported: false });

    // Дожидаемся, пока провайдер определит состояние, — шапка остаётся пустой.
    expect(await screen.findByTestId('push-status')).toHaveTextContent(
      'Браузер не поддерживает service worker',
    );
    expect(screen.queryByTestId('header-push')).not.toBeInTheDocument();
  });

  it('выключен при запрещённом разрешении', async () => {
    renderHeaderAndProfile({ permission: 'denied' });

    const toggle = await screen.findByTestId('header-push-toggle');
    expect(toggle).toHaveAttribute('aria-checked', 'false');
    expect(toggle).toBeDisabled();
  });

  it('отражает подписку и синхронизирован с профилем: отключение в шапке меняет карточку', async () => {
    renderHeaderAndProfile({
      permission: 'granted',
      hasSubscription: true,
      storedSubscriptionId: 1,
    });

    // И шапка, и профиль видят одну и ту же подписку.
    expect(await screen.findByTestId('push-status')).toHaveTextContent('Подписка активна');
    const toggle = screen.getByTestId('header-push-toggle');
    expect(toggle).toHaveAttribute('aria-checked', 'true');

    await userEvent.click(toggle);

    await waitFor(() =>
      expect(screen.getByTestId('push-status')).toHaveTextContent('Разрешение выдано'),
    );
    expect(screen.getByTestId('header-push-toggle')).toHaveAttribute('aria-checked', 'false');
  });
});
