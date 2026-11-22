/** Роутер React Router 7 (data router). */
import { createBrowserRouter, type RouterProviderProps } from 'react-router-dom';
import { routes } from './routes';

export function createAppRouter(): RouterProviderProps['router'] {
  return createBrowserRouter(routes);
}
