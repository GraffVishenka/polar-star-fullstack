/**
 * Типы API — временно описаны вручную строго по PLAN.md §3.13 (и §3.2–§3.10).
 *
 * TODO: генерировать из openapi/openapi.yaml после Б1 (контракт ещё не опубликован).
 *       После генерации файл не редактируется вручную: правки только через контракт.
 *
 * Правило проекта (NFR-7): даты, дедлайны и признак просрочки вычисляет сервер
 * в настроенном `timezone`; клиент получает готовые `deadline` и `is_overdue`.
 */

/** Роли по §3.1. */
export type Role = 'USER' | 'ADMIN';

/** Коды статусов заявки фиксированы (§3.3 TICKET-1), названия и цвета — из настроек. */
export type TicketStatus = 'new' | 'in_progress' | 'deferred' | 'done';

/** Типы сущностей корзины (§3.10). */
export type TrashType = 'ticket' | 'kb' | 'equipment';

/** Тип медиа в карточке комментария / статьи БЗ. */
export type MediaKind = 'image' | 'video' | 'document';

/** Единый формат ошибки API (§3.13). */
export interface ApiErrorResponse {
  code: string;
  message: string;
  details?: Record<string, unknown> | null;
  request_id: string;
}

/** Ответ с серверной пагинацией (§3.3 TICKET-15). */
export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  size: number;
}

// ---------------------------------------------------------------------------
// Auth (§3.2)
// ---------------------------------------------------------------------------

export interface AuthUser {
  id: number;
  login: string;
  full_name: string;
  role: Role;
  /** У ADMIN всегда null (§3.1). */
  building_id: number | null;
  building_name: string | null;
  /** AUTH-4: до смены пароля разрешены только me/password/logout. */
  must_change_password: boolean;
  is_active: boolean;
  created_at: string;
}

export interface LoginRequest {
  login: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  token_type: 'Bearer';
  /** TTL access-токена в секундах (AUTH-2: 15 минут). */
  expires_in: number;
  user: AuthUser;
}

export interface RefreshResponse {
  access_token: string;
  token_type: 'Bearer';
  expires_in: number;
  /** Необязательно: если сервер не отдаёт пользователя, клиент запрашивает `me`. */
  user?: AuthUser;
}

export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

// ---------------------------------------------------------------------------
// Здания, склады (§3.6 EQ-1)
// ---------------------------------------------------------------------------

/** Счётчики карточки здания (§3.3 TICKET-6): `done` в публичном выборе не учитывается. */
export interface BuildingCounts {
  new: number;
  in_progress: number;
  deferred: number;
}

/** Публичный список зданий: только активные, без складов и пользователей. */
export interface PublicBuilding {
  id: number;
  name: string;
  address: string | null;
  counts: BuildingCounts;
}

/** Здание для авторизованных экранов (с флагом активности, EQ-1). */
export interface Building {
  id: number;
  name: string;
  address: string | null;
  is_active: boolean;
  created_at: string;
  counts?: BuildingCounts;
}

export interface BuildingCreateRequest {
  name: string;
  address?: string | null;
}

export interface BuildingUpdateRequest {
  name?: string;
  address?: string | null;
  is_active?: boolean;
}

/** Склад/кабинет: одна сущность, имя уникально внутри здания (EQ-1). */
export interface Warehouse {
  id: number;
  building_id: number;
  building_name: string;
  name: string;
  created_at?: string;
}

export interface WarehouseCreateRequest {
  building_id: number;
  name: string;
}

export interface WarehouseUpdateRequest {
  name?: string;
}

// ---------------------------------------------------------------------------
// Файлы и медиа (§3.3 TICKET-14, §3.5 KB-5, Б3)
// ---------------------------------------------------------------------------

export interface FileMeta {
  id: number;
  filename: string;
  content_type: string;
  size_bytes: number;
  media_kind: MediaKind;
  /** Относительный URL выдачи файла под проверкой прав. */
  url: string;
  created_at: string;
}

/** Область загрузки: влияет на права выдачи файла. */
export type FileScope = 'ticket' | 'kb';

export interface FileUploadResponse {
  file: FileMeta;
}

// ---------------------------------------------------------------------------
// Заявки (§3.3)
// ---------------------------------------------------------------------------

