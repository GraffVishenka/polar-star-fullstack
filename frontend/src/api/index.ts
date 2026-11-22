/**
 * Точка входа слоя API: клиент, ошибки и доменные функции.
 *
 * Домены: auth, public, tickets, kb, equipment (+buildings/warehouses), admin,
 * stats, upload. Эндпоинты соответствуют PLAN.md §3.13.
 */
export * from './types';
export * from './errors';
export * from './client';
export * from './upload';
export * from './auth';
export * from './public';
export * from './tickets';
export * from './kb';
export * from './equipment';
export * from './admin';
export * from './dictionaries';
export * from './stats';
