/**
 * In-memory база MSW-моков по контракту §3.13.
 *
 * Назначение: другие агенты (fe-2, fe-3) работают без бэкенда — все эндпоинты
 * отвечают реалистичными данными, включая ошибки (409/422/428), счётчики
 * зданий, дерево БЗ глубины 6, журнал перемещений и корзину.
 *
 * Мутации сохраняются в памяти браузера до перезагрузки страницы.
 */
import type {
  AdminUser,
  AssignableUser,
  Building,
  BuildingCounts,
  Equipment,
  EquipmentListParams,
  EquipmentMovement,
  EquipmentBulkMoveResponse,
  FileMeta,
  KbDeleteResponse,
  KbNode,
  KbTreeNode,
  MediaKind,
  MonthStat,
  ProblemTypeStat,
  PublicBuilding,
  PublicTicket,
  PublicTicketCard,
  Role,
  StatsResponse,
  Ticket,
  TicketComment,
  TicketListParams,
  TicketStatus,
  TrashItem,
  TrashType,
  Warehouse,
} from '@/api/types';

// ---------------------------------------------------------------------------
// Внутренние модели (в моках храним пароли и служебные поля)
// ---------------------------------------------------------------------------

export interface MockUser {
  id: number;
  login: string;
  password: string;
  full_name: string;
  role: Role;
  building_id: number | null;
  is_active: boolean;
  must_change_password: boolean;
  created_at: string;
  last_login_at: string | null;
}

export interface MockBuilding {
  id: number;
  name: string;
  address: string | null;
  is_active: boolean;
  created_at: string;
}

export interface MockWarehouse {
  id: number;
  building_id: number;
  name: string;
  created_at: string;
}

export interface MockEquipment {
  id: number;
  name: string;
  equipment_type_id: number;
  serial_number: string;
  registration_number: string;
  warehouse_id: number;
  created_at: string;
  deleted_at: string | null;
}

export interface MockMovement {
  id: number;
  equipment_id: number;
  from_warehouse_id: number;
  to_warehouse_id: number;
  moved_by_user_id: number;
  moved_at: string;
  comment: string | null;
}

export interface MockTicket {
  id: number;
  building_id: number;
  applicant_name: string;
  cabinet: string;
  problem_type_id: number;
  description: string;
  deadline: string | null;
  executor_id: number | null;
  status: TicketStatus;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
  created_by_user_id: number | null;
  access_token: string | null;
  is_anonymous: boolean;
  deleted_at: string | null;
}

export interface MockComment {
  id: number;
  ticket_id: number;
  author_name: string;
  author_type: 'user' | 'anonymous';
  body: string;
  created_at: string;
  file_ids: number[];
}

export interface MockFile {
  id: number;
  filename: string;
  content_type: string;
  size_bytes: number;
  media_kind: MediaKind;
  created_at: string;
  scope: 'ticket' | 'kb';
  ticket_id: number | null;
  token: string | null;
}

export interface MockKbNode {
  id: number;
  title: string;
  content_html: string;
  parent_id: number | null;
  sort_order: number;
  is_hidden_for_anon: boolean;
  created_at: string;
  updated_at: string;
  updated_by_name: string | null;
  media_ids: number[];
}

export interface MockHoliday {
  id: number;
  date: string;
  name: string;
}

export interface MockSettings {
  app_name: string;
  deadline_days: number;
  workdays: string[];
  timezone: string;
  deadline_reminder_time: string;
  push_enabled: boolean;
  statuses: Array<{ code: TicketStatus; label: string; color: string }>;
  updated_at: string;
}

export interface MockTrashItem {
  type: TrashType;
  id: number;
  deleted_at: string;
  deleted_by_user_id: number;
}

export interface MockPushSubscription {
  id: number;
  user_id: number;
  endpoint: string;
  keys: { p256dh: string; auth: string };
  user_agent: string | null;
  created_at: string;
  last_success_at: string | null;
  is_active: boolean;
}

export interface MockDb {
  buildings: MockBuilding[];
  users: MockUser[];
  problemTypes: Array<{ id: number; name: string }>;
  equipmentTypes: Array<{ id: number; name: string }>;
  warehouses: MockWarehouse[];
  equipment: MockEquipment[];
  movements: MockMovement[];
  tickets: MockTicket[];
  comments: MockComment[];
  files: MockFile[];
  kbNodes: MockKbNode[];
  holidays: MockHoliday[];
  settings: MockSettings;
  trash: MockTrashItem[];
  pushSubscriptions: MockPushSubscription[];
}

// ---------------------------------------------------------------------------
// Утилиты дат: «сегодня» считается в поясе приложения (Asia/Yekaterinburg, +5)
// ---------------------------------------------------------------------------

const DAY_MS = 86_400_000;
const APP_UTC_OFFSET_MINUTES = 5 * 60;

/** Текущие сутки в поясе приложения, `YYYY-MM-DD`. */
export function appToday(): string {
  const shifted = new Date(Date.now() + APP_UTC_OFFSET_MINUTES * 60_000);
  return shifted.toISOString().slice(0, 10);
}

function isoDaysAgo(days: number, hour = 9, minute = 0): string {
  const date = new Date(Date.now() - days * DAY_MS);
  date.setUTCHours(hour - 5, minute, 0, 0); // пояс приложения UTC+5
  return date.toISOString();
}

function dateDaysFromNow(days: number): string {
  const date = new Date(Date.now() + days * DAY_MS);
  return date.toISOString().slice(0, 10);
}

/** Дедлайн по рабочим дням: маска Пн–Сб, как в CAL-1 по умолчанию. */
export function addWorkdays(fromIso: string, days: number, workdays: string[]): string {
  const weekdays = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
  const date = new Date(`${fromIso}T00:00:00Z`);
  let added = 0;
  while (added < days) {
    date.setUTCDate(date.getUTCDate() + 1);
    if (workdays.includes(weekdays[date.getUTCDay()]!)) added += 1;
  }
  return date.toISOString().slice(0, 10);
}

export function isOverdue(ticket: MockTicket): boolean {
  if (!ticket.deadline || ticket.status === 'done') return false;
  return ticket.deadline < appToday();
}

