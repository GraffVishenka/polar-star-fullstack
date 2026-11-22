/**
 * Мок браузерного push-окружения для Vitest (Ф9).
 *
 * jsdom не умеет service worker, PushManager и Notification, поэтому тесты
 * подменяют их управляемыми заглушками: разрешение, существующая подписка,
 * ошибки register/subscribe и вызов `pushManager.subscribe`.
 *
 * Использование:
 *   const env = installPushEnvironment({ permission: 'default' });
 *   ...
 *   env.cleanup(); // вернуть исходные значения (обычно в afterEach)
 */
import { vi } from 'vitest';
import { PUSH_STORAGE_KEY } from '../pushClient';

/** VAPID-ключ как в MSW-моке `GET /api/push/public-key`. */
export const FAKE_VAPID_PUBLIC_KEY =
  'BEl62iUYgUivxIkv69yViEuiBIa-Ib9-SkvMeAtA3LFgDzkrxZJjSgSnfckjBJuBkr3qBUYIHBQFLXYp5Nksh8U';

export const FAKE_PUSH_ENDPOINT = 'https://push.example.test/device-1';

export interface FakePushSubscription {
  endpoint: string;
  toJSON: () => {
    endpoint: string;
    expirationTime: null;
    keys: { p256dh: string; auth: string };
  };
  getKey: (name: string) => ArrayBuffer | null;
  unsubscribe: ReturnType<typeof vi.fn>;
}

export interface PushEnvironmentOptions {
  /** false — браузер без Push API (нет serviceWorker/PushManager/Notification). */
  supported?: boolean;
  permission?: NotificationPermission;
  requestPermissionResult?: NotificationPermission;
  /** Устройство уже подписано. */
  hasSubscription?: boolean;
  /** Id подписки, сохранённый в localStorage. */
  storedSubscriptionId?: number | null;
  secureContext?: boolean;
  registerError?: Error;
  subscribeError?: Error;
}

export interface PushTestEnvironment {
  registration: {
    scope: string;
    pushManager: {
      getSubscription: ReturnType<typeof vi.fn>;
      subscribe: ReturnType<typeof vi.fn>;
    };
    showNotification: ReturnType<typeof vi.fn>;
    unregister: ReturnType<typeof vi.fn>;
  };
  serviceWorker: {
    register: ReturnType<typeof vi.fn>;
    getRegistration: ReturnType<typeof vi.fn>;
    ready: Promise<unknown>;
    controller: null;
    addEventListener: ReturnType<typeof vi.fn>;
    removeEventListener: ReturnType<typeof vi.fn>;
  };
  subscribeCalls: PushSubscriptionOptionsInit[];
  requestPermission: ReturnType<typeof vi.fn>;
  showNotification: ReturnType<typeof vi.fn>;
  getSubscription: () => FakePushSubscription | null;
  cleanup: () => void;
}

function makeSubscription(endpoint: string): FakePushSubscription {
  return {
    endpoint,
    toJSON: () => ({
      endpoint,
      expirationTime: null,
      keys: { p256dh: 'fake-p256dh', auth: 'fake-auth' },
    }),
    getKey: () => null,
    unsubscribe: vi.fn(async () => true),
  };
}

type Descriptor = PropertyDescriptor | undefined;

function remember(target: object, key: string): Descriptor {
  return Object.getOwnPropertyDescriptor(target, key);
}

function restore(target: object, key: string, descriptor: Descriptor): void {
  if (descriptor) Object.defineProperty(target, key, descriptor);
  else Reflect.deleteProperty(target, key);
}

export function installPushEnvironment(options: PushEnvironmentOptions = {}): PushTestEnvironment {
  const {
    supported = true,
    permission = 'default',
    requestPermissionResult = 'granted',
    hasSubscription = false,
    storedSubscriptionId = null,
    secureContext = true,
    registerError,
    subscribeError,
  } = options;

  const originals: Array<[object, string, Descriptor]> = [
    [window, 'Notification', remember(window, 'Notification')],
    [window, 'PushManager', remember(window, 'PushManager')],
    [window, 'isSecureContext', remember(window, 'isSecureContext')],
    [navigator, 'serviceWorker', remember(navigator, 'serviceWorker')],
  ];

  const subscribeCalls: PushSubscriptionOptionsInit[] = [];
  let current: FakePushSubscription | null = hasSubscription
    ? makeSubscription(FAKE_PUSH_ENDPOINT)
    : null;
  let permissionState: NotificationPermission = permission;

  const requestPermission = vi.fn(async () => {
    permissionState = requestPermissionResult;
    return requestPermissionResult;
  });

  let showNotification: ReturnType<typeof vi.fn> = vi.fn(async () => undefined);

  const registration = {
    scope: '/',
    pushManager: {
      getSubscription: vi.fn(async () => current),
      subscribe: vi.fn(async (subscribeOptions: PushSubscriptionOptionsInit) => {
        subscribeCalls.push(subscribeOptions);
        if (subscribeError) throw subscribeError;
        current = makeSubscription(FAKE_PUSH_ENDPOINT);
        return current;
      }),
    },
    showNotification,
    unregister: vi.fn(async () => true),
  };

  const serviceWorker = {
    register: vi.fn(async () => {
      if (registerError) throw registerError;
      return registration;
    }),
    getRegistration: vi.fn(async () => registration),
    ready: Promise.resolve(registration),
    controller: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  };

  if (storedSubscriptionId !== null) {
    window.localStorage.setItem(PUSH_STORAGE_KEY, String(storedSubscriptionId));
  }

  if (supported) {
    const notificationMock = function NotificationMock() {} as unknown as typeof Notification;
    Object.defineProperty(notificationMock, 'permission', {
      get: () => permissionState,
      configurable: true,
    });
    notificationMock.requestPermission = requestPermission;

    Object.defineProperty(window, 'Notification', {
      value: notificationMock,
      configurable: true,
      writable: true,
    });
    Object.defineProperty(window, 'PushManager', {
      value: function PushManagerMock() {},
      configurable: true,
      writable: true,
    });
    Object.defineProperty(navigator, 'serviceWorker', {
      value: serviceWorker,
      configurable: true,
    });
  } else {
    Reflect.deleteProperty(window, 'Notification');
    Reflect.deleteProperty(window, 'PushManager');
    Reflect.deleteProperty(navigator, 'serviceWorker');
  }

  Object.defineProperty(window, 'isSecureContext', {
    value: secureContext,
    configurable: true,
  });

  return {
    registration,
    serviceWorker,
    subscribeCalls,
    requestPermission,
    showNotification,
    getSubscription: () => current,
    cleanup: () => {
      showNotification = vi.fn(async () => undefined);
      originals.forEach(([target, key, descriptor]) => restore(target, key, descriptor));
    },
  };
}
