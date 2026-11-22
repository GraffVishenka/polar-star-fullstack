# HANDOVER — передача проекта другому ИИ-агенту

Документ написан так, чтобы поднять проект и продолжить работу **без контекста
чата**. Структура — по PLAN.md §9 (15 разделов). Актуально на 2026-11-22,
версия `0.1.0`.

## 1. Назначение и статус

Внутреннее веб-приложение «Заявки · База знаний · Оборудование»: учёт заявок с
дедлайнами по рабочему календарю, база знаний с медиа, учёт оборудования с
перемещениями, push-уведомления, статистика, Admin-панель и публичный периметр
для анонимных заявок.

**Реализовано (backend):** Б1 ядро/аутентификация, Б2 пользователи, Б3 файлы,
Б4 здания/склады/оборудование, Б5 заявки, Б6 база знаний, Б7 настройки/
справочники/праздники, Б8 push, Б9 статистика, Б10 корзина, Б11 документы
(Word/Excel/ведомость), Б12 массовое перемещение, Б13 демо-сид/поставка/
документация.

**Осознанно вне объёма:** только перечисленное в PLAN §2 («Сознательно не
входит»): email/SMS, SSO, PWA-офлайн, PDF, версии статей, аудит-лог. Все
эндпоинты контракта реализованы, заглушек `501` больше нет (проверяется
`make openapi`). Фронтенд — отдельные блоки Ф1–Ф11, в этом репозитории лежит
готовое SPA (`frontend/`); какие экраны уже готовы — смотрите
`docs/PROGRESS.md`.

Осталось до «полного продакшена»: реальный браузерный E2E (Playwright) на
полном стеке, метрики Prometheus, проверка push на iOS/Safari — см. §13.

Последние изменения — `git log --oneline -15`; контракт — `openapi/openapi.yaml`;
изменения контракта — `docs/contract-diff.md`.

## 2. Стек и версии

| Слой | Технология | Версия/обоснование |
|---|---|---|
| Backend | Go | `backend/go.mod` (Go 1.26, локально 1.27.4); stdlib + `chi v5`, `database/sql`, `golang-migrate`, `golang.org/x/crypto/argon2`, `webpush-go`, `bluemonday` |
| БД | MySQL | 8.4, `utf8mb4_0900_ai_ci`, `default-time-zone=+00:00` |
| Frontend | React 19 + TypeScript + Ant Design 5 | Vite; MSW для разработки без backend |
| Контракт | OpenAPI 3.0.3 | `openapi/openapi.yaml` — единственный источник истины |
| Пуши | Web Push (VAPID) | `webpush-go`, no-op без ключей |
| Поставка | Docker + Compose | `backend/Dockerfile`, `frontend/Dockerfile` (nginx) |

Почему так: единственный сервер и MySQL достаточно для нагрузки (12 сотрудников,
50→200 RPS чтения), локальный диск проще S3 и разрешён PLAN §4, а Web Push не
требует email/SMS-инфраструктуры.

## 3. Структура репозитория

```
backend/
  cmd/api/            точка входа: serve|migrate|seed [--demo]|version, воркеры
  internal/
    auth/             логин, JWT access + refresh-cookie, роли, пароли
    users/            Admin CRUD пользователей, assignable
    buildings/        здания и склады
    equipment/        оборудование, перемещения, массовое перемещение (Б12)
    tickets/          заявки, комментарии, публичный периметр
    files/            загрузка/выдача медиа, MIME, лимиты, очистка
    kb/               дерево БЗ, статьи, видимость, медиа статей
    settings/         singleton-настройки, праздники, календарь (провайдер)
    dictionaries/     виды проблем и типы оборудования
    notifications/    push-подписки, outbox, напоминания, VAPID
    stats/            личная и сводная статистика, ExportRows для Excel
    httpserver/       маршруты, middleware, health, Swagger UI, статика
    platform/         apierr, календарь, часы, rate limit, request id
    db/               открытие БД, миграции, Seed, SeedDemo
  migrations/         0001_core … 0008_push (встроены через embed)
  Dockerfile
frontend/             SPA (React+TS), Dockerfile + nginx.conf
openapi/openapi.yaml  контракт
deploy/               docker-compose.prod.yml, backup.sh, k6/
scripts/              dev-env, go-test-wasm, e2e.sh, validate_openapi.mjs, acceptance.md
docs/                 HANDOVER.md, decisions.md, contract-diff.md, ENVIRONMENT.md, PROGRESS.md, templates.md
PLAN.md               полное ТЗ
```