// ---------------------------------------------------------------------------
// Сид
// ---------------------------------------------------------------------------

function seedBuildings(): MockBuilding[] {
  return [
    {
      id: 1,
      name: 'Центральный офис',
      address: 'ул. Ленина, 24',
      is_active: true,
      created_at: isoDaysAgo(400),
    },
    {
      id: 2,
      name: 'Производственный корпус',
      address: 'ул. Заводская, 7',
      is_active: true,
      created_at: isoDaysAgo(380),
    },
    {
      id: 3,
      name: 'Склад №3',
      address: 'пер. Складской, 1',
      is_active: true,
      created_at: isoDaysAgo(300),
    },
    { id: 4, name: 'Старый корпус', address: null, is_active: false, created_at: isoDaysAgo(700) },
  ];
}

function seedUsers(): MockUser[] {
  return [
    {
      id: 1,
      login: 'admin',
      password: 'admin12345',
      full_name: 'Администратор Системы',
      role: 'ADMIN',
      building_id: null,
      is_active: true,
      must_change_password: false,
      created_at: isoDaysAgo(400),
      last_login_at: isoDaysAgo(1),
    },
    {
      id: 2,
      login: 'user',
      password: 'user12345',
      full_name: 'Иванов Иван Иванович',
      role: 'USER',
      building_id: 1,
      is_active: true,
      must_change_password: false,
      created_at: isoDaysAgo(300),
      last_login_at: isoDaysAgo(2),
    },
    {
      id: 3,
      login: 'petrova',
      password: 'user12345',
      full_name: 'Петрова Анна Сергеевна',
      role: 'USER',
      building_id: 2,
      is_active: true,
      must_change_password: false,
      created_at: isoDaysAgo(280),
      last_login_at: null,
    },
    {
      id: 4,
      login: 'sidorov',
      password: 'user12345',
      full_name: 'Сидоров Пётр Алексеевич',
      role: 'USER',
      building_id: 1,
      is_active: true,
      must_change_password: false,
      created_at: isoDaysAgo(250),
      last_login_at: null,
    },
    {
      id: 5,
      login: 'kuznetsova',
      password: 'user12345',
      full_name: 'Кузнецова Ольга Игоревна',
      role: 'USER',
      building_id: 3,
      is_active: false,
      must_change_password: false,
      created_at: isoDaysAgo(200),
      last_login_at: isoDaysAgo(90),
    },
    {
      id: 6,
      login: 'bez-zdaniya',
      password: 'user12345',
      full_name: 'Смирнов Без Здания',
      role: 'USER',
      building_id: null,
      is_active: true,
      must_change_password: false,
      created_at: isoDaysAgo(10),
      last_login_at: null,
    },
    {
      id: 7,
      login: 'novichok',
      password: 'temp12345',
      full_name: 'Новиков Николай',
      role: 'USER',
      building_id: 2,
      is_active: true,
      must_change_password: true,
      created_at: isoDaysAgo(3),
      last_login_at: null,
    },
  ];
}

interface SeedTicketInput {
  id: number;
  building_id: number;
  applicant_name: string;
  cabinet: string;
  problem_type_id: number;
  description: string;
  status: TicketStatus;
  createdDaysAgo: number;
  deadlineInDays: number | null;
  executor_id?: number | null;
  created_by_user_id?: number | null;
  access_token?: string | null;
  completedDaysAgo?: number | null;
  deleted_at?: string | null;
}

function makeTicket(input: SeedTicketInput): MockTicket {
  const created = isoDaysAgo(input.createdDaysAgo, 10);
  return {
    id: input.id,
    building_id: input.building_id,
    applicant_name: input.applicant_name,
    cabinet: input.cabinet,
    problem_type_id: input.problem_type_id,
    description: input.description,
    deadline: input.deadlineInDays === null ? null : dateDaysFromNow(input.deadlineInDays),
    executor_id: input.executor_id ?? null,
    status: input.status,
    completed_at:
      input.completedDaysAgo === undefined || input.completedDaysAgo === null
        ? null
        : isoDaysAgo(input.completedDaysAgo, 16),
    created_at: created,
    updated_at: created,
    created_by_user_id: input.created_by_user_id ?? null,
    access_token: input.access_token ?? null,
    is_anonymous: input.created_by_user_id === null || input.created_by_user_id === undefined,
    deleted_at: input.deleted_at ?? null,
  };
}

