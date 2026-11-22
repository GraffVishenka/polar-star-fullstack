# Frontend — «Заявки · База знаний · Оборудование»

SPA на **React 19 + TypeScript (strict) + Vite**, UI — **Ant Design 5** (русская локаль),
роутинг — **React Router 7**, данные — **TanStack Query 5**, формы — **react-hook-form + zod**.
Бэкенда для разработки не требуется: все эндпоинты контракта (§3.13 PLAN.md) покрыты **MSW**.

## Быстрый старт

```powershell
# из корня репозитория
. .\scripts\dev-env.ps1        # pnpm + node в PATH (Windows-песочница)
cd frontend
pnpm install
pnpm dev                       # http://localhost:5173, MSW включён
```

Демо-доступ в MSW-режиме: `admin / admin12345` (ADMIN), `user / user12345`
(сотрудник «Центрального офиса»), `bez-zdaniya / user12345` (USER без здания — баннер AUTH-9),
`novichok / temp12345` (временный пароль → обязательная смена). Название приложения и
названия/цвета статусов меняются в `/admin/settings` и сразу видны на публичном
дашборде (`GET /api/public/settings`, с фолбэком на `app/config`).

Работа с реальным API:

```powershell
$env:VITE_ENABLE_MSW='false'   # vite proxy /api → http://localhost:8080
pnpm dev
```

## Скрипты

| Команда                        | Что делает                                  |
| ------------------------------ | ------------------------------------------- |
| `pnpm dev`                     | Vite dev-сервер, MSW по умолчанию           |
| `pnpm build`                   | `tsc -b` + `vite build` → `dist/`           |
| `pnpm preview`                 | статический предпросмотр собранного `dist/` |
| `pnpm typecheck`               | `tsc -b` (strict, без emit)                 |
| `pnpm lint`                    | ESLint 9 flat config, `--max-warnings=0`    |
| `pnpm test`                    | Vitest в watch-режиме                       |
| `pnpm test:run`                | один прогон (используется в CI)             |
| `pnpm test:coverage`           | покрытие v8 → `coverage/`                   |
| `pnpm format` / `format:check` | Prettier                                    |

## Структура

```
src/
  app/            каркас: providers (antd+query+auth), theme, router, routes/, config, toast
  api/            fetch-клиент (Bearer, single-flight refresh, retry), errors, types, домены
  auth/           AuthContext, guards (RequireAuth/RequireRole/RequireAnonOnly)
  components/     общие компоненты Ф1 (StatusTag, DeadlineBadge, FileUploader, ...)
  hooks/          useAnonBuilding (cookie выбранного здания)
  layouts/        PublicLayout, AppLayout, AdminLayout
  pages/          public/, tickets/, auth/, placeholders/, NotFoundPage
  mocks/          MSW: db.ts (сид), handlers/* (по доменам), browser.ts, server.ts
  test/           setup.ts (jest-dom, MSW, полифилы), utils.tsx (провайдеры, фабрики)
```

## API-клиент

- `baseURL` — `import.meta.env.VITE_API_BASE_URL` (по умолчанию `/api`).
- Access-токен **только в памяти** (`setAccessToken`), refresh — httpOnly cookie.
- При `401` — **single-flight refresh** и один повтор исходного запроса; при неудаче
  токен очищается и срабатывают подписчики `onAuthLost` (AuthProvider разлогинивает).
- Ошибки — `ApiError` с `code/status/details/requestId`, русский текст из `ERROR_MESSAGES`
  (маппинг кодов PLAN.md). Для 5xx/429/сети автоматически показывается тост с `request_id`
  (регистрируется в `app/ToastBridge.tsx`).
- Скачивание файлов — `apiDownload` (Word/Excel, имя из `Content-Disposition`).
- Загрузка файлов — `uploadFile` (XHR ради прогресса, лимиты 10 МБ фото / 200 МБ видео).

Типы: `src/api/types.ts` описаны **вручную строго по §3.13** с пометкой
`// TODO: генерировать из openapi/openapi.yaml после Б1`. После публикации контракта файл
перегенерируется, ручные правки запрещены.

## Права и маршруты