Точки входа: `backend/cmd/api/main.go` (сборка зависимостей и маршрутов),
`frontend/src/main.tsx`.

## 4. Запуск

### 4.1 Docker (полный стек)

```bash
cp .env.example .env         # заполнить секреты (production — обязательно)
make up                      # mysql + api + frontend
make migrate                 # миграции (api их также применяет при старте)
make seed                    # базовый сид: настройки, справочники, ADMIN
make seed-demo               # демо-данные (идемпотентно): 4 здания, 50 заявок, …
```

- Frontend: <http://localhost:8081> (nginx проксирует `/api` на `api:8080`)
- API: <http://localhost:8080>, Swagger UI: <http://localhost:8080/docs>
- Health: `/api/health`, `/api/health/ready`

`make demo` = `up` + `seed-demo`. Демо-данные также включаются
`SEED_DEMO=true` (сид выполнится при старте API).

### 4.2 Без Docker (backend)

```bash
cd backend
DATABASE_URL='mysql://polar:polar@tcp(127.0.0.1:3306)/polar_star' \
JWT_SECRET='dev-only-secret-change-me-please-32' \
SEED_ADMIN_PASSWORD='ChangeMe_12345' \
  go run ./cmd/api serve
```

### 4.3 Тестовые учётки

| Логин | Пароль | Роль | Доступ |
|---|---|---|---|
| `admin` | `SEED_ADMIN_PASSWORD` (в compose по умолчанию `ChangeMe_12345`) | ADMIN | всё, все здания |
| `demo.admin1`, `demo.admin2` | `SEED_DEMO_PASSWORD` (`Demo_12345`) | ADMIN | всё, все здания |
| `demo.user01…demo.user10` | `Demo_12345` | USER | заявки своего здания; БЗ/оборудование — все |
| аноним | — | anonymous | публичные заявки по `access_url` (токен выдаётся один раз при создании) |

Демо-заявки созданы без публичного токена (`access_token_hash IS NULL`):
публичную ссылку получают только заявки, созданные через
`POST /api/public/tickets` (в ответе `access_token`/`access_url`).

### 4.4 Переменные окружения

Все переменные читаются из окружения (`backend/internal/config/config.go`).
Секреты — только через env; `.env` в git не коммитится.

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `APP_ENV` | `development` | `production` включает строгие проверки и Secure-cookie |
| `APP_VERSION` | `0.1.0` | версия в `/api/health` |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |
| `PORT` / `HTTP_ADDR` | `8080` / `:8080` | адрес API |
| `DATABASE_URL` | — (обязательна) | `mysql://user:pass@tcp(host:3306)/db` или DSN |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / `DB_CONN_MAX_LIFETIME` | `25`/`10`/`3m` | пул соединений |
| `JWT_SECRET` | — (≥32 символов) | подпись access-токенов |
| `ACCESS_TOKEN_TTL` / `REFRESH_TOKEN_TTL` | `15m` / `168h` | время жизни токенов |
| `JWT_ISSUER` | `polar-star-service` | issuer токена |
| `TZ` (алиас `TIMEZONE`) | `Asia/Yekaterinburg` | IANA-пояс приложения (CAL-2); в БД — UTC |
| `CORS_ORIGIN` | `http://localhost:5173,http://localhost:8081` | через запятую |
| `TRUST_PROXY` | `true` | доверять `X-Forwarded-For` (за nginx) |
| `UPLOAD_DIR` | `./data/uploads` | корень медиа |
| `MAX_UPLOAD_BYTES` | `209715200` | общий лимит загрузки (видео 200 МБ) |
| `VAPID_PUBLIC_KEY` / `VAPID_PRIVATE_KEY` / `VAPID_SUBJECT` | — / — / `mailto:admin@example.com` | Web Push; без ключей push в no-op |
| `SEED_ADMIN_LOGIN` / `SEED_ADMIN_PASSWORD` / `SEED_ADMIN_FULL_NAME` | `admin` / — / `Администратор` | первичный ADMIN |
| `SEED_DEMO` | `false` | включает демо-данные при `seed`/старте |
| `SEED_DEMO_PASSWORD` | `Demo_12345` | пароль демо-учёток |
| `OPENAPI_PATH` | `openapi/openapi.yaml` | Swagger UI и статика |
| `SHUTDOWN_TIMEOUT` | `15s` | graceful shutdown |
| `FRONTEND_PORT` (compose) | `8081` | порт nginx локально |
| `HTTP_PORT` (prod compose) | `80` | порт nginx в production |
| `MYSQL_*` (compose) | `polar_star`/`polar`/`polar` | БД в docker |
| `BACKUP_DIR` / `RETENTION_DAYS` | `./backups` / `7` | `deploy/backup.sh` |

