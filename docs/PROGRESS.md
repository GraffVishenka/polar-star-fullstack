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
| Б6 База знаний | `feat/back-b6-kb` | готово | `ec77428` |
| Б7 Настройки, справочники, время | `feat/back-b7-settings` | готово | `4b8d76d` |
| Б8 Push-уведомления | `feat/back-b8-push` | готово | `1425665` |
| Б9 Статистика | `feat/back-b9-stats` | готово | `b89c3be` |
| Б10 Корзина | `feat/back-b10-trash` | готово | `7f7a8ab` |
| Б11 Документы Word/Excel | `feat/back-b11-documents` | готово | `5734981` |
| Б12 Массовые перемещения | `feat/back-b12-bulk-move` | готово | `1a31d47` |
| Б13 Интеграция и HANDOVER | `feat/back-b13-integration` | готово | `e5e1da9` |

## Frontend (React + TS)

| Блок | Ветка | Статус | Коммит мержа |
|---|---|---|---|
| Ф1 Каркас, API-клиент, MSW | `feat/front-f1-shell` | готово | `d4f20de` |
| Ф2 Заявки (доска, drawer) | `feat/front-f2-tickets` | готово | `5c9a1ed` |
| Ф3 База знаний | `feat/front-f3-kb` | готово | `c4db09f` |
| Ф4 Оборудование | `feat/front-f4-equipment` | готово | `66d159f` |
| Ф5 Публичный периметр | `feat/front-f5-public` | готово | `771bd3a` |
| Ф6 Админ: пользователи, справочники, настройки | `feat/front-f6-admin-core` | готово | `2a9777c` |
| Ф7 Админ: здания, модерация БЗ, корзина | `feat/front-f7-admin-infra` | готово | `e3c8349` |
| Ф8 Документы и печать | `feat/front-f8-documents` | готово | `3a921e4` |
| Ф9 Push-уведомления | `feat/front-f9-push` | готово | `1425665` |
| Ф10 Статистика и профиль | `feat/front-f10-stats` | готово | `1425665` |
| Ф11 Интеграция, E2E | `feat/front-f11-e2e` | готово | `f58269c` |

## Инфраструктура

- CI GitHub Actions: backend (MySQL 8.4, миграции, тесты) + frontend (typecheck, lint, tests, build).
- **Локальный полный стек (проверено 2026-11-23):** Docker Desktop 29.9.3 + WSL2,
  `docker compose up -d --build`; MySQL 8.4, API и frontend (nginx) — healthy.
  Миграции (9) и `seed --demo` применены; Go-тесты `go test -p 1 ./...` с MySQL —
  все пакеты `ok`.
- `scripts/dev-env.ps1`, `scripts/go-test-wasm.ps1` — локальная проверка в ограниченной Windows-среде.
- `docs/ENVIRONMENT.md` — что можно и нельзя проверить локально.
- CI аннотирует упавшие Go-тесты (`scripts/ci_test_report.py`), т.к. логи без токена недоступны.

## Известные решения и пробелы

| Вопрос | Решение |
|---|---|
| Анонимная загрузка вложений | `POST /api/public/files?token=` + alias `/api/public/tickets/{token}/files` |
| Публичная медиа БЗ | `GET /api/public/files/{id}` без токена для `scope=kb` с проверкой публичности статьи |
| `GET /api/problem-types`, `/equipment-types`, `/public/problem-types`, `/public/settings` | добавлены Б7 |
| `executor_id=none` | поддержан Б5 |
| Дедлайн: примеры CAL-6 | нормативно CAL-4 (пятница+2 Пн–Сб → понедельник) |
| Тесты KB на MySQL | ожидания исправлены lead по факту семантики (append в корень, видимость childA) |
| Formatting KB/Ф6 | исправлено в main |