function seedTickets(): MockTicket[] {
  return [
    makeTicket({
      id: 1,
      building_id: 1,
      applicant_name: 'Иванов Иван Иванович',
      cabinet: 'каб. 204',
      problem_type_id: 1,
      description: 'Не работает освещение в переговорной, мигает лампа.',
      status: 'new',
      createdDaysAgo: 1,
      deadlineInDays: 1,
      created_by_user_id: 2,
    }),
    makeTicket({
      id: 2,
      building_id: 1,
      applicant_name: 'Сидоров Пётр Алексеевич',
      cabinet: 'каб. 310',
      problem_type_id: 2,
      description: 'Протечка под раковиной в столовой, вода на полу.',
      status: 'in_progress',
      createdDaysAgo: 3,
      deadlineInDays: -1,
      executor_id: 2,
      created_by_user_id: 4,
    }),
    makeTicket({
      id: 3,
      building_id: 1,
      applicant_name: 'Козлова Мария',
      cabinet: 'каб. 105',
      problem_type_id: 3,
      description: 'Принтер в бухгалтерии не захватывает бумагу.',
      status: 'deferred',
      createdDaysAgo: 5,
      deadlineInDays: 2,
      executor_id: 2,
      created_by_user_id: 2,
    }),
    makeTicket({
      id: 4,
      building_id: 1,
      applicant_name: 'Анонимный посетитель',
      cabinet: 'каб. 12',
      problem_type_id: 4,
      description: 'Требуется уборка после ремонта.',
      status: 'done',
      createdDaysAgo: 9,
      deadlineInDays: -5,
      executor_id: 2,
      created_by_user_id: null,
      access_token: 'demo-token-1',
      completedDaysAgo: 7,
    }),
    makeTicket({
      id: 5,
      building_id: 2,
      applicant_name: 'Петрова Анна Сергеевна',
      cabinet: 'цех А',
      problem_type_id: 3,
      description: 'Станок №4 останавливается через 10 минут работы.',
      status: 'new',
      createdDaysAgo: 2,
      deadlineInDays: 0,
      created_by_user_id: 3,
    }),
    makeTicket({
      id: 6,
      building_id: 2,
      applicant_name: 'Анонимный посетитель',
      cabinet: 'склад сырья',
      problem_type_id: 2,
      description: 'Течёт крыша над стеллажами.',
      status: 'in_progress',
      createdDaysAgo: 6,
      deadlineInDays: 3,
      executor_id: 3,
      created_by_user_id: null,
      access_token: 'demo-token-2',
    }),
    makeTicket({
      id: 7,
      building_id: 2,
      applicant_name: 'Григорьев Олег',
      cabinet: 'цех Б',
      problem_type_id: 1,
      description: 'Провести свет в зону упаковки.',
      status: 'deferred',
      createdDaysAgo: 12,
      deadlineInDays: -2,
      executor_id: 3,
      created_by_user_id: 3,
    }),
    makeTicket({
      id: 8,
      building_id: 2,
      applicant_name: 'Петрова Анна Сергеевна',
      cabinet: 'лаборатория',
      problem_type_id: 5,
      description: 'Заменить замок в двери.',
      status: 'done',
      createdDaysAgo: 20,
      deadlineInDays: 2,
      executor_id: 3,
      created_by_user_id: 3,
      completedDaysAgo: 18,
    }),
    makeTicket({
      id: 9,
      building_id: 3,
      applicant_name: 'Анонимный посетитель',
      cabinet: 'зона отгрузки',
      problem_type_id: 3,
      description: 'Погрузчик не заводится.',
      status: 'in_progress',
      createdDaysAgo: 4,
      deadlineInDays: -1,
      created_by_user_id: null,
      access_token: 'demo-token-3',
    }),
    makeTicket({
      id: 10,
      building_id: 3,
      applicant_name: 'Кузнецова Ольга',
      cabinet: 'склад',
      problem_type_id: 4,
      description: 'Вывезти упаковку после поставки.',
      status: 'new',
      createdDaysAgo: 1,
      deadlineInDays: 2,
      created_by_user_id: 5,
    }),
    makeTicket({
      id: 11,
      building_id: 3,
      applicant_name: 'Анонимный посетитель',
      cabinet: 'проходная',
      problem_type_id: 5,
      description: 'Заявка создана анонимно для проверки ссылки-токена.',
      status: 'new',
      createdDaysAgo: 0,
      deadlineInDays: 2,
      created_by_user_id: null,
      access_token: 'demo-token-11',
    }),
    makeTicket({
      id: 12,
      building_id: 1,
      applicant_name: 'Иванов Иван Иванович',
      cabinet: 'каб. 402',
      problem_type_id: 1,
      description: 'Заменить перегоревшую лампу.',
      status: 'done',
      createdDaysAgo: 30,
      deadlineInDays: -25,
      executor_id: 2,
      created_by_user_id: 2,
      completedDaysAgo: 28,
    }),
    makeTicket({
      id: 13,
      building_id: 1,
      applicant_name: 'Удалённая заявка',
      cabinet: 'каб. 1',
      problem_type_id: 5,
      description: 'Заявка удалена в корзину администратором.',
      status: 'done',
      createdDaysAgo: 40,
      deadlineInDays: -35,
      created_by_user_id: 2,
      completedDaysAgo: 38,
      deleted_at: isoDaysAgo(3),
    }),
  ];
}

function seedKbNodes(): MockKbNode[] {
  const base = isoDaysAgo(120);
  const node = (
    id: number,
    title: string,
    parent_id: number | null,
    sort_order: number,
    options: Partial<MockKbNode> = {},
  ): MockKbNode => ({
    id,
    title,
    parent_id,
    sort_order,
    content_html: `<p>${title}</p><p>Описание раздела и порядок действий.</p>`,
    is_hidden_for_anon: false,
    created_at: base,
    updated_at: base,
    updated_by_name: 'Администратор Системы',
    media_ids: [],
    ...options,
  });

  return [
    node(1, 'Инструкции', null, 10),
    node(2, 'Заявки', 1, 10),
    node(3, 'Как создать заявку', 2, 10, {
      content_html:
        '<h2>Создание заявки</h2><p>Выберите здание, кабинет и вид проблемы.</p><img src="/api/files/1" alt="Форма заявки"><p>После отправки сохраните ссылку.</p>',
      media_ids: [1],
    }),
    node(4, 'Поля формы', 3, 10),
    node(5, 'Дедлайн', 4, 10, {
      content_html:
        '<p>Дедлайн считается на сервере: к дате создания прибавляются рабочие дни.</p>',
    }),
    node(6, 'Рабочий календарь', 5, 10, {
      content_html:
        '<p>По умолчанию Пн–Сб. Праздники исключаются. Изменение настроек не пересчитывает старые дедлайны.</p>',
    }),
    node(7, 'Оборудование', null, 20),
    node(8, 'Перемещения', 7, 10, { media_ids: [2] }),
    node(9, 'Ведомость помещения', 8, 10),
    node(10, 'Администрирование', null, 30, { is_hidden_for_anon: true }),
    node(11, 'Пользователи и роли', 10, 10, { media_ids: [1] }),
    node(12, 'Заявки на проверку', 10, 20),
    // Ф3: второй скрытый корень с потомком — проверка наследования флага
    // (KB-1/KB-7): замок показывается и у дочернего раздела.
    node(13, 'Регламенты и приказы', null, 40, {
      is_hidden_for_anon: true,
      content_html: '<h2>Служебные регламенты</h2><p>Раздел доступен только сотрудникам.</p>',
    }),
    node(14, 'Приказ о пропускном режиме', 13, 10, {
      content_html: '<p>Пропуска оформляются в кабинете 101.</p>',
    }),
  ];
}