VAPID-ключи: `npx web-push generate-vapid-keys`. В production API не стартует без
них (NOTIF-1); публичный ключ отдаётся `GET /api/push/public-key`.

## 5. Контракт API

- Файл: `openapi/openapi.yaml` (OpenAPI 3.0.3). Изменения **только аддитивные**:
  новые пути/поля/коды; переименования и удаления запрещены. История и
  обоснования — `docs/contract-diff.md`.
- Формат ошибки: `{"code","message","details","request_id"}` (`platform/apierr`).
- Проверка: `make openapi` → `node scripts/validate_openapi.mjs` (YAML, `$ref`,
  `operationId`, path-параметры, security-схемы; офлайн). В CI дополнительно
  запускается `redocly lint` (информационно).
- Регенерация типов фронта:

  ```bash
  npx openapi-typescript openapi/openapi.yaml -o frontend/src/api/types.ts
  ```

  `frontend/src/api/types.ts` вручную не редактируется.
- Все операции контракта реализованы, заглушек `501` нет (см. §8).

## 6. Модель данных

Миграции `backend/migrations/0001…0008`, применяются `golang-migrate` (embed) при
`api migrate` и старте API; `down`-файлы есть у всех. Правила:

- все `DATETIME` — в UTC (DSN `loc=UTC`, MySQL `default-time-zone=+00:00`);
  календарные даты (`deadline`, `holiday_date`) — `DATE` без пояса;
- физическое удаление зданий/складов/пользователей не выполняется — деактивация
  и `deleted_at`/корзина; FK — `RESTRICT` там, где история важна;
- имена таблиц и колонок не переименовываются; новая миграция — только вперёд.

Ключевые таблицы:

| Таблица | Назначение | Связи/индексы |
|---|---|---|
| `buildings` | здания, `is_active` | `idx_buildings_active` |
| `users` | аккаунты, `role`, `building_id` (USER обязателен), `is_active`, `must_change_password` | `uq_users_login`, FK → buildings RESTRICT |
| `refresh_tokens` | хеши refresh-токенов, ротация | `uq_refresh_tokens_hash` |
| `settings` | singleton `id=1`: `app_name`, `deadline_days`, `workdays` (JSON 1..7), `timezone`, `deadline_reminder_time`, `push_enabled`, `status_labels/colors` | чек `deadline_days 1..30` |
| `problem_types`, `equipment_types` | справочники (деактивация вместо удаления) | `uq_*_name` |
| `holidays` | праздники (исключения календаря) | `uq_holidays_date`, `holiday_year` generated |
| `warehouses` | склады/кабинеты здания | `uq_warehouses_building_live_name` |
| `equipment` | единицы, всегда на складе | FK type/warehouse RESTRICT; `uq_equipment_registration_live` |
| `equipment_movements` | неизменяемый журнал | FK equipment/склады/users RESTRICT |
| `tickets` | заявки, `status` ENUM, `deadline`, `executor_id`, `completed_at`, `access_token_hash` | индексы по building+status+created, executor+status |
| `ticket_comments`, `comment_files` | комментарии и вложения | FK на tickets/comments/files |
| `files` | метаданные медиа: путь, mime, размер, sha256, `scope`, `linked_at` | orphan-очистка по `linked_at` |
| `kb_nodes` | дерево статей, `parent_id` самоссылка, `is_hidden_for_anon`, `deleted_at` | `idx_kb_parent_sort` |
| `push_subscriptions`, `notification_outbox` | подписки и outbox | `uq_push_subscriptions_endpoint`, `uq_notification_outbox_idempotency` |