| Маршрут                      | Доступ                     | Состояние Ф1                                                                        |
| ---------------------------- | -------------------------- | ----------------------------------------------------------------------------------- |
| `/`                          | аноним (`RequireAnonOnly`) | выбор здания с реальными счётчиками                                                 |
| `/b/:buildingId`, `/b/all`   | все                        | публичная доска здания / сводка по всем зданиям                                     |
| `/t/:token`                  | все (по токену)            | карточка по токену: поля, комментарии, Word                                         |
| `/public/kb`                 | все                        | публичная БЗ: дерево без скрытых разделов                                           |
| `/login`, `/change-password` | аноним / авторизованный    | формы на antd `Form` с правилами                                                    |
| `/tickets`                   | авторизованные             | доска 4 колонки, фильтры, карточка, комментарии (Ф2)                                |
| `/kb`                        | авторизованные             | дерево БЗ, статья и редактор (Ф3)                                                   |
| `/equipment`                 | авторизованные             | здания → склады → оборудование, карточка, перемещения (Ф4)                          |
| `/stats`                     | авторизованные             | личная статистика с Excel-выгрузкой (Ф10)                                           |
| `/profile`                   | авторизованные             | профиль, пароль, push-уведомления (Ф9)                                              |
| `/admin/*`                   | только `ADMIN`             | Ф6 (пользователи/справочники/настройки), Ф7 (здания, БЗ, корзина), Ф10 (статистика) |
| `/403`                       | все                        | страница «доступ запрещён»                                                          |

Заглушки живут в `src/pages/placeholders/PlaceholderPage.tsx` — они **не занимают**
каталоги чужих агентов. Замена — одна строка в `src/app/routes/appRoutes.tsx`.

## Заявки (Ф2)

`/tickets` — доска из 4 колонок (`new`, `in_progress`, `deferred`, `done`) со счётчиками
из серверного `total`. Пагинация каждой колонки — `useInfiniteQuery` («Показать ещё»),
фильтры — в URL, поэтому ссылку можно переслать:

| Параметр URL                     | Значение                                                                  |
| -------------------------------- | ------------------------------------------------------------------------- |
| `status`                         | повторяющийся параметр (`status=new&status=done`)                         |
| `executor_id`                    | `none` — без исполнителя, `me` — только мои, число — конкретный сотрудник |
| `problem_type_id`, `building_id` | справочники; у USER здание принудительно своё (TICKET-7)                  |
| `date_from`, `date_to`           | `YYYY-MM-DD`, фильтр по дате создания                                     |
| `search`                         | заявитель или кабинет                                                     |
| `sort_by`, `sort_dir`            | `created_at`/`deadline`, `asc`/`desc`                                     |

Права в UI повторяют матрицу §3.1 (сервер проверяет то же самое):

- USER видит только своё здание, селект здания заблокирован с подсказкой;
- статус меняют USER своего здания и ADMIN; «Назначить себя» — USER, если он не исполнитель;
- ADMIN: выбор любого USER здания заявки или ADMIN, снятие исполнителя, дедлайн,
  смена здания с предупреждением о сбросе USER-исполнителя (`executor_reset`),
  удаление в корзину с подтверждением;
- поля заявки правит исполнитель или ADMIN (иначе кнопка заблокирована с пояснением);
- просрочка показывается **только** по серверному `is_overdue` (NFR-7);
- баннер AUTH-9 при отсутствии здания у USER.

Смена статуса и добавление комментария обновляют кэш оптимистично; при `403/404/422`
снимок кэша восстанавливается и показывается русский текст ошибки (`useUpdateTicket`,
`TicketComments`). Комментарии — infinite-пагинация по 50, до 5 вложений (фото/видео
через `FileUploader` с прогрессом) и превью в `MediaGallery`.

Структура раздела: `src/pages/tickets/{TicketsPage.tsx,components/,hooks/,__tests__/}`.
MSW-хендлеры заявок — `src/mocks/handlers/tickets.ts` (владелец fe-1).

## Публичный периметр (Ф5)

`/` → `/b/{id}` → `/t/{token}` работает без входа и без бэкенда (MSW):

