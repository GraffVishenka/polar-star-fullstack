# Изменения контракта (contract diff)

Документ фиксирует **все аддитивные изменения** `openapi/openapi.yaml` за проект
и объясняет, почему они понадобились. Контракт — единственный источник истины
(AGENTS.md): существующие пути, схемы и коды ответов не переименовывались и не
удалялись, кроме снятия заглушек `501` у уже реализованных операций.

Правило проверки: `git log --oneline -- openapi/openapi.yaml` показывает каждый
шаг; ниже — сводка по блокам.

## 1. Исходный контракт (Б1)

`f6bd68a feat(b1): full OpenAPI 3.0.3 contract for all PLAN §3.13 endpoints` —
полный каркас: все пути, схемы, коды ответов и заглушки `501` для ещё не
реализованных операций. Дальнейшие блоки только снимали `501` и добавляли
недостающие поля.

## 2. Б3 — загрузка вложений анонимом

`787cbf6 docs(openapi): add POST /api/public/files (b3, additive)`

- **`POST /api/public/files?token=`** (multipart). TICKET-14/PUB-6 разрешают
  анониму прикладывать файл к комментарию, но эндпоинта загрузки в контракте не
  было. Тот же лимит 10 МБ и rate limit 5/10 мин, что у публичных заявок.
- Существующие схемы и ответы не менялись.

## 3. Б5 — заявки и публичные комментарии

`d0b17b4`, `ce32882`, `c93c687`

- Снят `501` у реализованных операций Б2/Б5: CRUD заявок и комментариев,
  `GET /api/public/buildings`, публичный дашборд, создание/карточка публичной
  заявки, `GET /api/users/assignable`.
- **`GET /api/public/tickets/{token}/comments`** — плоский список комментариев
  для публичной карточки (Ф5).
- **`POST /api/public/tickets/{token}/files`** — алиас Б3
  `POST /api/public/files?token=`: токен заявки передаётся в пути, внутри
  обёртки транслируется в query; лимит тот же. Добавлен по просьбе Ф5, чтобы не
  собирать URL с query на клиенте.
- **`CommentSizeQuery`** — размер страницы комментариев по умолчанию 50
  (аддитивный query-параметр; старые вызовы без него работают).
- **`is_overdue`** в `PublicTicketDetail` — просрочка вычисляется сервером
  (TICKET-10), клиент не сравнивает даты сам.
- `executor_id` принимает целое или литерал **`none`** (исполнитель не назначен,
  Ф2-фильтр).
- Удалён дубликат блока `POST /api/public/files` (после ребейза Б3 уже добавил
  его); сохранена схема Б3.

## 4. Б6 — медиа публичной базы знаний

`4577e31 docs(openapi): public kb media without token (b6, additive)`

- У **`GET /api/public/files/{id}`** параметр `token` стал необязательным.
  Без токена endpoint отдаёт медиа **публичной** статьи БЗ: файл должен иметь
  `scope=kb` и быть привязан через `/api/files/{id}` в `content_html` хотя бы
  одного публичного узла (корень не скрыт, узел не удалён), иначе `404`.
- Вариант с `?token=` (TICKET-13) не изменён; остальные коды ответов сохранены.
- Причина: Ф5 рендерит картинки публичной БЗ, а отдельный
  `/api/public/kb/files/{id}` дублировал бы проверку доступа. Реализация —
  интерфейс `files.KBMediaAuthorizer` в домене `kb` (без циклической
  зависимости `files → kb`).

## 5. Б7 — настройки, праздники и справочники

`561b830 docs(openapi): additive public settings, problem-types and holiday patch`

- **`GET /api/public/settings`** + схема `PublicSettings` — имя приложения и
  названия статусов для публичного дашборда и формы (TICKET-4/5). Полные
  настройки остаются только у ADMIN.
- **`GET /api/public/problem-types`** + `PublicProblemType` — активные виды
  проблем для формы анонимной заявки.
- **`GET /api/problem-types`**, **`GET /api/equipment-types`** — чтение
  справочников авторизованным (с `include_inactive` для ADMIN).
- **`PATCH /api/admin/holidays/{id}`** + `HolidayUpdateRequest` — правка даты
  праздника вместо удаления и создания заново (CAL-1); аддитивно к CRUD из Б1.
- **`Settings`** расширена полями `timezone`, `workdays`, `status_labels`,
  `status_colors`, `deadline_reminder_time`, `push_enabled` — они уже были в
  схеме БД и контракте Б1, блок лишь зафиксировал их в ответах Б7.

## 6. Б8 — Web Push

`c8d1b53 feat(b8): push subscriptions, VAPID sender and API`

- Снят `501` у `GET /api/push/public-key`, `POST /api/push/subscriptions`,
  `DELETE /api/push/subscriptions/{id}` — операции реализованы (NOTIF-1..7).