export interface TicketComment {
  id: number;
  ticket_id: number;
  author_name: string;
  author_type: 'user' | 'anonymous';
  body: string;
  created_at: string;
  files: FileMeta[];
}

/** Публичная карточка дашборда (§3.3 TICKET-5): без description, комментариев и файлов. */
export interface PublicTicketCard {
  id: number;
  building_name: string;
  applicant_name: string;
  cabinet: string;
  problem_type_name: string;
  status: TicketStatus;
  status_label: string;
  status_color: string;
  deadline: string | null;
  executor_name: string | null;
  created_at: string;
}

export interface PublicTicketList {
  items: PublicTicketCard[];
  total: number;
}

/** Публичная карточка по токену (§3.3 TICKET-13, PUB-6). */
export interface PublicTicket {
  id: number;
  building_id: number;
  building_name: string;
  applicant_name: string;
  cabinet: string;
  problem_type_name: string;
  status: TicketStatus;
  status_label: string;
  status_color: string;
  deadline: string | null;
  is_overdue: boolean;
  executor_name: string | null;
  description: string;
  created_at: string;
  completed_at: string | null;
  comments: TicketComment[];
  files: FileMeta[];
}

export interface PublicTicketCreateRequest {
  building_id: number;
  applicant_name: string;
  cabinet: string;
  problem_type_id: number;
  description: string;
}

/** Ответ на публичное создание: токен выдаётся один раз (TICKET-13). */
export interface PublicTicketCreated {
  id: number;
  access_token: string;
  /** Готовый путь карточки `/t/{token}`. */
  url: string;
  deadline: string | null;
  created_at: string;
  status: TicketStatus;
  status_label: string;
}

export interface PublicCommentCreateRequest {
  author_name?: string;
  body: string;
  file_ids?: number[];
}

/** Полная заявка для авторизованных. */
export interface Ticket {
  id: number;
  building_id: number;
  building_name: string;
  applicant_name: string;
  cabinet: string;
  problem_type_id: number;
  problem_type_name: string;
  description: string;
  deadline: string | null;
  /** TICKET-16: флаг считает сервер, клиент только отображает. */
  is_overdue: boolean;
  executor_id: number | null;
  executor_name: string | null;
  executor_role: Role | null;
  status: TicketStatus;
  status_label: string;
  status_color: string;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
  is_anonymous: boolean;
  created_by_user_id: number | null;
  comments_count: number;
  files: FileMeta[];
}

export interface TicketListParams {
  page?: number;
  size?: number;
  /** Мультивыбор статусов (TICKET-15). */
  status?: TicketStatus[];
  executor_id?: number | null;
  problem_type_id?: number;
  /** У USER игнорируется сервером и заблокирован в UI (TICKET-7). */
  building_id?: number | 'all';
  date_from?: string;
  date_to?: string;
  search?: string;
  sort_by?: 'created_at' | 'deadline';
  sort_dir?: 'asc' | 'desc';
}

export interface TicketCreateRequest {
  building_id: number;
  applicant_name: string;
  cabinet: string;
  problem_type_id: number;
  description: string;
}

/** TICKET-9/TICKET-11: набор полей, доступных к правке по роли. */
export interface TicketUpdateRequest {
  applicant_name?: string;
  cabinet?: string;
  problem_type_id?: number;
  description?: string;
  status?: TicketStatus;
  executor_id?: number | null;
  /** Только ADMIN (TICKET-9). */
  deadline?: string | null;
  /** Только ADMIN; при смене здания USER-исполнитель сбрасывается (TICKET-9). */
  building_id?: number;
}

export interface TicketUpdateResponse extends Ticket {
  /** TICKET-9: исполнитель-USER из другого здания сброшен. */
  executor_reset?: boolean;
}

export interface TicketCommentCreateRequest {
  body: string;
  file_ids?: number[];
}

export interface AssignableUser {
  id: number;
  login: string;
  full_name: string;
  role: Role;
  building_id: number | null;
  building_name: string | null;
}

// ---------------------------------------------------------------------------
// База знаний (§3.5)
// ---------------------------------------------------------------------------

