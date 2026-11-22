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

export function fetchProblemTypesForSelection(): Promise<ProblemType[]> {
  return apiRequest<ProblemType[]>('/problem-types');
}

/** Типы оборудования для формы: активные значения справочника (EQ-2). */
export function fetchEquipmentTypesForSelection(): Promise<EquipmentType[]> {
  return apiRequest<EquipmentType[]>('/equipment-types');
}

/**
 * Публичные настройки (аддитивное расширение Б7): название приложения,
 * календарь и подписи/цвета статусов для анонимных и авторизованных экранов.
 */
export function fetchPublicSettings(): Promise<PublicSettings> {
  return apiRequest<PublicSettings>('/public/settings', { skipAuth: true });
}
