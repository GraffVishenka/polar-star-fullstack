/**
 * Push-контекст (Ф9, NOTIF-5).
 *
 * Один источник состояния для шапки (`PushHeaderToggle`) и профиля
 * (`PushSettingsCard`): статус разрешения, наличие подписки, ошибки включения
 * и отключения. Провайдер монтируется в `AppLayout` — push доступен только
 * авторизованным USER/ADMIN (NOTIF-1).
 */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { fetchPushPublicKey, subscribePush, unsubscribePush } from '@/api/admin';
import { ApiError, isApiError } from '@/api/errors';
import {
  clearStoredSubscriptionId,
  detectPushSupport,
  getCurrentPushSubscription,
  isPushSecureContext,
  readStoredSubscriptionId,
  readSubscriptionPayload,
  registerPushServiceWorker,
  storeSubscriptionId,
  urlBase64ToUint8Array,
} from './pushClient';

export type PushStatus =
  'loading' | 'unsupported' | 'denied' | 'default' | 'granted' | 'subscribed' | 'error';

export interface PushContextValue {
  status: PushStatus;
  /** Причина недоступности (нет Push API, insecure context и т.п.). */
  supportReason: string | null;
  /** Текст последней ошибки включения/отключения. */
  error: string | null;
  isBusy: boolean;
  /** HTTPS или localhost — иначе push в браузере не заработает (NOTIF-1). */
  secureContext: boolean;
  /** Id активной подписки на этом устройстве. */
  subscriptionId: number | null;
  /** Можно ли предложить включение прямо сейчас. */
  canEnable: boolean;
  enable: () => Promise<void>;
  disable: () => Promise<void>;
  refresh: () => Promise<void>;
}

export const PushContext = createContext<PushContextValue | null>(null);

/** Понятный текст ошибки включения (без технических кодов в интерфейсе). */
export function describePushError(cause: unknown): string {
  if (isApiError(cause)) {
    return cause.code === 'PASSWORD_CHANGE_REQUIRED'
      ? 'Сначала смените временный пароль'
      : cause.userMessage;
  }
  if (cause instanceof DOMException) {
    if (cause.name === 'NotAllowedError' || cause.name === 'PermissionDeniedError') {
      return 'Браузер запретил уведомления. Разрешите их в настройках сайта и повторите.';
    }
    if (cause.name === 'AbortError') {
      return 'Не удалось подписаться на уведомления. Попробуйте ещё раз.';
    }
    return cause.message || 'Ошибка браузера при подписке на уведомления';
  }
  if (cause instanceof ApiError) return cause.userMessage;
  if (cause instanceof Error) return cause.message;
  return 'Не удалось включить уведомления';
}

export interface PushProviderProps {
  children: ReactNode;
}

export function PushProvider({ children }: PushProviderProps) {
  const [status, setStatus] = useState<PushStatus>('loading');
  const [supportReason, setSupportReason] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);
  const [subscriptionId, setSubscriptionId] = useState<number | null>(() =>
    readStoredSubscriptionId(),
  );
  const secureContext = isPushSecureContext();

  /**
   * Определяет текущее состояние без запроса разрешения:
   * подписка → denied → granted → default. Разрешение запрашивается только
   * по явному действию пользователя (NOTIF-5).
   */
  const refresh = useCallback(async () => {
    const support = detectPushSupport();
    if (!support.supported) {
      setSupportReason(support.reason);
      setStatus('unsupported');
      return;
    }
    setSupportReason(null);

    const permission = typeof Notification === 'undefined' ? 'default' : Notification.permission;
    if (permission === 'denied') {
      setStatus('denied');
      return;
    }

    const subscription = await getCurrentPushSubscription();
    if (subscription) {
      setSubscriptionId(readStoredSubscriptionId());
      setStatus('subscribed');
      return;
    }
    setSubscriptionId(null);
    setStatus(permission === 'granted' ? 'granted' : 'default');
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const enable = useCallback(async () => {
    setError(null);
    setBusy(true);
    try {
      if (!isPushSecureContext()) {
        throw new Error('Уведомления работают только по HTTPS (исключение — localhost)');
      }

      const permission =
        Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission();
      if (permission !== 'granted') {
        setStatus(permission === 'denied' ? 'denied' : 'default');
        return;
      }

      const registration = await registerPushServiceWorker();
      const { public_key: publicKey } = await fetchPushPublicKey();
      const existing = await registration.pushManager.getSubscription();
      const subscription =
        existing ??
        (await registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: urlBase64ToUint8Array(publicKey),
        }));

      const saved = await subscribePush(readSubscriptionPayload(subscription));
      storeSubscriptionId(saved.id);
      setSubscriptionId(saved.id);
      setStatus('subscribed');
    } catch (cause) {
      setError(describePushError(cause));
      setStatus('error');
    } finally {
      setBusy(false);
    }
  }, []);

  const disable = useCallback(async () => {
    setError(null);
    setBusy(true);
    try {
      const subscription = await getCurrentPushSubscription();
      const storedId = readStoredSubscriptionId();
      if (storedId !== null) {
        try {
          await unsubscribePush(storedId);
        } catch (cause) {
          // 404 — подписку уже удалили на сервере: локально всё равно отписываемся.
          if (!(isApiError(cause) && cause.status === 404)) throw cause;
        }
      }
      await subscription?.unsubscribe();
      clearStoredSubscriptionId();
      setSubscriptionId(null);
      setStatus('granted');
    } catch (cause) {
      setError(describePushError(cause));
      setStatus('error');
    } finally {
      setBusy(false);
    }
  }, []);

  const value = useMemo<PushContextValue>(
    () => ({
      status,
      supportReason,
      error,
      isBusy,
      secureContext,
      subscriptionId,
      canEnable: status === 'default' || status === 'granted' || status === 'error',
      enable,
      disable,
      refresh,
    }),
    [enable, disable, error, isBusy, refresh, secureContext, status, subscriptionId, supportReason],
  );

  return <PushContext.Provider value={value}>{children}</PushContext.Provider>;
}

export function usePush(): PushContextValue {
  const context = useContext(PushContext);
  if (!context) throw new Error('usePush должен использоваться внутри <PushProvider>');
  return context;
}