function seedSettings(): MockSettings {
  return {
    app_name: 'Сервисный центр',
    deadline_days: 2,
    workdays: ['mon', 'tue', 'wed', 'thu', 'fri', 'sat'],
    timezone: 'Asia/Yekaterinburg',
    deadline_reminder_time: '10:00',
    push_enabled: true,
    statuses: [
      { code: 'new', label: 'Новая', color: '#1677ff' },
      { code: 'in_progress', label: 'В работе', color: '#fa8c16' },
      { code: 'deferred', label: 'Отложена', color: '#8c8c8c' },
      { code: 'done', label: 'Завершена', color: '#52c41a' },
    ],
    updated_at: isoDaysAgo(5),
  };
}

function seedFiles(): MockFile[] {
  return [
    {
      id: 1,
      filename: 'forma-zayavki.svg',
      content_type: 'image/svg+xml',
      size_bytes: 4_096,
      media_kind: 'image',
      created_at: isoDaysAgo(30),
      scope: 'kb',
      ticket_id: null,
      token: null,
    },
    {
      id: 2,
      filename: 'peremeshchenie.svg',
      content_type: 'image/svg+xml',
      size_bytes: 5_120,
      media_kind: 'image',
      created_at: isoDaysAgo(20),
      scope: 'kb',
      ticket_id: null,
      token: null,
    },
    {
      id: 3,
      filename: 'protechka-foto.svg',
      content_type: 'image/svg+xml',
      size_bytes: 3_072,
      media_kind: 'image',
      created_at: isoDaysAgo(3),
      scope: 'ticket',
      ticket_id: 2,
      token: null,
    },
    {
      id: 4,
      filename: 'akt-osmotra.txt',
      content_type: 'text/plain',
      size_bytes: 512,
      media_kind: 'document',
      created_at: isoDaysAgo(3),
      scope: 'ticket',
      ticket_id: 2,
      token: null,
    },
  ];
}

function seedEquipment(): { warehouses: MockWarehouse[]; equipment: MockEquipment[] } {
  const warehouses: MockWarehouse[] = [
    { id: 1, building_id: 1, name: 'Кабинет 101', created_at: isoDaysAgo(300) },
    { id: 2, building_id: 1, name: 'Склад ИТ', created_at: isoDaysAgo(300) },
    { id: 3, building_id: 2, name: 'Цех А', created_at: isoDaysAgo(280) },
    { id: 4, building_id: 3, name: 'Основной склад', created_at: isoDaysAgo(270) },
    // EQ-1/EQ-3: у деактивированного здания (id 4) остаётся склад — он нужен
    // в истории/фильтрах, а перемещение в него доступно только ADMIN.
    { id: 5, building_id: 4, name: 'Архив', created_at: isoDaysAgo(600) },
  ];

  const equipment: MockEquipment[] = [
    {
      id: 1,
      name: 'Ноутбук Dell Latitude',
      equipment_type_id: 1,
      serial_number: 'DL-4410-01',
      registration_number: 'INV-1001',
      warehouse_id: 2,
      created_at: isoDaysAgo(200),
      deleted_at: null,
    },
    {
      id: 2,
      name: 'Монитор LG 24"',
      equipment_type_id: 4,
      serial_number: 'LG-24-771',
      registration_number: 'INV-1002',
      warehouse_id: 2,
      created_at: isoDaysAgo(190),
      deleted_at: null,
    },
    {
      id: 3,
      name: 'Принтер HP LaserJet',
      equipment_type_id: 2,
      serial_number: 'HP-9921',
      registration_number: 'INV-1003',
      warehouse_id: 1,
      created_at: isoDaysAgo(180),
      deleted_at: null,
    },
    {
      id: 4,
      name: 'Стол письменный',
      equipment_type_id: 3,
      serial_number: 'TBL-0021',
      registration_number: 'INV-1004',
      warehouse_id: 1,
      created_at: isoDaysAgo(170),
      deleted_at: null,
    },
    {
      id: 5,
      name: 'Компьютер сборный',
      equipment_type_id: 1,
      serial_number: 'PC-A-118',
      registration_number: 'INV-1005',
      warehouse_id: 3,
      created_at: isoDaysAgo(160),
      deleted_at: null,
    },
    {
      id: 6,
      name: 'Принтер Canon',
      equipment_type_id: 2,
      serial_number: 'CN-5512',
      registration_number: 'INV-1006',
      warehouse_id: 3,
      created_at: isoDaysAgo(150),
      deleted_at: null,
    },
    {
      id: 7,
      name: 'Стол складской',
      equipment_type_id: 3,
      serial_number: 'TBL-0099',
      registration_number: 'INV-1007',
      warehouse_id: 4,
      created_at: isoDaysAgo(140),
      deleted_at: null,
    },
    {
      id: 8,
      name: 'Монитор Samsung 27"',
      equipment_type_id: 4,
      serial_number: 'SM-27-441',
      registration_number: 'INV-1008',
      warehouse_id: 4,
      created_at: isoDaysAgo(130),
      deleted_at: null,
    },
    // EQ-1: единица в деактивированном здании — остаётся в списках и истории,
    // бейдж «Неактивно» и запрет перемещения внутрь для USER проверяются на ней.
    {
      id: 9,
      name: 'Сервер HP ProLiant',
      equipment_type_id: 1,
      serial_number: 'HP-SRV-01',
      registration_number: 'INV-1009',
      warehouse_id: 5,
      created_at: isoDaysAgo(500),
      deleted_at: null,
    },
  ];

  return { warehouses, equipment };
}

function seedMovements(): MockMovement[] {
  return [
    {
      id: 1,
      equipment_id: 1,
      from_warehouse_id: 1,
      to_warehouse_id: 2,
      moved_by_user_id: 2,
      moved_at: isoDaysAgo(15, 12),
      comment: 'Перенос на склад ИТ',
    },
    {
      id: 2,
      equipment_id: 5,
      from_warehouse_id: 2,
      to_warehouse_id: 3,
      moved_by_user_id: 1,
      moved_at: isoDaysAgo(10, 14),
      comment: null,
    },
    // Журнал хранит перемещение в деактивированное здание (EQ-5/EQ-6).
    {
      id: 3,
      equipment_id: 9,
      from_warehouse_id: 2,
      to_warehouse_id: 5,
      moved_by_user_id: 1,
      moved_at: isoDaysAgo(60, 10),
      comment: 'Выведен из эксплуатации, перенесён в архив',
    },
  ];
}

