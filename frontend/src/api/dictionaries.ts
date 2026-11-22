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
import type { EquipmentType, ProblemType, PublicSettings } from './types';

// Перегрузки: функции можно передавать в `queryFn` без аргументов (контекст
// React Query не путается с параметрами) и вызывать с `include_inactive`.
export function fetchProblemTypesForSelection(): Promise<ProblemType[]>;
export function fetchProblemTypesForSelection(params: {
  include_inactive?: boolean;
}): Promise<ProblemType[]>;
export function fetchProblemTypesForSelection(
  params: { include_inactive?: boolean } = {},
): Promise<ProblemType[]> {
  // Контракт Б7: `include_inactive` учитывается только для ADMIN.
  return apiRequest<ProblemType[]>('/problem-types', { query: params });
}

/** Типы оборудования для формы: активные значения справочника (EQ-2). */
export function fetchEquipmentTypesForSelection(): Promise<EquipmentType[]>;
export function fetchEquipmentTypesForSelection(params: {
  include_inactive?: boolean;
}): Promise<EquipmentType[]>;
export function fetchEquipmentTypesForSelection(
  params: { include_inactive?: boolean } = {},
): Promise<EquipmentType[]> {
  return apiRequest<EquipmentType[]>('/equipment-types', { query: params });
}

/**
 * Публичные настройки (аддитивное расширение Б7): название приложения,
 * календарь и подписи/цвета статусов для анонимных и авторизованных экранов.
 */
export function fetchPublicSettings(): Promise<PublicSettings> {
  return apiRequest<PublicSettings>('/public/settings', { skipAuth: true });
}