Бэклог: индексы `tickets` под дашборд и статистику; при росте — партиционирование
не требуется (10k заявок проверены k6).

## 7. Правила видимости, зданий и уведомлений

### 7.1 Роли и заявки (PLAN §3.1)

- anonymous: создаёт заявку в **активном** здании и видит её по `access_token`
  (`/api/public/tickets/{token}`); список/дашборд — публичный.
- USER: видит **только заявки своего здания** (фильтр `building_id`
  принудительно = своему; запрос чужого здания → 403); БЗ и оборудование — все.
- ADMIN: все заявки, смена здания, корзина (Б10), удаление.

Здание заявки проверяется на каждом запросе по `auth.UserFrom`; смена здания
пользователя действует сразу.

### 7.2 Публичный периметр

- `/` — активные здания со счётчиками (`GET /api/public/buildings`);
- `/b/{id}` — дашборд здания (`GET /api/public/tickets?building_id=`), cookie
  выбранного здания на 30 дней;
- деактивированное здание не принимает новые заявки, но существующие остаются
  доступными ADMIN; публичная карточка по токену работает всегда.

### 7.3 База знаний

- Публично: корни с `is_hidden_for_anon=0` и всё их поддерево.
- Скрытый корень скрывает **всё поддерево** (даже если у потомков флаг 0).
- Медиа публичной статьи: `GET /api/public/files/{id}` без токена отдаёт файл,
  если он `scope=kb` и привязан `/api/files/{id}` в `content_html` хотя бы
  одного публичного узла; иначе 404. С токеном заявки — обычная проверка Б3.

### 7.4 Файлы

- `GET /api/files/{id}`: ADMIN — любой; USER — файл своей заявки/своего здания;
  `scope=kb` — любой авторизованный (БЗ общая); иначе 403/404.
- `GET /api/public/files/{id}?token=`: токен заявки (TICKET-13), либо публичная
  статья БЗ без токена (см. выше).
- Лимиты: фото ≤10 МБ, видео ≤200 МБ, MIME по сигнатуре (не по расширению),
  имя на диске — UUID; путь `UPLOAD_DIR/YYYY/MM/{uuid}.{ext}`.

### 7.5 Push (NOTIF)

- События: новая заявка в здании → USER этого здания + ADMIN; смена статуса/
  назначение исполнителя — участникам; напоминание о дедлайне — исполнителю.
- Напоминания: ежедневно в `deadline_reminder_time` по `timezone` из настроек;
  idempotency_key `deadline:<ticket>:<user>:<date>` — повторный запуск не
  дублирует доставку.
- Без VAPID-ключей push в no-op: события не создаются, API и подписки работают.

### 7.6 Календарь и часовой пояс

- Рабочие дни — маска `workdays` (по умолчанию Пн–Сб), минус `holidays`.
- Дедлайн: `deadline_days` **рабочих** дней вперёд от даты создания (TICKET-3);
  при переносе через праздник — следующий рабочий день.
- Хранение: даты в UTC; сравнение «просрочено» — по календарной дате в поясе
  приложения (пример: завершение `2026-05-02T20:00Z` в Екатеринбурге — это
  `03.05 01:00`, значит дедлайн 2 мая просрочен).
- Смена `timezone` в настройках применяется к новым расчётам сразу (календарь —
  атомарный провайдер `settings.CalendarProvider`, Б7/CAL-5).