export function createSeed(): MockDb {
  const { warehouses, equipment } = seedEquipment();
  return {
    buildings: seedBuildings(),
    users: seedUsers(),
    problemTypes: [
      { id: 1, name: 'Освещение' },
      { id: 2, name: 'Протечка / сантехника' },
      { id: 3, name: 'Неисправность оборудования' },
      { id: 4, name: 'Уборка' },
      { id: 5, name: 'Прочее' },
    ],
    equipmentTypes: [
      { id: 1, name: 'Компьютер' },
      { id: 2, name: 'Принтер' },
      { id: 3, name: 'Мебель' },
      { id: 4, name: 'Монитор' },
    ],
    warehouses,
    equipment,
    movements: seedMovements(),
    tickets: seedTickets(),
    comments: [
      {
        id: 1,
        ticket_id: 2,
        author_name: 'Иванов Иван Иванович',
        author_type: 'user',
        body: 'Выехал на место, перекрыл воду. Нужна замена сифона.',
        created_at: isoDaysAgo(2, 11),
        file_ids: [3],
      },
      {
        id: 2,
        ticket_id: 2,
        author_name: 'Сидоров Пётр Алексеевич',
        author_type: 'user',
        body: 'Спасибо, жду мастера после 15:00.',
        created_at: isoDaysAgo(1, 15),
        file_ids: [4],
      },
      {
        id: 3,
        ticket_id: 4,
        author_name: 'Иванов Иван Иванович',
        author_type: 'user',
        body: 'Уборка выполнена, заявка закрыта.',
        created_at: isoDaysAgo(7, 17),
        file_ids: [],
      },
    ],
    files: seedFiles(),
    kbNodes: seedKbNodes(),
    holidays: [
      { id: 1, date: `${new Date().getFullYear()}-01-01`, name: 'Новый год' },
      { id: 2, date: `${new Date().getFullYear()}-01-07`, name: 'Рождество' },
      { id: 3, date: `${new Date().getFullYear()}-02-23`, name: 'День защитника Отечества' },
      { id: 4, date: `${new Date().getFullYear()}-03-08`, name: 'Международный женский день' },
      { id: 5, date: `${new Date().getFullYear()}-05-01`, name: 'Праздник весны и труда' },
    ],
    settings: seedSettings(),
    trash: [
      { type: 'ticket', id: 13, deleted_at: isoDaysAgo(3), deleted_by_user_id: 1 },
      { type: 'kb', id: 12, deleted_at: isoDaysAgo(4), deleted_by_user_id: 1 },
    ],
    pushSubscriptions: [
      {
        id: 1,
        user_id: 2,
        endpoint: 'https://push.example.test/subscription/1',
        keys: { p256dh: 'mock-p256dh', auth: 'mock-auth' },
        user_agent: 'Mozilla/5.0 (mock)',
        created_at: isoDaysAgo(7),
        last_success_at: isoDaysAgo(1),
        is_active: true,
      },
    ],
  };
}

/** Живая база: пересоздаётся между тестами и при перезагрузке страницы. */
export let db: MockDb = createSeed();

export function resetDb(): MockDb {
  db = createSeed();
  return db;
}

// ---------------------------------------------------------------------------
// Мапперы «внутренняя модель → ответ API»
// ---------------------------------------------------------------------------

export function nextId(items: Array<{ id: number }>): number {
  return items.reduce((max, item) => Math.max(max, item.id), 0) + 1;
}

export function findBuilding(id: number | null): MockBuilding | undefined {
  return id === null ? undefined : db.buildings.find((building) => building.id === id);
}

export function findUser(id: number | null): MockUser | undefined {
  return id === null ? undefined : db.users.find((user) => user.id === id);
}

export function findWarehouse(id: number): MockWarehouse | undefined {
  return db.warehouses.find((warehouse) => warehouse.id === id);
}

export function findTicket(id: number): MockTicket | undefined {
  return db.tickets.find((ticket) => ticket.id === id && ticket.deleted_at === null);
}

export function statusMeta(code: TicketStatus): { label: string; color: string } {
  const status = db.settings.statuses.find((item) => item.code === code);
  return status ? { label: status.label, color: status.color } : { label: code, color: '#8c8c8c' };
}

export function fileMeta(file: MockFile): FileMeta {
  return {
    id: file.id,
    filename: file.filename,
    content_type: file.content_type,
    size_bytes: file.size_bytes,
    media_kind: file.media_kind,
    url: file.token
      ? `/api/public/files/${file.id}?token=${encodeURIComponent(file.token)}`
      : `/api/files/${file.id}`,
    created_at: file.created_at,
  };
}

export function filesByIds(ids: number[]): FileMeta[] {
  return ids
    .map((id) => db.files.find((file) => file.id === id))
    .filter((file): file is MockFile => Boolean(file))
    .map(fileMeta);
}

export function ticketComments(ticketId: number): TicketComment[] {
  return db.comments
    .filter((comment) => comment.ticket_id === ticketId)
    .sort((a, b) => a.created_at.localeCompare(b.created_at))
    .map((comment) => ({
      id: comment.id,
      ticket_id: comment.ticket_id,
      author_name: comment.author_name,
      author_type: comment.author_type,
      body: comment.body,
      created_at: comment.created_at,
      files: filesByIds(comment.file_ids),
    }));
}

/** Публичная карточка дашборда (TICKET-5): без description и вложений. */
export function toPublicCard(ticket: MockTicket): PublicTicketCard {
  const meta = statusMeta(ticket.status);
  const building = findBuilding(ticket.building_id);
  const problemType = db.problemTypes.find((type) => type.id === ticket.problem_type_id);
  const executor = findUser(ticket.executor_id);
  return {
    id: ticket.id,
    building_name: building?.name ?? '—',
    applicant_name: ticket.applicant_name,
    cabinet: ticket.cabinet,
    problem_type_name: problemType?.name ?? '—',
    status: ticket.status,
    status_label: meta.label,
    status_color: meta.color,
    deadline: ticket.deadline,
    executor_name: executor?.full_name ?? null,
    created_at: ticket.created_at,
  };
}

