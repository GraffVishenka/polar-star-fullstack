/**
 * MSW: health-эндпоинты (NFR-4, §3.13).
 *
 * Клиент их не вызывает (это проверки инфраструктуры), но контракт должен быть
 * покрыт полностью: docker healthcheck и e2e в Ф11 могут обращаться к /health.
 */
import { http } from 'msw';
import { json } from '../http';

export const healthHandlers = [
  http.get('*/health', () => json({ status: 'ok', version: 'mock' })),
  http.get('*/health/ready', () => json({ status: 'ok' })),
];
