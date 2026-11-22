/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Базовый URL API. По умолчанию `/api` (проксируется nginx/vite). */
  readonly VITE_API_BASE_URL?: string;
  /** `false` отключает MSW в dev-режиме. */
  readonly VITE_ENABLE_MSW?: string;
  /** Отображаемое имя приложения, пока настройки недоступны (Ф6). */
  readonly VITE_APP_NAME?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
