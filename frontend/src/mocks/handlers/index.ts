/**
 * Все MSW-хендлеры по контракту §3.13.
 *
 * Доменные агенты правят только свой файл (`kb.ts`, `equipment.ts`, ...),
 * чтобы не конфликтовать при слиянии.
 */
import { adminHandlers } from './admin';
import { authHandlers } from './auth';
import { equipmentHandlers } from './equipment';
import { fileHandlers } from './files';
import { healthHandlers } from './health';
import { kbHandlers } from './kb';
import { publicHandlers } from './public';
import { statsHandlers } from './stats';
import { ticketHandlers } from './tickets';

export const handlers = [
  ...authHandlers,
  ...publicHandlers,
  ...ticketHandlers,
  ...kbHandlers,
  ...equipmentHandlers,
  ...adminHandlers,
  ...statsHandlers,
  ...fileHandlers,
  ...healthHandlers,
];