export function toPublicTicket(ticket: MockTicket): PublicTicket {
  const meta = statusMeta(ticket.status);
  const building = findBuilding(ticket.building_id);
  const problemType = db.problemTypes.find((type) => type.id === ticket.problem_type_id);
  const executor = findUser(ticket.executor_id);
  return {
    id: ticket.id,
    building_id: ticket.building_id,
    building_name: building?.name ?? '—',
    applicant_name: ticket.applicant_name,
    cabinet: ticket.cabinet,
    problem_type_name: problemType?.name ?? '—',
    status: ticket.status,
    status_label: meta.label,
    status_color: meta.color,
    deadline: ticket.deadline,
    is_overdue: isOverdue(ticket),
    executor_name: executor?.full_name ?? null,
    description: ticket.description,
    created_at: ticket.created_at,
    completed_at: ticket.completed_at,
    comments: ticketComments(ticket.id),
    files: db.files
      .filter((file) => file.ticket_id === ticket.id && file.scope === 'ticket')
      .map(fileMeta),
  };
}

export function toTicket(ticket: MockTicket): Ticket {
  const meta = statusMeta(ticket.status);
  const building = findBuilding(ticket.building_id);
  const problemType = db.problemTypes.find((type) => type.id === ticket.problem_type_id);
  const executor = findUser(ticket.executor_id);
  const comments = db.comments.filter((comment) => comment.ticket_id === ticket.id);
  return {
    id: ticket.id,
    building_id: ticket.building_id,
    building_name: building?.name ?? '—',
    applicant_name: ticket.applicant_name,
    cabinet: ticket.cabinet,
    problem_type_id: ticket.problem_type_id,
    problem_type_name: problemType?.name ?? '—',
    description: ticket.description,
    deadline: ticket.deadline,
    is_overdue: isOverdue(ticket),
    executor_id: ticket.executor_id,
    executor_name: executor?.full_name ?? null,
    executor_role: executor?.role ?? null,
    status: ticket.status,
    status_label: meta.label,
    status_color: meta.color,
    completed_at: ticket.completed_at,
    created_at: ticket.created_at,
    updated_at: ticket.updated_at,
    is_anonymous: ticket.is_anonymous,
    created_by_user_id: ticket.created_by_user_id,
    comments_count: comments.length,
    files: db.files
      .filter((file) => file.ticket_id === ticket.id && file.scope === 'ticket')
      .map(fileMeta),
  };
}

export function buildingCounts(buildingId: number): BuildingCounts {
  const active = db.tickets.filter(
    (ticket) => ticket.building_id === buildingId && ticket.deleted_at === null,
  );
  return {
    new: active.filter((ticket) => ticket.status === 'new').length,
    in_progress: active.filter((ticket) => ticket.status === 'in_progress').length,
    deferred: active.filter((ticket) => ticket.status === 'deferred').length,
  };
}

export function toPublicBuilding(building: MockBuilding): PublicBuilding {
  return {
    id: building.id,
    name: building.name,
    address: building.address,
    counts: buildingCounts(building.id),
  };
}

export function toBuilding(building: MockBuilding): Building {
  return {
    id: building.id,
    name: building.name,
    address: building.address,
    is_active: building.is_active,
    created_at: building.created_at,
    counts: buildingCounts(building.id),
  };
}

export function toWarehouse(warehouse: MockWarehouse): Warehouse {
  return {
    id: warehouse.id,
    building_id: warehouse.building_id,
    building_name: findBuilding(warehouse.building_id)?.name ?? '—',
    name: warehouse.name,
    created_at: warehouse.created_at,
  };
}

export function toEquipment(item: MockEquipment): Equipment {
  const warehouse = findWarehouse(item.warehouse_id);
  const building = findBuilding(warehouse?.building_id ?? null);
  const type = db.equipmentTypes.find(
    (equipmentType) => equipmentType.id === item.equipment_type_id,
  );
  return {
    id: item.id,
    name: item.name,
    equipment_type_id: item.equipment_type_id,
    equipment_type_name: type?.name ?? '—',
    serial_number: item.serial_number,
    registration_number: item.registration_number,
    warehouse_id: item.warehouse_id,
    warehouse_name: warehouse?.name ?? '—',
    building_id: building?.id ?? 0,
    building_name: building?.name ?? '—',
    is_building_active: building?.is_active ?? false,
    created_at: item.created_at,
  };
}

export function toMovement(movement: MockMovement): EquipmentMovement {
  const equipment = db.equipment.find((item) => item.id === movement.equipment_id);
  const from = findWarehouse(movement.from_warehouse_id);
  const to = findWarehouse(movement.to_warehouse_id);
  const user = findUser(movement.moved_by_user_id);
  return {
    id: movement.id,
    equipment_id: movement.equipment_id,
    equipment_name: equipment?.name ?? '—',
    from_warehouse_id: movement.from_warehouse_id,
    from_warehouse_name: from?.name ?? '—',
    from_building_name: findBuilding(from?.building_id ?? null)?.name ?? '—',
    to_warehouse_id: movement.to_warehouse_id,
    to_warehouse_name: to?.name ?? '—',
    to_building_name: findBuilding(to?.building_id ?? null)?.name ?? '—',
    moved_by_user_id: movement.moved_by_user_id,
    moved_by_name: user?.full_name ?? '—',
    moved_at: movement.moved_at,
    comment: movement.comment,
  };
}

export function toAdminUser(user: MockUser): AdminUser {
  return {
    id: user.id,
    login: user.login,
    full_name: user.full_name,
    role: user.role,
    building_id: user.building_id,
    building_name: findBuilding(user.building_id)?.name ?? null,
    must_change_password: user.must_change_password,
    is_active: user.is_active,
    created_at: user.created_at,
    last_login_at: user.last_login_at,
  };
}

