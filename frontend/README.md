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

| Маршрут                                   | Доступ                     | Состояние Ф1                                      |
| ----------------------------------------- | -------------------------- | ------------------------------------------------- |
| `/`                                       | аноним (`RequireAnonOnly`) | выбор здания с реальными счётчиками               |
| `/b/:buildingId`, `/b/all`                | все                        | каркас публичного дашборда (3 колонки без `done`) |
| `/t/:token`                               | все (по токену)            | карточка по токену: поля, комментарии, Word       |
| `/public/kb`                              | все                        | заглушка Ф3 (fe-2)                                |
| `/login`, `/change-password`              | аноним / авторизованный    | реальные формы (RHF + zod)                        |
| `/tickets`                                | авторизованные             | каркас доски Ф2 (fe-1)                            |
| `/kb`, `/equipment`, `/stats`, `/profile` | авторизованные             | заглушки Ф3/Ф4/Ф10                                |
| `/admin/*`                                | только `ADMIN`             | заглушки Ф6/Ф7/Ф10                                |

Заглушки живут в `src/pages/placeholders/PlaceholderPage.tsx` — они **не занимают**
каталоги чужих агентов. Замена — одна строка в `src/app/routes/appRoutes.tsx`.

## MSW: что покрыто

Все эндпоинты §3.13. Хендлеры разбиты по доменам — правит только владелец домена:

| Файл                    | Эндпоинты                                                                                   |
| ----------------------- | ------------------------------------------------------------------------------------------- |
| `handlers/auth.ts`      | `/auth/login                                                                                | refresh                       | logout | me  | password`, push-подписки |
| `handlers/public.ts`    | `/public/buildings`, `/public/tickets*`, `/public/files/:id`, `/public/kb/*`                |
| `handlers/tickets.ts`   | `/tickets*`, комментарии, `export.docx`, `/users/assignable`                                |
| `handlers/kb.ts`        | `/kb/tree                                                                                   | nodes*`, `move`, `visibility` |
| `handlers/equipment.ts` | `/buildings*`, `/warehouses*`, `/equipment*`, `inventory.xlsx`                              |
| `handlers/admin.ts`     | `/admin/users*`, `/admin/settings`, справочники, праздники, `/admin/trash*`, `/admin/stats` |
| `handlers/stats.ts`     | `/stats/me`, `/stats/export.xlsx`                                                           |
| `handlers/files.ts`     | `/files` (multipart), `/files/:id`                                                          |

Что моделируется «как на сервере»:

- роли и видимость заявок по зданию (USER чужого здания → `404`, TICKET-7);
- `must_change_password` → `428` на всех эндпоинтах кроме `me/password/logout`;
- смена здания заявки ADMIN'ом с `executor_reset: true` (TICKET-9);
- `is_overdue` и `deadline` считаются в моках (клиент их только отображает);
- дерево БЗ глубины 6, скрытый корневой раздел (`/public/kb/*` → `404`), цикл → `409 CYCLE_DETECTED`;
- `SAME_WAREHOUSE`, `DUPLICATE_REGISTRATION_NUMBER`, `WAREHOUSE_NOT_EMPTY`, `BUILDING_IN_USE`,
  `LAST_ADMIN`, `CANNOT_MODIFY_SELF`, `BUILDING_REQUIRED_FOR_USER`, `409/422` в целом;
- корзина: восстановление и безвозвратное удаление, предупреждения (неактивное здание, потомки БЗ).

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
- `AppLayout` не запрашивает `app_name` из настроек (эндпоинт только для ADMIN) — используется
  `VITE_APP_NAME`/константа.
- Публичный дашборд и карточка заявки — каркас: полная версия в Ф5, доска заявок — Ф2.
- `public/sw.js` (Web Push) — зона fe-2, здесь не создаётся.
- MSW-видео отдаётся пустым потоком: превью фото работает, реальное видео проверяется на бэкенде.
- `mockServiceWorker.js` попадает в `dist/`, но в production не регистрируется
  (`VITE_ENABLE_MSW=false` в Dockerfile).

## Docker

```bash
docker build -t polar-star-web ./frontend
docker run --rm -p 8081:80 polar-star-web
```

`nginx.conf`: SPA fallback, `proxy /api` → `api:8080`, `client_max_body_size 210m`
(видео до 200 МБ), `/health` и `/health/ready` проксируются, кэширование хешированных ассетов.