## 8. Реализованные модули (файлы, эндпоинты, тесты, ограничения)

| Модуль | Файлы | Эндпоинты (кратко) | Тесты | Ограничения |
|---|---|---|---|---|
| auth | `internal/auth/*` | `POST /api/auth/login|refresh|logout`, `GET /api/auth/me`, `POST /api/auth/password` | `*_test.go` | refresh — только cookie; нет самостоятельной регистрации |
| users | `internal/users/*` | `GET /api/users/assignable`, `GET/POST/PATCH /api/admin/users`, `POST /api/admin/users/{id}/password` | да | USER обязан иметь здание |
| buildings | `internal/buildings/*` | `GET /api/buildings`, `/api/warehouses`; admin CRUD | да | удаление — только пустого склада; здание деактивируется |
| equipment | `internal/equipment/*` | `GET/POST /api/equipment`, `GET/PATCH/DELETE /api/equipment/{id}`, `POST /api/equipment/{id}/move`, `GET .../movements`, `POST /api/equipment/bulk-move` | unit + `repo_mysql_test.go` (skip без `TEST_DATABASE_URL`) | ведомость помещения — в `documents` |
| tickets | `internal/tickets/*` | `GET/POST /api/tickets`, `GET/PATCH/DELETE /api/tickets/{id}`, комментарии, публичные `/api/public/tickets*` | да | Word-экспорт — в `documents` |
| files | `internal/files/*` | `POST /api/files`, `GET/HEAD /api/files/{id}`, `/api/public/files/{id}`, `POST /api/public/files` | да | локальный диск; видео не транскодируется |
| kb | `internal/kb/*` | `GET/POST /api/kb/*`, `PATCH /api/kb/nodes/{id}`, `move`, `visibility`, `DELETE`; публичные `/api/public/kb/*` | да | нет версий статей |
| settings | `internal/settings/*` | `GET /api/public/settings`, `GET/PUT /api/admin/settings`, CRUD праздников | да | singleton `id=1` |
| dictionaries | `internal/dictionaries/*` | `GET /api/problem-types`, `/api/equipment-types`, `/api/public/problem-types`, admin CRUD | да | удаление = деактивация |
| notifications | `internal/notifications/*` | `GET /api/push/public-key`, `POST /api/push/subscriptions`, `DELETE /api/push/subscriptions/{id}` | да | нет email/SMS |
| stats | `internal/stats/*` | `GET /api/stats/me`, `GET /api/admin/stats`; `ExportRows` для Excel | unit + MySQL-тест | N+1 по сотрудникам в admin-списке |
| documents | `internal/documents/*` | `GET /api/tickets/{id}/export.docx`, `/api/public/tickets/{token}/export.docx`, `/api/stats/export.xlsx`, `/api/warehouses/{id}/inventory.xlsx` | да (`docx_test`, `handler_test`) | ведомость ≤ 1000 позиций; PDF не входит в объём |
| trash | `internal/trash/*` | `GET /api/admin/trash`, `POST /api/admin/trash/{type}/{id}/restore`, `DELETE /api/admin/trash/{type}/{id}` | unit + MySQL-тест | срок хранения 90 дней (TRASH-4), очистка в 03:00 |
| httpserver | `internal/httpserver/*` | маршрутизация, middleware, health, `/docs`, контракт | да | — |

Все операции контракта реализованы, заглушек `501` нет. Форматы документов и
ограничения — `docs/templates.md`.

## 9. Фоновые процессы

| Процесс | Код | Период | Что делает |
|---|---|---|---|
| Очистка orphan-файлов | `files.NewCleaner(...).Run` (`main.go`) | 1 час, хранение 24 ч | удаляет файлы с `linked_at IS NULL` старше суток и временные staging-файлы; ошибки только логируются |
| Push outbox + напоминания | `notifications.NewWorker(...).Run` | 30 секунд | отправляет `pending` из outbox с backoff; раз в сутки в `deadline_reminder_time` создаёт напоминания по дедлайнам с idempotency |
| Автоочистка корзины | `trash.NewCleaner(...).Run` (`main.go`) | ежедневно в 03:00 по поясу приложения | окончательно удаляет элементы, удалённые более 90 дней назад (TRASH-4), вместе с медиа |
| HTTP-сервер | `cmd/api` | — | graceful shutdown по SIGINT/SIGTERM (`SHUTDOWN_TIMEOUT`) |