export function toAssignableUser(user: MockUser): AssignableUser {
  return {
    id: user.id,
    login: user.login,
    full_name: user.full_name,
    role: user.role,
    building_id: user.building_id,
    building_name: findBuilding(user.building_id)?.name ?? null,
  };
}

/** Видимость узла с учётом наследования от корня (KB-1, KB-7). */
export function isKbNodeHidden(node: MockKbNode): boolean {
  let current: MockKbNode | undefined = node;
  const visited = new Set<number>();
  while (current) {
    if (visited.has(current.id)) break;
    visited.add(current.id);
    if (current.is_hidden_for_anon) return true;
    current =
      current.parent_id === null
        ? undefined
        : db.kbNodes.find((item) => item.id === current!.parent_id);
  }
  return false;
}

export function toKbNode(node: MockKbNode): KbNode {
  return {
    id: node.id,
    title: node.title,
    content_html: node.content_html,
    parent_id: node.parent_id,
    sort_order: node.sort_order,
    is_hidden_for_anon: node.is_hidden_for_anon,
    is_hidden_effective: isKbNodeHidden(node),
    created_at: node.created_at,
    updated_at: node.updated_at,
    updated_by_name: node.updated_by_name,
    media: filesByIds(node.media_ids),
  };
}

export function toKbTree(visible?: (node: MockKbNode) => boolean): KbTreeNode[] {
  const nodes = db.kbNodes.filter((node) => (visible ? visible(node) : true));
  const build = (parentId: number | null): KbTreeNode[] =>
    nodes
      .filter((node) => node.parent_id === parentId)
      .sort((a, b) => a.sort_order - b.sort_order || a.title.localeCompare(b.title))
      .map((node) => {
        const children = build(node.id);
        return {
          id: node.id,
          title: node.title,
          parent_id: node.parent_id,
          sort_order: node.sort_order,
          is_hidden_for_anon: node.is_hidden_for_anon,
          is_hidden_effective: isKbNodeHidden(node),
          has_children: children.length > 0,
          children,
        };
      });
  return build(null);
}

export function kbDeletePreview(id: number): KbDeleteResponse {
  const collect = (parentId: number, acc: number[] = []): number[] => {
    const children = db.kbNodes.filter((node) => node.parent_id === parentId);
    children.forEach((child) => {
      acc.push(child.id);
      collect(child.id, acc);
    });
    return acc;
  };
  return { deleted_id: id, descendants_deleted: collect(id).length };
}

export function toTrashItem(item: MockTrashItem): TrashItem {
  const deletedBy = findUser(item.deleted_by_user_id);
  if (item.type === 'ticket') {
    const ticket = db.tickets.find((entry) => entry.id === item.id);
    return {
      id: item.id,
      type: item.type,
      title: `Заявка №${item.id}`,
      description: ticket
        ? `${ticket.applicant_name} · ${findBuilding(ticket.building_id)?.name ?? '—'}`
        : null,
      deleted_at: item.deleted_at,
      deleted_by_name: deletedBy?.full_name ?? '—',
      warnings:
        ticket && findBuilding(ticket.building_id)?.is_active === false
          ? ['Здание деактивировано: после восстановления заявка будет недоступна публично']
          : [],
    };
  }
  if (item.type === 'kb') {
    const node = db.kbNodes.find((entry) => entry.id === item.id);
    return {
      id: item.id,
      type: item.type,
      title: node?.title ?? `Узел №${item.id}`,
      description: node ? 'Раздел базы знаний' : null,
      deleted_at: item.deleted_at,
      deleted_by_name: deletedBy?.full_name ?? '—',
      warnings: ['Потомки узла не восстанавливаются'],
    };
  }
  const equipment = db.equipment.find((entry) => entry.id === item.id);
  return {
    id: item.id,
    type: item.type,
    title: equipment?.name ?? `Оборудование №${item.id}`,
    description: equipment?.registration_number ?? null,
    deleted_at: item.deleted_at,
    deleted_by_name: deletedBy?.full_name ?? '—',
    warnings: [],
  };
}

/** Статистика по заявкам: те же метрики, что в STATS-1/STATS-2. */
export function computeStats(
  userId: number | null,
  buildingId: number | null,
  from?: string,
  to?: string,
): StatsResponse {
  const scope = db.tickets.filter((ticket) => {
    if (ticket.deleted_at) return false;
    if (!ticket.completed_at) return false;
    if (userId !== null && ticket.executor_id !== userId) return false;
    if (buildingId !== null && ticket.building_id !== buildingId) return false;
    const completedDate = ticket.completed_at.slice(0, 10);
    if (from && completedDate < from) return false;
    if (to && completedDate > to) return false;
    return true;
  });

  const user = userId !== null ? findUser(userId) : undefined;
  const durations = scope
    .map((ticket) => {
      const duration =
        (new Date(ticket.completed_at!).getTime() - new Date(ticket.created_at).getTime()) /
        3_600_000;
      return Math.round(duration * 10) / 10;
    })
    .sort((a, b) => a - b);

  const avg = durations.length
    ? Math.round((durations.reduce((sum, value) => sum + value, 0) / durations.length) * 10) / 10
    : null;
  const median = durations.length
    ? durations.length % 2 === 1
      ? durations[(durations.length - 1) / 2]!
      : Math.round(
          ((durations[durations.length / 2 - 1]! + durations[durations.length / 2]!) / 2) * 10,
        ) / 10
    : null;

  const onTime = scope.filter(
    (ticket) => ticket.deadline !== null && ticket.completed_at!.slice(0, 10) <= ticket.deadline,
  ).length;

  const byProblemType: ProblemTypeStat[] = db.problemTypes
    .map((type) => {
      const items = scope.filter((ticket) => ticket.problem_type_id === type.id);
      return {
        problem_type_id: type.id,
        problem_type_name: type.name,
        total: items.length,
        done: items.length,
      };
    })
    .filter((stat) => stat.total > 0);

  const monthMap = new Map<string, number>();
  scope.forEach((ticket) => {
    const month = ticket.completed_at!.slice(0, 7);
    monthMap.set(month, (monthMap.get(month) ?? 0) + 1);
  });
  const byMonth: MonthStat[] = [...monthMap.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([month, done]) => ({ month, done }));

  const overdueCount = scope.filter(
    (ticket) => ticket.deadline !== null && ticket.completed_at!.slice(0, 10) > ticket.deadline,
  ).length;

  return {
    user_id: userId ?? 0,
    user_name: user?.full_name ?? 'Все сотрудники',
    from: from ?? '',
    to: to ?? '',
    total_done: db.tickets.filter(
      (ticket) =>
        !ticket.deleted_at &&
        ticket.completed_at !== null &&
        (userId === null || ticket.executor_id === userId) &&
        (buildingId === null || ticket.building_id === buildingId),
    ).length,
    done_in_period: scope.length,
    avg_resolution_hours: avg,
    median_resolution_hours: median,
    on_time_percent: scope.length ? Math.round((onTime / scope.length) * 1000) / 10 : null,
    overdue_count: overdueCount,
    by_problem_type: byProblemType,
    by_month: byMonth,
  };
}