export interface KbNode {
  id: number;
  title: string;
  content_html: string;
  parent_id: number | null;
  sort_order: number;
  /** Тумблер только у корневых, у потомков наследуется (KB-1). */
  is_hidden_for_anon: boolean;
  /** Итоговая видимость с учётом наследования. */
  is_hidden_effective: boolean;
  created_at: string;
  updated_at: string;
  updated_by_name: string | null;
  media: FileMeta[];
}

/** Узел дерева БЗ (KB-8: дерево приходит одним запросом). */
export interface KbTreeNode {
  id: number;
  title: string;
  parent_id: number | null;
  sort_order: number;
  is_hidden_for_anon: boolean;
  is_hidden_effective: boolean;
  has_children: boolean;
  /** Вложенность не ограничена (KB-2); UI обязан поддерживать глубину ≥ 5. */
  children: KbTreeNode[];
}

export interface KbNodeCreateRequest {
  title: string;
  content_html?: string;
  parent_id?: number | null;
  sort_order?: number;
  is_hidden_for_anon?: boolean;
}

export interface KbNodeUpdateRequest {
  title?: string;
  content_html?: string;
  parent_id?: number | null;
  sort_order?: number;
  is_hidden_for_anon?: boolean;
}

export interface KbMoveRequest {
  parent_id: number | null;
  before_node_id?: number | null;
}

/** Ответ на удаление узла: число удалённых потомков (KB-3). */
export interface KbDeleteResponse {
  deleted_id: number;
  descendants_deleted: number;
}

// ---------------------------------------------------------------------------
// Оборудование (§3.6)
// ---------------------------------------------------------------------------

export interface EquipmentType {
  id: number;
  name: string;
  /**
   * Аддитивно (Б7/Ф6): порядок в справочнике и признак активности.
   * Деактивированный тип остаётся в старых записях оборудования.
   */
  sort_order?: number;
  is_active?: boolean;
  /** Сколько действующих записей ссылается на тип (показывает админка). */
  usage_count?: number;
}

export interface ProblemType {
  id: number;
  name: string;
  /** Аддитивно (Б7/Ф6): порядок в справочнике и признак активности. */
  sort_order?: number;
  is_active?: boolean;
  /** Сколько действующих заявок ссылается на вид проблемы. */
  usage_count?: number;
}

export interface Equipment {
  id: number;
  name: string;
  equipment_type_id: number;
  equipment_type_name: string;
  serial_number: string;
  registration_number: string;
  warehouse_id: number;
  warehouse_name: string;
  building_id: number;
  building_name: string;
  /** Неактивное здание: перемещение внутрь разрешено только ADMIN (EQ-3). */
  is_building_active: boolean;
  created_at: string;
}

export interface EquipmentListParams {
  page?: number;
  size?: number;
  building_id?: number;
  warehouse_id?: number;
  equipment_type_id?: number;
  search?: string;
}

export interface EquipmentCreateRequest {
  name: string;
  equipment_type_id: number;
  serial_number: string;
  registration_number: string;
  warehouse_id: number;
}

export interface EquipmentUpdateRequest {
  name?: string;
  equipment_type_id?: number;
  serial_number?: string;
  registration_number?: string;
  warehouse_id?: number;
}

export interface EquipmentMovement {
  id: number;
  equipment_id: number;
  equipment_name: string;
  from_warehouse_id: number;
  from_warehouse_name: string;
  from_building_name: string;
  to_warehouse_id: number;
  to_warehouse_name: string;
  to_building_name: string;
  moved_by_user_id: number;
  moved_by_name: string;
  moved_at: string;
  comment: string | null;
}

export interface EquipmentMoveRequest {
  to_warehouse_id: number;
  comment?: string;
}

export interface EquipmentBulkMoveRequest {
  equipment_ids: number[];
  to_warehouse_id: number;
  comment?: string;
}

export interface EquipmentBulkMoveResponse {
  moved_count: number;
  to_warehouse_id: number;
  to_warehouse_name: string;
  movements_created: number;
}

// ---------------------------------------------------------------------------
// Настройки и справочники (§3.2 AUTH-4, §3.4 CAL, Б7)
// ---------------------------------------------------------------------------

/** Маска рабочих дней: ISO-подобные коды, Пн–Сб по умолчанию (CAL-1). */
export type Weekday = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun';

export interface TicketStatusSetting {
  code: TicketStatus;
  label: string;
  color: string;
}

export interface Holiday {
  id: number;
  date: string;
  name: string;
}