- **Выбор здания** (`/`, PUB-1): карточки только активных зданий со счётчиками
  `new`/`in_progress`/`deferred`; выбор пишет cookie `anon_building_id`
  (30 дней, `SameSite=Lax`, `path=/`, доступна JS) и ведёт на `/b/{id}`.
  Cookie валидного здания → авторедирект; `?change=1` его отключает; cookie на
  деактивированное/удалённое здание сбрасывается. Авторизованного `RequireAnonOnly`
  уводит на `/tickets`.
- **Доска здания** (`/b/:buildingId`, PUB-2/PUB-4): 3 колонки без `done`, карточки
  только из разрешённых полей (заявитель, кабинет, вид проблемы, статус, дедлайн,
  исполнитель), без описания/комментариев/файлов. Переключатель зданий — в шапке
  `PublicLayout`; `/b/all` — сводка по всем активным зданиям, только чтение:
  кнопка создания неактивна с подсказкой. Неактивное/несуществующее здание → 404.
  Просрочка на публичной доске не считается: `is_overdue` есть только в токен-карточке.
- **Создание заявки** (PUB-5): здание подставлено и изменяемо; валидация
  (applicant 1–120, cabinet 1–32, вид проблемы, описание 1–5000). После успеха —
  экран со ссылкой `/t/{token}` и кнопкой копирования (токен выдаётся один раз),
  cookie переключается на выбранное здание, кнопка «Перейти в дашборд» открывает
  `/b/{id}`.
- **Карточка по токену** (`/t/:token`, PUB-6): поля, дедлайн по серверному
  `is_overdue`, статус, вложения с просмотром/видео, комментарии с серверной
  пагинацией по 50, добавление комментария с до 5 файлами (фото ≤10 МБ,
  видео ≤200 МБ, прогресс) и «Скачать в Word». Чужой/битый токен → 404.
- **Публичная БЗ** (`/public/kb`, PUB-7): дерево без скрытых разделов, статья
  read-only; HTML санитизируется (KB-4, DOMPurify + allowlist), медиа
  переписывается на `/api/public/files/{id}`. Прямой запрос скрытого узла → 404.
- Страницы: `PublicNotFound` (404) и `/403` — `ForbiddenPage`.

Файлы: `src/pages/public/{BuildingSelectPage,BuildingDashboardPage,PublicTicketPage,PublicKbPage}.tsx`,
компоненты в `src/pages/public/components/`, cookie-хук — `src/hooks/useAnonBuilding.ts`,
копирование ссылки — `src/app/clipboard.ts`. MSW — `src/mocks/handlers/public.ts`.

## Документы и экспорт (Ф8)

Единый механизм скачивания Word/Excel: `GET` → blob → файл с именем из
`Content-Disposition`, `attachment`, `Cache-Control` не кэшируем. Внутри —
`apiDownload` (Bearer/`skipAuth`, single-flight refresh, таймаут), `saveBlob`,
тосты с `request_id` для 5xx и русские тексты 403/404/422/таймаута. Ошибка или
пустой ответ не создают файл, повторное нажатие во время загрузки игнорируется.