export function toTicketListParams(url: URL): TicketListParams {
  const status = url.searchParams.getAll('status') as TicketStatus[];
  const numeric = (key: string): number | undefined => {
    const value = url.searchParams.get(key);
    const parsed = value === null ? Number.NaN : Number(value);
    return Number.isSafeInteger(parsed) ? parsed : undefined;
  };
  // «Без исполнителя»: клиент передаёт сентинел `none` (TICKET-15).
  const executorRaw = url.searchParams.get('executor_id');
  const executorId =
    executorRaw === 'none' ? null : executorRaw === null ? undefined : numeric('executor_id');

  return {
    page: numeric('page'),
    size: numeric('size'),
    status: status.length ? status : undefined,
    executor_id: executorId,
    problem_type_id: numeric('problem_type_id'),
    building_id: numeric('building_id'),
    date_from: url.searchParams.get('date_from') ?? undefined,
    date_to: url.searchParams.get('date_to') ?? undefined,
    search: url.searchParams.get('search') ?? undefined,
    sort_by: (url.searchParams.get('sort_by') as TicketListParams['sort_by']) ?? undefined,
    sort_dir: (url.searchParams.get('sort_dir') as TicketListParams['sort_dir']) ?? undefined,
  };
}

export function filterTickets(
  params: TicketListParams,
  options: { userBuildingId?: number | null } = {},
): { items: MockTicket[]; total: number } {
  const page = params.page && params.page > 0 ? params.page : 1;
  const size = params.size && params.size > 0 ? Math.min(params.size, 100) : 20;

  let items = db.tickets.filter((ticket) => ticket.deleted_at === null);

  // TICKET-7: USER видит только своё здание, фильтр по зданию игнорируется сервером.
  if (options.userBuildingId !== undefined && options.userBuildingId !== null) {
    items = items.filter((ticket) => ticket.building_id === options.userBuildingId);
  } else if (typeof params.building_id === 'number') {
    items = items.filter((ticket) => ticket.building_id === params.building_id);
  }

  if (params.status?.length)
    items = items.filter((ticket) => params.status!.includes(ticket.status));
  if (params.executor_id === null) {
    // «Без исполнителя» (TICKET-8: ADMIN снимает исполнителя).
    items = items.filter((ticket) => ticket.executor_id === null);
  } else if (params.executor_id !== undefined) {
    items = items.filter((ticket) => ticket.executor_id === params.executor_id);
  }
  if (params.problem_type_id !== undefined) {
    items = items.filter((ticket) => ticket.problem_type_id === params.problem_type_id);
  }
  if (params.date_from)
    items = items.filter((ticket) => ticket.created_at.slice(0, 10) >= params.date_from!);
  if (params.date_to)
    items = items.filter((ticket) => ticket.created_at.slice(0, 10) <= params.date_to!);
  if (params.search) {
    const search = params.search.toLowerCase();
    items = items.filter(
      (ticket) =>
        ticket.applicant_name.toLowerCase().includes(search) ||
        ticket.cabinet.toLowerCase().includes(search),
    );
  }

  const sortBy = params.sort_by ?? 'created_at';
  const direction = params.sort_dir === 'asc' ? 1 : -1;
  items = [...items].sort((a, b) => {
    const left = sortBy === 'deadline' ? (a.deadline ?? '') : a.created_at;
    const right = sortBy === 'deadline' ? (b.deadline ?? '') : b.created_at;
    return left.localeCompare(right) * direction;
  });

  const total = items.length;
  const start = (page - 1) * size;
  return { items: items.slice(start, start + size), total };
}

export function filterEquipment(params: EquipmentListParams): {
  items: MockEquipment[];
  total: number;
} {
  const page = params.page && params.page > 0 ? params.page : 1;
  const size = params.size && params.size > 0 ? Math.min(params.size, 100) : 20;

  let items = db.equipment.filter((item) => item.deleted_at === null);
  if (params.building_id) {
    items = items.filter((item) => {
      const warehouse = findWarehouse(item.warehouse_id);
      return warehouse?.building_id === params.building_id;
    });
  }
  if (params.warehouse_id)
    items = items.filter((item) => item.warehouse_id === params.warehouse_id);
  if (params.equipment_type_id) {
    items = items.filter((item) => item.equipment_type_id === params.equipment_type_id);
  }
  if (params.search) {
    const search = params.search.toLowerCase();
    items = items.filter(
      (item) =>
        item.name.toLowerCase().includes(search) ||
        item.serial_number.toLowerCase().includes(search) ||
        item.registration_number.toLowerCase().includes(search),
    );
  }
  const total = items.length;
  const start = (page - 1) * size;
  return { items: items.slice(start, start + size), total };
}

export function equipmentBulkMoveResult(
  movedCount: number,
  warehouseId: number,
): EquipmentBulkMoveResponse {
  return {
    moved_count: movedCount,
    to_warehouse_id: warehouseId,
    to_warehouse_name: findWarehouse(warehouseId)?.name ?? '—',
    movements_created: movedCount,
  };
}
