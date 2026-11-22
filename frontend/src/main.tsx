/**
 * Точка входа SPA.
 *
 * В dev-режиме по умолчанию поднимается MSW (моки по контракту §3.13), чтобы
 * работать без бэкенда: `VITE_ENABLE_MSW=false` отключает моки и включает
 * proxy на `http://localhost:8080` из vite.config.ts.
 */
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { RouterProvider } from 'react-router-dom';
import '@ant-design/v5-patch-for-react-19';
import 'antd/dist/reset.css';
import './app/global.css';
import { Providers } from './app/providers';
import { createAppRouter } from './app/router';

async function prepareMocks(): Promise<void> {
  const mocksEnabled = import.meta.env.DEV && import.meta.env.VITE_ENABLE_MSW !== 'false';
  if (!mocksEnabled) return;
  const { startMockServiceWorker } = await import('./mocks/browser');
  await startMockServiceWorker();
}

async function bootstrap(): Promise<void> {
  await prepareMocks();

  const container = document.getElementById('root');
  if (!container) throw new Error('Не найден элемент #root');

  createRoot(container).render(
    <StrictMode>
      <Providers>
        <RouterProvider router={createAppRouter()} />
      </Providers>
    </StrictMode>,
  );
}

void bootstrap();
