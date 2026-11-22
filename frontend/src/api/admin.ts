/** Admin-панель: пользователи, настройки, справочники, праздники, корзина (§3.2, §3.4, §3.10). */
import { apiRequest } from './client';
import type {
  AdminPasswordResetRequest,
  AdminUser,
  AdminUserCreateRequest,
  AdminUserListParams,
  AdminUserUpdateRequest,
  AppSettings,
  AppSettingsUpdateRequest,
  EquipmentType,
  Page,
  ProblemType,
  PushPublicKeyResponse,
  PushSubscription,
  PushSubscriptionRequest,
  TrashItem,
  TrashListParams,
  TrashType,
} from './types';

// --- Пользователи (ADMIN) -------------------------------------------------

export function fetchAdminUsers(params: AdminUserListParams = {}): Promise<Page<AdminUser>> {
  return apiRequest<Page<AdminUser>>('/admin/users', { query: params });
}

/** AUTH-4: для USER обязателен `building_id`, пароль временный. */
export function createAdminUser(body: AdminUserCreateRequest): Promise<AdminUser> {
  return apiRequest<AdminUser>('/admin/users', { method: 'POST', body });
}

/** AUTH-7/AUTH-8: смена роли/здания/блокировка с серверными проверками. */
export function updateAdminUser(id: number, body: AdminUserUpdateRequest): Promise<AdminUser> {
  return apiRequest<AdminUser>(`/admin/users/${id}`, { method: 'PATCH', body });
}

/** AUTH-5: ADMIN сбрасывает пароль, флаг must_change_password ставится на сервере. */
export function resetAdminUserPassword(id: number, body: AdminPasswordResetRequest): Promise<void> {
  return apiRequest<void>(`/admin/users/${id}/password`, { method: 'POST', body });
}

// --- Настройки ------------------------------------------------------------

export function fetchSettings(): Promise<AppSettings> {
  return apiRequest<AppSettings>('/admin/settings');
}

export function updateSettings(body: AppSettingsUpdateRequest): Promise<AppSettings> {
  return apiRequest<AppSettings>('/admin/settings', { method: 'PUT', body });
}

// --- Справочники ----------------------------------------------------------

export function fetchProblemTypes(): Promise<ProblemType[]> {
  return apiRequest<ProblemType[]>('/admin/problem-types');
}

export function createProblemType(body: { name: string }): Promise<ProblemType> {
  return apiRequest<ProblemType>('/admin/problem-types', { method: 'POST', body });
}

export function updateProblemType(id: number, body: { name: string }): Promise<ProblemType> {
  return apiRequest<ProblemType>(`/admin/problem-types/${id}`, { method: 'PATCH', body });
}

export function deleteProblemType(id: number): Promise<void> {
  return apiRequest<void>(`/admin/problem-types/${id}`, { method: 'DELETE' });
}

export function fetchEquipmentTypes(): Promise<EquipmentType[]> {
  return apiRequest<EquipmentType[]>('/admin/equipment-types');
}

export function createEquipmentType(body: { name: string }): Promise<EquipmentType> {
  return apiRequest<EquipmentType>('/admin/equipment-types', { method: 'POST', body });
}

export function updateEquipmentType(id: number, body: { name: string }): Promise<EquipmentType> {
  return apiRequest<EquipmentType>(`/admin/equipment-types/${id}`, { method: 'PATCH', body });
}

export function deleteEquipmentType(id: number): Promise<void> {
  return apiRequest<void>(`/admin/equipment-types/${id}`, { method: 'DELETE' });
}

// --- Праздники (CAL-1) ----------------------------------------------------

export function fetchHolidays(): Promise<AppSettings['holidays']> {
  return apiRequest<AppSettings['holidays']>('/admin/holidays');
}

export function createHoliday(body: {
  date: string;
  name: string;
}): Promise<AppSettings['holidays'][number]> {
  return apiRequest<AppSettings['holidays'][number]>('/admin/holidays', { method: 'POST', body });
}

export function deleteHoliday(id: number): Promise<void> {
  return apiRequest<void>(`/admin/holidays/${id}`, { method: 'DELETE' });
}

// --- Корзина (TRASH-1..3) -------------------------------------------------

export function fetchTrash(params: TrashListParams): Promise<Page<TrashItem>> {
  return apiRequest<Page<TrashItem>>('/admin/trash', { query: params });
}

export function restoreTrashItem(type: TrashType, id: number): Promise<void> {
  return apiRequest<void>(`/admin/trash/${type}/${id}/restore`, { method: 'POST' });
}

/** TRASH-3: физическое удаление необратимо, требует подтверждения в UI. */
export function purgeTrashItem(type: TrashType, id: number): Promise<void> {
  return apiRequest<void>(`/admin/trash/${type}/${id}`, { method: 'DELETE' });
}

// --- Push (NOTIF-5) -------------------------------------------------------

export function fetchPushPublicKey(): Promise<PushPublicKeyResponse> {
  return apiRequest<PushPublicKeyResponse>('/push/public-key');
}

export function subscribePush(body: PushSubscriptionRequest): Promise<PushSubscription> {
  return apiRequest<PushSubscription>('/push/subscriptions', { method: 'POST', body });
}

export function unsubscribePush(id: number): Promise<void> {
  return apiRequest<void>(`/push/subscriptions/${id}`, { method: 'DELETE' });
}