| Модуль                                        | Для кого | Что делает                                                                                                                                                      |
| --------------------------------------------- | -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ExportButton` (`@/components`)               | все      | Универсальная кнопка, Word-заявки (`ticket_{id}.docx`)                                                                                                          |
| `useFileDownload` (`@/components/exports`)    | все      | Базовый хук: `download/loading/error/filename/canDownload`                                                                                                      |
| `useInventoryExport`, `InventoryExportButton` | **fe-3** | Ведомость помещения `GET /warehouses/{id}/inventory.xlsx`, 422 при >1000 позиций; подключена на `/equipment` (тулбар выбранного склада и карточка оборудования) |
| `useStatsExport`, `StatsExportButton`         | **fe-2** | Excel-статистика `GET /stats/export.xlsx`: `user_id`, `building_id`, `from`, `to`; 403 для чужого отчёта                                                        |

Примеры подключения и контракт ошибок — `frontend/src/components/exports/README.md`.
Word-выгрузка уже стоит в `TicketDrawer` (`/tickets`) и на публичной `/t/{token}`
(`skipAuth`), имя файла приходит из `Content-Disposition` с fallback `ticket_{id}.docx`.

## Статистика (Ф10)

Страницы статистики — `src/pages/stats/`; вход в личную статистику — из `/profile`
(кнопка «Моя статистика») и из пункта «Статистика» в шапке `AppLayout`:

- **`/stats`** (STATS-1/2/4): карточки «Завершено всего», «Завершено за период»,
  «Среднее время», «Медиана», «В срок», «Просрочено»; `null`-метрики — «—»;
  разбивка по видам проблем (таблица с долей) и по месяцам (бар в таблице);
  период «Месяц / Квартал / Год / Период» + RangePicker, дефолт — текущий месяц;
  пустое состояние «Нет завершённых заявок»; Excel-выгрузка своего отчёта.
- **`/admin/stats`** (только ADMIN, `RequireRole`; USER → `/tickets`): фильтры
  «сотрудник» (`/api/admin/users`, поиск по ФИО), «здание» (только активные,
  «Все здания») и период; те же метрики и разбивки; Excel с
  `user_id/building_id/from/to` (`StatsExportButton` из `@/components/exports`).
- **Сводка по сотрудникам**: если `GET /api/admin/stats` (без `user_id`) вернёт
  аддитивное поле `employees[]`, показывается таблица «Сводка по сотрудникам»;
  без поля блок скрыт. Типы синхронизированы с контрактом в `src/api/types.ts`
  (`EmployeeStats`, `StatsResponse.employees?`), в MSW агрегат уже отдаёт `employees`.
- Пункт «Статистика» есть в шапке `AppLayout`; в меню профиля добавлен
  «Моя статистика» → `/stats`.
- Метрики/просрочку/«в срок» считает сервер (NFR-7); клиент только отображает.

MSW: `/stats/me` и `/admin/stats` учитывают `from/to`, `user_id`, `building_id`;
`null` при отсутствии завершённых заявок; для агрегата ADMIN отдаётся `employees`;
обратный период → `422`, чужие данные → `403`.

## Профиль и push-уведомления (Ф9)

`/profile` — учётная запись (ФИО, логин, роль, здание; у USER без здания — баннер
AUTH-9), смена своего пароля (antd `Form` с правилами: текущий, ≥8 символов,
подтверждение, отличие от текущего) и карточка уведомлений. Личная статистика
открывается ссылкой «Моя статистика» → `/stats` (STATS-4).

Push (NOTIF-1..NOTIF-5) работает только для USER/ADMIN и только по HTTPS
(исключение — localhost):

- `public/sw.js` — обработчики `push` (payload `title/body/url/tag/icon`, title ≤60,
  body ≤120, `showNotification` с `tag` для схлопывания) и `notificationclick`
  (фокус открытой вкладки → `navigate` на нужный экран → `clients.openWindow`).
  Обработчика `fetch` нет: worker не кэширует API и не мешает SPA/MSW.
- Регистрация и подписка — `src/push/pushClient.ts`: `GET /api/push/public-key` → VAPID,
  `pushManager.subscribe({userVisibleOnly:true, applicationServerKey})`,
  `POST /api/push/subscriptions`, id подписки в
  `localStorage['polar-star.push.subscriptionId']`, отключение —
  `DELETE /api/push/subscriptions/{id}` + `unsubscribe()`.
- Scope worker'а: в dev с MSW — `/push/` (чтобы не перетирать `mockServiceWorker.js`),
  в production (`VITE_ENABLE_MSW=false`) — `/`, тогда клик по уведомлению фокусирует вкладку.
- Состояния (`src/push/PushProvider.tsx`, общие для шапки и профиля): не поддерживается,
  разрешение не запрошено, разрешено без подписки, подписано, запрещено (инструкция для
  Chrome/Firefox/Safari), ошибка. Разрешение запрашивается только по кнопке (NOTIF-5).
- «Отправить тестовое уведомление» — выключено с пояснением: серверного эндпоинта
  тестовой отправки в контракте нет (B8 отдаёт только ключ и CRUD подписок).
- Тесты: `src/pages/profile/ProfilePage.test.tsx`, `src/push/*.test.*`; браузерное
  окружение (service worker/PushManager/Notification) подменяется через
  `src/push/testing/pushEnv.ts`, логика `public/sw.js` проверяется исполнением файла
  в песочнице с подменённым `self`.

## MSW: что покрыто

Все эндпоинты §3.13. Хендлеры разбиты по доменам — правит только владелец домена:

| Файл                       | Эндпоинты                                                                                                                                                                  |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `handlers/auth.ts`         | `/auth/login                                                                                                                                                               | refresh                       | logout | me  | password`, push-подписки |
| `handlers/public.ts`       | `/public/buildings`, `/public/tickets*`, `/public/problem-types`, `/public/tickets/{token}/comments`, `/public/tickets/{token}/files`, `/public/files/:id`, `/public/kb/*` |
| `handlers/tickets.ts`      | `/tickets*`, комментарии, `export.docx`, `/users/assignable`                                                                                                               |
| `handlers/kb.ts`           | `/kb/tree                                                                                                                                                                  | nodes*`, `move`, `visibility` |
| `handlers/equipment.ts`    | `/buildings*`, `/warehouses*`, `/equipment*`, `inventory.xlsx`                                                                                                             |
| `handlers/admin.ts`        | `/admin/users*`, `/admin/settings`, справочники (`include_inactive`), праздники (`POST/PATCH/DELETE`), `/admin/trash*`, `/admin/stats`                                     |
| `handlers/stats.ts`        | `/stats/me`, `/stats/export.xlsx`                                                                                                                                          |
| `handlers/files.ts`        | `/files` (multipart), `/files/:id`                                                                                                                                         |
| `handlers/dictionaries.ts` | `GET /api/problem-types`, `/api/equipment-types` (`include_inactive` — только ADMIN), `GET /api/public/settings`                                                           |
| `handlers/health.ts`       | `/health`, `/health/ready`                                                                                                                                                 |

Что моделируется «как на сервере»:

- роли и видимость заявок по зданию (USER чужого здания → `404`, TICKET-7);
- `must_change_password` → `428` на всех эндпоинтах кроме `me/password/logout`;
- смена здания заявки ADMIN'ом с `executor_reset: true` (TICKET-9);
- `is_overdue` и `deadline` считаются в моках (клиент их только отображает);
- дерево БЗ глубины 6, скрытый корневой раздел (`/public/kb/*` → `404`), цикл → `409 CYCLE_DETECTED`;
- `SAME_WAREHOUSE`, `DUPLICATE_REGISTRATION_NUMBER`, `WAREHOUSE_NOT_EMPTY`, `BUILDING_IN_USE`,
  `LAST_ADMIN`, `CANNOT_MODIFY_SELF`, `BUILDING_REQUIRED_FOR_USER`, `409/422` в целом;
- корзина: восстановление и безвозвратное удаление, предупреждения (неактивное здание, потомки БЗ);
- push: `GET /push/public-key` (демо-VAPID), подписка устройства по `endpoint` (повторная
  активация без дублей), `DELETE` деактивирует подписку и проверяет владельца;
- публичный периметр: 404 для неактивного здания и чужого токена, `done` не отдаётся в списке,
  токен анонимной заявки, комментарии анонима по токену, Word-blob, санитизация БЗ.

Данные живут в памяти (`src/mocks/db.ts`, `resetDb()` в тестах): перезагрузка страницы
возвращает сид. Сид: 4 здания (одно неактивное), 7 пользователей, 13 заявок (включая
анонимные с токенами `demo-token-1/2/3/11`), дерево БЗ из 14 узлов, 5 складов
(в т.ч. в деактивированном здании), 10 единиц оборудования (одна в корзине),
журнал перемещений, настройки, праздники и пример push-подписки.

## Как добавить свой раздел (fe-2 / fe-3)

1. Страницу — в свой каталог: `src/pages/<домен>/...`. Общие компоненты Ф1 не переписываются:
   если чего-то не хватает, добавьте своё локальное.
2. Маршрут — одна строка в `src/app/routes/appRoutes.tsx` (заменить `PlaceholderPage`).
3. Данные — функции в `src/api/<домен>.ts` (для kb/equipment/admin файлы уже есть),
   ключи кэша — `src/app/queryKeys.ts` (аддитивно).
4. MSW — только свой файл `src/mocks/handlers/<домен>.ts`.
5. Ошибки — не парсить текст, а проверять `error.code` и показывать `error.userMessage`.

## Ограничения Ф1

- `src/api/types.ts` ручной до генерации из `openapi.yaml`.
- MSW-сессия (refresh-cookie) живёт до перезагрузки страницы: в браузере cookie настоящая,
  в моках — модульная переменная. После F5 нужно войти заново.
- `AppLayout` не запрашивает `app_name` из настроек — в шапке сотрудников остаётся
  `VITE_APP_NAME`/константа. Публичная часть берёт `app_name` из
  `GET /api/public/settings` (`usePublicSettings`) с фолбэком на константу.
- Публичный дашборд и карточка заявки — каркас: полная версия в Ф5.
- `public/sw.js` реализован в Ф9 (fe-3): push и клик по уведомлению, без кэширования API —
  см. раздел «Профиль и push-уведомления».
- MSW-видео отдаётся пустым потоком: превью фото работает, реальное видео проверяется на бэкенде.
- `mockServiceWorker.js` попадает в `dist/`, но в production не регистрируется
  (`VITE_ENABLE_MSW=false` в Dockerfile).

## Ограничения Ф2

- **Контракт-гэп:** в §3.13 виды проблем доступны только ADMIN (`/api/admin/problem-types`),
  а форма заявки нужна USER. Клиент использует `GET /api/problem-types` (см.
  `src/api/dictionaries.ts`, мок `handlers/dictionaries.ts`) — нужен аддитивный эндпоинт
  в `openapi.yaml`, согласовать с lead/be. По той же причине названия статусов у USER —
  из `app/config` (в заявках приходят `status_label`/`status_color`, они и показываются).
- `executor_id=none` — клиентский сентинел для «без исполнителя» (TICKET-15 не фиксирует
  кодирование null). Бэкенду нужно поддержать `none` (или согласовать `0`/пустую строку).
- «Показать ещё» в колонке не пишется в URL: в ссылке хранятся только фильтры.
- В jsdom MSW не разбирает `multipart/form-data`, поэтому в тесте комментария загрузчик
  подменён; сам `FileUploader` покрыт отдельными тестами (лимиты, drag&drop, прогресс).
- Ф8 добавит «Скачать в Word» в карточку заявки (кнопка уже есть, использует `ExportButton`).

## Ограничения Ф5

- **Контракт-гэпы**: два из трёх закрыты бэкендом Б5 — `GET /api/public/tickets/{token}/comments`
  (пагинация 50) и `POST /api/public/tickets/{token}/files` (алиас `/api/public/files?token=`)
  уже в `openapi.yaml`, клиент использует их. Третий, `GET /api/public/problem-types`,
  зафиксирован в контракте Б7 (только активные `id/name/sort_order`) — клиент и мок
  приведены к нему.
- Публичная доска не показывает `is_overdue`: TICKET-5 не включает флаг в карточку,
  поэтому просрочка отображается только в токен-карточке (NFR-7 — не считаем на клиенте).
- Медиа статей БЗ в дереве/статье переписывается на `/api/public/files/{id}`
  (в контракте `<img src="/api/files/{id}">` требует авторизации).
- В jsdom MSW не разбирает `multipart/form-data`, поэтому тесты публичного комментария
  подменяют `uploadPublicCommentFile`; сам транспорт и лимиты проверяются в Ф1/Б3.
- Word/Excel-выгрузки в Ф8 переведены на общий `useFileDownload` (см. раздел ниже);
  Ф11: Playwright-сценарии публичного периметра (приватное окно, cookie, Word, 375 px).

## Синхронизация контракта (Б7/Ф11)

- `GET /api/public/settings` (`PublicSettings`: `app_name` + 4 статуса) — единственный
  источник имени приложения и подписей/цветов на публичных экранах: хук
  `src/hooks/usePublicSettings.ts`, один ключ `queryKeys.publicSettings`;
  при ошибке или неполном ответе — фолбэк на `app/config`, запрос один на все компоненты.
- Справочники: `GET /api/problem-types` и `GET /api/equipment-types` отдают активные
  значения (`include_inactive=true` учитывается только ADMIN); публичный
  `GET /api/public/problem-types` — только `id/name/sort_order` и только активные
  (`PublicProblemType`). В типах `ProblemType`/`EquipmentType` есть `sort_order`,
  `is_active` (необязательные для совместимости фикстур) и аддитивный `usage_count`.
- `PATCH /api/admin/holidays/{id}` (обязательная `date`, `409` при дубле); `name`
  в моках и UI — аддитивное поле, контракт `Holiday` его не требует.
- Корзина: добавлены `deleted_by_user_id`, `building_id`, `building_name`;
  UI продолжает показывать `deleted_by_name` и предупреждения восстановления.
- `employees[]` (`EmployeeStats`) — аддитивное поле агрегата `/api/admin/stats`
  (для запроса с `user_id` отсутствует).

## E2E (Playwright, Ф11)

Сквозные сценарии — `frontend/e2e/`, конфиг `frontend/playwright.config.ts`.

```bash
cd frontend
pnpm install
pnpm e2e:install       # один раз: Chromium для Playwright
pnpm e2e               # все сценарии (поднимает dev-сервер с MSW)
pnpm e2e:ui            # интерактивный UI-режим
```

- **Без бэкенда**: `webServer` запускает `vite` с `VITE_ENABLE_MSW=true`, данные
  живут в MSW (`src/mocks/db.ts`) и сбрасываются при перезагрузке страницы.
  Реальный API для e2e не нужен.
- **Проекты**: `chromium` (1440×900, все сценарии) и `mobile-375` (375×812,
  только `e2e/mobile.spec.ts`). `trace` — на первом ретрае, скриншот/видео — при падении.
- **Сценарии**: выбор здания/cookie/авторедирект; анонимная заявка со сменой
  здания и ссылкой `/t/{token}` в чистом контексте; Word-скачивание; USER —
  видимость своего здания, 404 на чужую заявку, «Назначить себя», комментарий с
  файлом, смена статуса; ADMIN — дедлайн, смена здания со сбросом исполнителя,
  корзина/восстановление; БЗ — подраздел, изображение между абзацами, DnD;
  оборудование — перемещение/журнал, массовое перемещение, ведомость;
  настройки/пользователи; статистика + Excel; axe-core (без critical);
  адаптив 375px; консоль без ошибок (`pageerror` всегда валит тест).
- **Отчёт**: `frontend/playwright-report/` (HTML), `frontend/test-results/`
  (трейсы/скриншоты). Оба каталога в `.gitignore`.
- **CI**: job `e2e` в `.github/workflows/ci.yml` — `playwright install --with-deps
chromium`, `pnpm e2e`, отчёт артефактом при падении.
- **Ограничение**: кнопка «Ведомость помещения» на `/equipment` пока заглушка
  (`inventory-stub`, интеграция `InventoryExportButton` — fe-3/lead), поэтому
  ведомость проверяется на эндпоинте `GET /api/warehouses/{id}/inventory.xlsx`
  через fetch в контексте страницы (MSW) с проверкой `Content-Disposition`.
- **MSW и контексты**: база моков живёт в памяти страницы, поэтому токен,
  созданный в одном браузерном контексте, не виден в другом. Сценарий «ссылка
  `/t/{token}` без cookie» очищает cookie и открывает ссылку SPA-переходом в том
  же контексте; полноценная проверка «нового контекста» — в Ф11 на реальном API.
- **Локальный запуск на Windows**: если Application Control блокирует
  скачанный Chromium (`spawn UNKNOWN`), конфиг по умолчанию использует
  установленный Edge (`channel: 'msedge'`). Переопределение: `E2E_CHANNEL=msedge`
  или `E2E_CHANNEL=chromium`; в CI всегда Playwright Chromium. Видео отключено
  (`video: 'off'`) — ffmpeg из Playwright тоже может блокироваться политикой.
- **Размер бандла**: начальная загрузка (index + antd + react + query + css) —
  ≈546 КБ gzip, лимит 600 КБ. Тяжёлые разделы (`KbPage` с TipTap, dnd-kit и др.)
  вынесены в ленивые чанки (`React.lazy` в `appRoutes`/`publicRoutes`,
  Suspense-фолбэк в layout'ах), поэтому общий вес всех чанков больше — это нормально.

## Docker

```bash
docker build -t polar-star-web ./frontend
docker run --rm -p 8081:80 polar-star-web
```

`nginx.conf`: SPA fallback, `proxy /api` → `api:8080`, `client_max_body_size 210m`
(видео до 200 МБ), `/health` и `/health/ready` проксируются, кэширование хешированных ассетов.
