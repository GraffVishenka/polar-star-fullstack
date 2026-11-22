# Статус разработки

Обновляется `lead` по мере мержа веток. Источник требований — [PLAN.md](../PLAN.md).

## Backend (Go + MySQL)

| Блок | Ветка | Статус | Коммит мержа |
|---|---|---|---|
| Б1 Фундамент, контракт, auth | `feat/back-b1-core` | готово | `a394dcd` |
| Б2 Пользователи и роли | `feat/back-b2-users` | готово | `2ae1410` |
| Б3 Файлы и медиа | `feat/back-b3-files` | готово | `00e96ed` |
| Б4 Здания, склады, оборудование | `feat/back-b4-equipment` | готово | `3a2fd49` |
| Б5 Заявки и публичные дашборды | `feat/back-b5-tickets` | готово | `1120fec` |
| Б6 База знаний | `feat/back-b6-kb` | в работе | — |
| Б7 Настройки, справочники, время | `feat/back-b7-settings` | в работе | — |
| Б8 Push-уведомления | `feat/back-b8-push` | в работе | — |
| Б9 Статистика | — | ожидает | — |
| Б10 Корзина | — | ожидает | — |
| Б11 Документы Word/Excel | — | ожидает | — |
| Б12 Массовые перемещения | — | ожидает | — |
| Б13 Интеграция и HANDOVER | — | ожидает | — |

## Frontend (React + TS)

| Блок | Ветка | Статус | Коммит мержа |
|---|---|---|---|
| Ф1 Каркас, API-клиент, MSW | `feat/front-f1-shell` | готово | `d4f20de` |
| Ф2 Заявки (доска, drawer) | `feat/front-f2-tickets` | готово | `5c9a1ed` |
| Ф3 База знаний | `feat/front-f3-kb` | готово | `c4db09f` |
| Ф4 Оборудование | `feat/front-f4-equipment` | готово | `66d159f` |
| Ф5 Публичный периметр | `feat/front-f5-public` | готово | `771bd3a` |
| Ф6 Админ: пользователи, справочники, настройки | `feat/front-f6-admin-core` | в работе | — |
| Ф7 Админ: здания, модерация БЗ, корзина | `feat/front-f7-admin-infra` | готово | `e3c8349` |
| Ф8 Документы и печать | `feat/front-f8-documents` | в работе | — |
| Ф9 Push-уведомления | `feat/front-f9-push` | в работе | — |
| Ф10 Статистика и профиль | — | ожидает | — |
| Ф11 Интеграция, E2E | — | ожидает | — |

## Инфраструктура

- CI GitHub Actions: backend (MySQL 8.4, миграции, тесты) + frontend (typecheck, lint, tests, build) — зелёный на `main`.
- `scripts/dev-env.ps1`, `scripts/go-test-wasm.ps1` — локальная проверка в ограниченной Windows-среде.
- `docs/ENVIRONMENT.md` — что можно и нельзя проверить локально.

## Известные пробелы, закрываемые по ходу

| Пробел | Решение |
|---|---|
| Анонимная загрузка вложений | `POST /api/public/files?token=` + alias `/api/public/tickets/{token}/files` (Б3/Б5) |
| Публичная медиа БЗ | `GET /api/public/files/{id}` без токена для `scope=kb` (Б6) |
| `GET /api/problem-types` для USER | read-only справочники (Б7) |
| `executor_id=none` | поддержан в Б5 |
| Formatting KB из Ф3 | исправлено в main |