- Схемы `PushSubscriptionRequest`, `PushSubscriptionKeys`, `PushSubscription`
  не менялись (были в Б1).

## 7. Б9 — статистика

`39b9ff9`/`116e36e` (код), `docs/contract-diff.md` (этот документ)

- Контракт **не менялся**: `StatsResponse`, `StatsListResponse`, `StatsPeriod`,
  `StatsProblemType`, `StatsMonth` уже содержали все поля
  (`total_done`, `done_in_period`, `avg_resolution_hours`,
  `median_resolution_hours`, `on_time_percent`, `overdue_count`,
  `by_problem_type`, `by_month`).
- `GET /api/admin/stats` без `user_id` отдаёт `{items: StatsResponse[]}` по
  сотрудникам; поле `employees` **не вводилось** (см. §10).
- Добавлено только описание `StatsListResponse.items` — чтобы фронт не ждал
  `employees`.

## 8. Б12 — массовое перемещение

`cf34c7b feat(b12): wire bulk move route and contract`

- Снят `501` у `POST /api/equipment/bulk-move` (реализован, EQ-4).
- **`skipped_count`** в `EquipmentBulkMoveResponse` — аддитивное поле: единицы,
  уже находившиеся в целевом складе и пропущенные (записи журнала для них не
  создаются). `moved_count`/`movements` контракта сохранены, чтобы не ломать
  сгенерированные типы фронта.
- Описание операции дополнено: `404` с полным откатом при отсутствующей
  единице, `409 SAME_WAREHOUSE`, если все уже в целевом складе, `422` для
  USER/неактивного здания и границ набора 1..200.

## 9. Б10 и Б11 (смержены в main) — корзина и документы

- **Корзина (Б10):** `GET /api/admin/trash`, `POST /api/admin/trash/{type}/{id}/restore`,
  `DELETE /api/admin/trash/{type}/{id}`; в `TrashItem` зафиксированы
  `deleted_by_user_id`/`deleted_by_name`, причина восстановления и область
  окончательного удаления. `501` сняты после реализации.
- **Документы (Б11):** `GET /api/tickets/{id}/export.docx`,
  `GET /api/public/tickets/{token}/export.docx`, `GET /api/stats/export.xlsx`,
  `GET /api/warehouses/{id}/inventory.xlsx` — пути были в контракте с Б1, снят
  `501`, описания дополнены ограничениями (1000 позиций ведомости, права).
- Шаблоны документов описаны в `docs/templates.md` (что реально генерируется).

## 10. Б13 — финальная сверка контракта

- Сняты устаревшие `501` у **53** реализованных операций (50 в этом коммите +
  3 корзины после мержа Б10): пользователи, здания/склады, оборудование,
  база знаний, файлы, настройки/праздники, справочники, статистика, push,
  публичные справочники/настройки/дерево БЗ, документы, корзина.
  **Заглушек `501` в контракте не осталось** — все 87 операций реализованы.
- Сверка `routes.go` ↔ контракт: 87 зарегистрированных операций, все
  доменные пути присутствуют в контракте. Вне контракта осознанно остаются
  инфраструктурные `GET /api/health`, `/api/health/ready`, `/api/docs`,
  `/api/openapi.yaml` и `HEAD`-варианты файлов (OpenAPI описывает `GET`, HEAD
  следует из него по HTTP).
- Добавлено описание `StatsListResponse` (см. §11).
- Проверка: `make openapi` (офлайн-валидатор) — `64 путей, 87 операций,
  заглушек 501: 0`.

## 11. Чего в контракте нет и почему

| Ожидание | Фактическое решение |
|---|---|
| `employees` в сводной статистике | Б9 отдаёт `{items}`, контракт описывает `items` и явно говорит, что это сотрудники; дублирующее поле `employees` не вводилось. |
| `deleted_by` отдельным полем | Б10 фиксирует удаляющего как `deleted_by_user_id` + `deleted_by_name` в `TrashItem`; отдельное поле `deleted_by` не вводилось, чтобы не дублировать данные. |
| `by_user` в статистике | Группировка по сотрудникам реализована как список `items` (контракт это допускает); `by_user` не добавлялся. |
| Word/Excel/ведомость/корзина | Реализованы (Б11/Б10), `501` сняты; форматы и ограничения — в `docs/templates.md`. |
| Отдельный `/api/public/kb/files/{id}` | Не вводился: расширен существующий `GET /api/public/files/{id}` (см. §4), чтобы не дублировать проверку доступа. |

## 12. Регенерация типов фронта

Контракт — источник `frontend/src/api/types.ts`:

```bash
# из корня репозитория
npx openapi-typescript openapi/openapi.yaml -o frontend/src/api/types.ts
```

Типы генерируются, вручную не правятся. При добавлении полей (например,
`skipped_count`) старые поля остаются, поэтому регенерация не ломает текущие
экраны.