Включение: всё стартует вместе с `api serve`; отдельной настройки нет.
Отладка: `docker compose logs -f api | grep -E "cleaner|worker|push|reminder"`.
Ручной прогон очистки — `files.Cleaner.RunOnce` (используется в тестах).

## 10. Файлы и шаблоны

- Медиа: `UPLOAD_DIR/YYYY/MM/{uuid}.{ext}` (пояс приложения), БД хранит
  относительный путь, mime, размер, sha256. Отдача — только через API.
- Документы (Б11): Word-шаблон `backend/templates/ticket.docx` + генератор
  `backend/scripts/gen-ticket-template.go` (перегенерация через `go run`),
  Excel собирается на лету. Плейсхолдеры, листы и ограничения — `docs/templates.md`.
  Эндпоинты: `export.docx` (заявка), `stats/export.xlsx`, `inventory.xlsx`.
- Статика фронта: `frontend/dist` внутри nginx-образа; `nginx.conf` проксирует
  `/api` на `api:8080`, `/health` — health-проверки.

## 11. Эксплуатация

- **Логи:** stdout/stderr, JSON-совместимый `slog` (`LOG_LEVEL`), `request_id` в
  каждой строке и в ответах API; ошибки воркеров — с именем воркера.
- **Здоровье:** `/api/health` (процесс), `/api/health/ready` (БД).
- **Бэкап:** `make backup` → `deploy/backup.sh` (mysqldump + tar медиа,
  sha256, ротация 7 дней; `BACKUP_MODE=compose` по умолчанию в Makefile).
  Восстановление:

  ```bash
  gunzip -c backups/db-*.sql.gz | mysql -u"$MYSQL_USER" -p"$MYSQL_DATABASE"
  tar -xzf backups/uploads-*.tar.gz -C /data      # каталог uploads
  ```

- **Типовые инциденты:** 502 от nginx → `api` не поднялся (смотрите логи, чаще
  всего MySQL/`JWT_SECRET`); push не приходит → нет VAPID-ключей или HTTPS;
  «демо-данные не появились» → `make seed-demo` и проверьте `SEED_DEMO`.
- **Метрики:** Prometheus не подключён (PLAN §8); ориентируйтесь на access-логи
  nginx и k6-профиль `deploy/k6/load.js` (200 RPS, p95 ≤ 300 мс).
- **Прод:** `deploy/docker-compose.prod.yml`, опубликован только фронтенд (80),
  TLS — на внешнем reverse proxy.

## 12. Правила параллельной работы агентов

- Владение каталогами — AGENTS.md; пересечение запрещено. Общие файлы
  (`openapi/openapi.yaml`, `cmd/api/main.go`, `routes.go`, `server.go`,
  `src/api/types.ts`) меняются аддитивно и только после согласования с `lead`.
- Ветки: `feat/back-bN-...` / `feat/front-fN-...` от актуального `main`, PR/merge
  делает `lead`; один коммит — один логический шаг.
- Миграции: следующий свободный номер `000N_...`, только вперёд, с `down`.
- Контракт: только аддитивно; при добавлении поля — пометка в коммите и запись
  в `docs/contract-diff.md`.
- Качество перед merge: `gofmt -l .`, `go vet ./...`, `go build ./...`,
  тесты; локально на Windows — `scripts/go-test-wasm.ps1`.
- Секреты только через env; в репозиторий не коммитятся.

## 13. Бэклог и открытые вопросы

См. PLAN.md §8 (полный список). Все блоки Б1–Б13 реализованы; приоритеты
доработок:

1. Реальный E2E Playwright на полном стеке (сейчас `scripts/e2e.sh` — API smoke).
2. Метрики Prometheus + алерты, structured access-log middleware.
3. Оптимизация `/api/admin/stats` (N+1) при росте числа сотрудников.
4. Push: проверка на iOS/Safari, хранение VAPID в секретах CI.
5. Индексы/партиционирование заявок при объёме > 100k и повторный k6-профиль.
6. Проверка восстановления бэкапа по расписанию (сейчас — ручной сценарий).

## 14. Чек-лист приёмки

Автоматически: `make openapi`, `cd backend && go vet ./... && go build ./...`,
`make test` (в CI — с MySQL), `make demo && make e2e`.

Вручную (полный список — `scripts/acceptance.md`):

- [ ] `docker compose up -d --build` → `docker compose ps` — все сервисы healthy.
- [ ] `make seed-demo` → в логах API учётки и «demo seed: готово».
- [ ] `http://localhost:8081` — список активных зданий со счётчиками.
- [ ] `demo.user01` видит только заявки «Демо-корпуса А»; чужая заявка → 403.
- [ ] Аноним создаёт заявку, получает ссылку `/t/{token}`, видит её и скачивает
      вложения.
- [ ] Дедлайн учитывает Пн–Сб и праздник; напоминание приходит в 10:00 по
      Екатеринбургу (при настроенных VAPID).
- [ ] БЗ: дерево глубиной 5+, скрытый раздел не виден анониму, медиа статьи
      открывается без токена.
- [ ] Оборудование: одиночное и массовое перемещение, журнал, `skipped_count`.
- [ ] Статистика: личная и сводная, `null` при пустом периоде.
- [ ] Документы: Word заявки, Excel статистики и ведомость помещения
      скачиваются; USER не получает чужой отчёт.
- [ ] Корзина: удалённая заявка видна в `/api/admin/trash`, восстановление
      возвращает её, окончательное удаление убирает.
- [ ] `make backup` создаёт `db-*.sql.gz` и `uploads-*.tar.gz`, ротация 7 дней.
- [ ] `k6 run deploy/k6/load.js` — p95 ≤ 300 мс, ошибок < 1 % (10k заявок).

## 15. Карта решений (ADR)

Полные формулировки — `docs/decisions.md`:

- MySQL 8 как единственное хранилище; UTC в БД, пояс приложения — в настройках.
- Рабочий календарь считается на сервере в Go (без `CONVERT_TZ`, без tz-таблиц).
- Здание — ось видимости заявок и публичной навигации; деактивация вместо удаления.
- Корзина: удаление помечает `deleted_at`/`deleted_by`, хранение 90 дней,
  окончательная очистка ежедневно вместе с медиа.
- Документы генерируются на сервере: Word — по embed-шаблону с заменой
  плейсхолдеров, Excel — из тех же данных, что JSON-эндпоинты (без второго SQL).
- Локальный диск для медиа + отдача через API с проверкой прав (без S3/CDN).
- JWT access (15 мин, память) + refresh httpOnly cookie (7 дней, ротация).
- argon2id для паролей; блокировка отзывает refresh-токены.
- Определение MIME по сигнатуре, UUID-имена, лимиты 10/200 МБ.
- Санитизация HTML статей БЗ через bluemonday; медиа-ссылки проверяются
  авторизатором, а не доверенным HTML.
- Статистика: медиана — среднее двух центральных значений, округление до 0.1;
  `on_time_percent` — только по заявкам с дедлайном.
- Массовое перемещение: одна транзакция, блокировки по возрастанию id,
  пропуск уже перемещённых (`skipped_count`), полный откат при 404.
- Контракт аддитивен: `skipped_count` и описания добавлены без переименований;
  `employees`/`deleted_by` не выдумывались (см. `docs/contract-diff.md`).
- Демо-сид идемпотентен по детерминированным маркерам; не перезаписывает
  настройки, изменённые администратором.
- Web Push без email/SMS; без VAPID — no-op, а не ошибка старта.

---

**Если что-то не сходится с кодом — верьте коду и `git log`**, а этот документ
правьте отдельным коммитом `docs(handover): ...`.
