/** MSW-сервер для Vitest: те же хендлеры, что и в браузере. */
import { setupServer } from 'msw/node';
import { handlers } from './handlers';

export const server = setupServer(...handlers);
