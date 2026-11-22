# Нагрузочный профиль k6 (Б13)

Профиль проверяет критерий PLAN §7: **200 RPS, p95 ≤ 300 мс, ошибок < 1%** на
чтении основных экранов.

## 1. Подготовка стенда

```bash
# стек + демо-данные (4 здания, 50 заявок, оборудование, БЗ)
make demo

# 10 000 заявок поверх демо-данных
docker compose exec -T mysql \
  mysql -upolar -ppolar polar_star < deploy/k6/seed-load.sql
```

`seed-load.sql` идемпотентен (маркер `[load]`), создаёт заявки в первом
активном здании во всех статусах с разными датами.

Проверить объём:

```bash
docker compose exec -T mysql \
  mysql -upolar -ppolar polar_star -e "SELECT COUNT(*) FROM tickets WHERE description LIKE '[load]%'"
```

## 2. Запуск

```bash
BASE_URL=http://localhost:8080 \
ADMIN_LOGIN=demo.admin1 ADMIN_PASSWORD=Demo_12345 \
  k6 run deploy/k6/load.js
```

Параметры окружения:

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `BASE_URL` | `http://localhost:8080` | адрес API (можно через nginx фронтенда) |
| `ADMIN_LOGIN` | `demo.admin1` | учётка для логина (ADMIN видит все заявки) |
| `ADMIN_PASSWORD` | `Demo_12345` | пароль демо-учёток |
| `HOLD_DURATION` | `5m` | сколько держать 200 RPS после разгона |

Профиль: 1 минута на 50 RPS → 4 минуты разгона до 200 RPS → 5 минут на 200 RPS.
Всего ~10 минут, ~100k запросов.

Сценарии (ротация по итерациям): `GET /api/tickets` (40 %), `/api/equipment`,
`/api/kb/tree`, `/api/stats/me?from&to`, `/api/public/settings`,
`/api/public/tickets`.

## 3. Ожидаемый результат

```
checks.........................: 100.00% ✓ 100k ✗ 0
http_req_failed................: 0.00%   ✓ 0     ✗ 0
http_req_duration..............: avg=... p(95)<300ms
```

Порог задан в `options.thresholds`; при превышении k6 завершится с кодом 1.
Если тест падает на `login`, проверьте, что демо-данные засеяны:
`docker compose logs api | grep "demo seed"`.

## 4. Что смотреть при отклонениях

- `docker compose logs api` — 5xx, медленные запросы, ошибки MySQL.
- `SELECT COUNT(*) FROM tickets` и план `EXPLAIN` для `GET /api/tickets`:
  индексы `idx_tickets_building_status_created`, `idx_tickets_executor_status`.
- k6 `--summary-export=summary.json` для разбора p95 по тегам
  (`http_req_duration{name:tickets}`).
- Стенд с 10k заявок — не production-объём; для реального профиля увеличьте
  `seed_load_tickets(100000)` и `maxVUs`.

Тест не входит в CI (нужен поднятый стек и 10k строк); в CI собираются образы и
крутятся unit/integration-тесты.