export interface AppSettings {
  app_name: string;
  /** CAL: 1–30 рабочих дней, по умолчанию 2 (TICKET-3). */
  deadline_days: number;
  workdays: Weekday[];
  /** IANA-идентификатор, по умолчанию Asia/Yekaterinburg (CAL-2). */
  timezone: string;
  /** HH:MM в `timezone` (CAL-3). */
  deadline_reminder_time: string;
  push_enabled: boolean;
  statuses: TicketStatusSetting[];
  holidays: Holiday[];
  updated_at: string;
}

export interface AppSettingsUpdateRequest {
  app_name?: string;
  deadline_days?: number;
  workdays?: Weekday[];
  timezone?: string;
  deadline_reminder_time?: string;
  push_enabled?: boolean;
  statuses?: TicketStatusSetting[];
}

/**
 * Публичные настройки (`GET /api/public/settings`) — аддитивное расширение
 * контракта из Б7: анонимной форме заявки и публичным экранам нужны название
 * приложения и подписи/цвета статусов. Без паролей, праздников и push-флагов.
 */
export interface PublicSettings {
  app_name: string;
  deadline_days: number;
  workdays: Weekday[];
  timezone: string;
  statuses: TicketStatusSetting[];
}

// ---------------------------------------------------------------------------
// Пользователи и админ-панель (§3.2, §3.13)
// ---------------------------------------------------------------------------

export interface AdminUser extends AuthUser {
  last_login_at: string | null;
}

export interface AdminUserListParams {
  page?: number;
  size?: number;
  role?: Role;
  building_id?: number;
  is_active?: boolean;
  search?: string;
}

export interface AdminUserCreateRequest {
  login: string;
  full_name: string;
  role: Role;
  /** Обязателен для USER (AUTH-4): 422 BUILDING_REQUIRED_FOR_USER. */
  building_id?: number | null;
  /** Временный пароль ≥ 8 символов. */
  password: string;
}

export interface AdminUserUpdateRequest {
  full_name?: string;
  role?: Role;
  building_id?: number | null;
  is_active?: boolean;
}

export interface AdminPasswordResetRequest {
  /** Новый временный пароль ≥ 8 символов. */
  password: string;
}

// ---------------------------------------------------------------------------
// Статистика (§3.8)
// ---------------------------------------------------------------------------

export interface ProblemTypeStat {
  problem_type_id: number;
  problem_type_name: string;
  total: number;
  done: number;
}

export interface MonthStat {
  /** `YYYY-MM` в `timezone` приложения. */
  month: string;
  done: number;
}

export interface StatsResponse {
  user_id: number;
  user_name: string;
  from: string;
  to: string;
  total_done: number;
  done_in_period: number;
  /** null, если завершённых заявок нет (округление до 0.1 часа). */
  avg_resolution_hours: number | null;
  median_resolution_hours: number | null;
  /** null, если завершённых заявок нет (STATS-2). */
  on_time_percent: number | null;
  overdue_count: number;
  by_problem_type: ProblemTypeStat[];
  by_month: MonthStat[];
}

export interface StatsParams {
  from?: string;
  to?: string;
}

export interface AdminStatsParams extends StatsParams {
  user_id?: number;
  building_id?: number;
}

// ---------------------------------------------------------------------------
// Корзина (§3.10)
// ---------------------------------------------------------------------------

export interface TrashItem {
  id: number;
  type: TrashType;
  title: string;
  description: string | null;
  deleted_at: string;
  deleted_by_name: string;
  /** Предупреждения восстановления (TRASH-2): неактивное здание, конфликт номера. */
  warnings: string[];
}

export interface TrashListParams {
  type: TrashType;
  page?: number;
  size?: number;
}

// ---------------------------------------------------------------------------
// Push (§3.7)
// ---------------------------------------------------------------------------

export interface PushPublicKeyResponse {
  public_key: string;
}

export interface PushSubscriptionRequest {
  endpoint: string;
  keys: {
    p256dh: string;
    auth: string;
  };
  user_agent?: string;
}

export interface PushSubscription {
  id: number;
  endpoint: string;
  created_at: string;
  last_success_at: string | null;
  is_active: boolean;
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

export interface HealthResponse {
  status: 'ok' | 'degraded';
  version?: string;
}
