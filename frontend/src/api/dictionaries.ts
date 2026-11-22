/**
 * Справочники, доступные всем авторизованным (формы и фильтры заявок).
 *
 * Контракт §3.13 отдаёт виды проблем только через ADMIN-эндпоинт
 * `/api/admin/problem-types`, но форма создания заявки доступна USER (TICKET-4),
 * поэтому нужен авторизованный read-only список. MSW реализует
 * `GET /api/problem-types`; это **аддитивное расширение контракта** — при
 * фиксации `openapi.yaml` согласовать с lead/be (то же касается публичных
 * настроек для названий статусов).
 */
import { apiRequest } from './client';
import type { ProblemType } from './types';

export function fetchProblemTypesForSelection(): Promise<ProblemType[]> {
  return apiRequest<ProblemType[]>('/problem-types');
}
