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
`novichok / temp12345` (временный пароль → обязательная смена).

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

| Маршрут                                   | Доступ                     | Состояние Ф1                                         |
| ----------------------------------------- | -------------------------- | ---------------------------------------------------- |
| `/`                                       | аноним (`RequireAnonOnly`) | выбор здания с реальными счётчиками                  |
| `/b/:buildingId`, `/b/all`                | все                        | публичная доска здания / сводка по всем зданиям      |
| `/t/:token`                               | все (по токену)            | карточка по токену: поля, комментарии, Word          |
| `/public/kb`                              | все                        | публичная БЗ: дерево без скрытых разделов            |
| `/login`, `/change-password`              | аноним / авторизованный    | реальные формы (RHF + zod)                           |
| `/tickets`                                | авторизованные             | доска 4 колонки, фильтры, карточка, комментарии (Ф2) |
| `/kb`, `/equipment`, `/stats`, `/profile` | авторизованные             | заглушки Ф3/Ф4/Ф10                                   |
| `/admin/*`                                | только `ADMIN`             | заглушки Ф6/Ф7/Ф10                                   |
| `/403`                                    | все                        | страница «доступ запрещён»                           |

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

## MSW: что покрыто

Все эндпоинты §3.13. Хендлеры разбиты по доменам — правит только владелец домена:

| Файл                       | Эндпоинты                                                                                                                                                                  |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `handlers/auth.ts`         | `/auth/login                                                                                                                                                               | refresh                       | logout | me  | password`, push-подписки |
| `handlers/public.ts`       | `/public/buildings`, `/public/tickets*`, `/public/problem-types`, `/public/tickets/{token}/comments`, `/public/tickets/{token}/files`, `/public/files/:id`, `/public/kb/*` |
| `handlers/tickets.ts`      | `/tickets*`, комментарии, `export.docx`, `/users/assignable`                                                                                                               |
| `handlers/kb.ts`           | `/kb/tree                                                                                                                                                                  | nodes*`, `move`, `visibility` |
| `handlers/equipment.ts`    | `/buildings*`, `/warehouses*`, `/equipment*`, `inventory.xlsx`                                                                                                             |
| `handlers/admin.ts`        | `/admin/users*`, `/admin/settings`, справочники, праздники, `/admin/trash*`, `/admin/stats`                                                                                |
| `handlers/stats.ts`        | `/stats/me`, `/stats/export.xlsx`                                                                                                                                          |
| `handlers/files.ts`        | `/files` (multipart), `/files/:id`                                                                                                                                         |
| `handlers/dictionaries.ts` | `GET /api/problem-types` — аддитивное расширение, см. «Ограничения Ф2»                                                                                                     |
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
- публичный периметр: 404 для неактивного здания и чужого токена, `done` не отдаётся в списке,
  токен анонимной заявки, комментарии анонима по токену, Word-blob, санитизация БЗ.

Данные живут в памяти (`src/mocks/db.ts`, `resetDb()` в тестах): перезагрузка страницы
возвращает сид. Сид: 4 здания (одно неактивное), 7 пользователей, 13 заявок (включая
анонимные с токенами `demo-token-1/2/3/11`), дерево БЗ из 12 узлов, 4 склада, 8 единиц
оборудования, журнал перемещений, настройки и праздники.

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
- `AppLayout` не запрашивает `app_name` из настроек (эндпоинт только для ADMIN) — используется
  `VITE_APP_NAME`/константа.
- Публичный дашборд и карточка заявки — каркас: полная версия в Ф5.
- `public/sw.js` (Web Push) — зона fe-2, здесь не создаётся.
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

- **Контракт-гэпы** (все три — аддитивные расширения, нужен `openapi.yaml` и согласование
  с lead/be; клиент уже работает через MSW):
  1. `GET /api/public/problem-types` — анониму нужен справочник для формы (TICKET-4),
     в §3.13 он есть только у ADMIN;
  2. `GET /api/public/tickets/{token}/comments?page&size` — публичная пагинация
     комментариев по 50 (в контракте был только POST);
  3. `POST /api/public/tickets/{token}/files` (multipart) — вложение комментария анонима:
     `POST /api/files` доступен только USER/ADMIN, а PUB-6 разрешает файлы.
- Публичная доска не показывает `is_overdue`: TICKET-5 не включает флаг в карточку,
  поэтому просрочка отображается только в токен-карточке (NFR-7 — не считаем на клиенте).
- Медиа статей БЗ в дереве/статье переписывается на `/api/public/files/{id}`
  (в контракте `<img src="/api/files/{id}">` требует авторизации).
- В jsdom MSW не разбирает `multipart/form-data`, поэтому тесты публичного комментария
  подменяют `uploadPublicCommentFile`; сам транспорт и лимиты проверяются в Ф1/Б3.
- Ф8 доработает `ExportButton` (состояния/ошибки) для публичной карточки; сейчас кнопка
  использует общий компонент и `skipAuth`.
- Ф11: Playwright-сценарии публичного периметра (приватное окно, cookie, Word, 375 px).

## Docker

```bash
docker build -t polar-star-web ./frontend
docker run --rm -p 8081:80 polar-star-web
```

`nginx.conf`: SPA fallback, `proxy /api` → `api:8080`, `client_max_body_size 210m`
(видео до 200 МБ), `/health` и `/health/ready` проксируются, кэширование хешированных ассетов.
