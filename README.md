# Заявки · База знаний · Оборудование

Внутреннее веб-приложение: учёт заявок, база знаний и учёт оборудования с
авторизацией (`USER`, `ADMIN`, `anonymous`), Admin-панелью, push-уведомлениями,
статистикой и демо-данными для приёмки.

- Полное ТЗ: [PLAN.md](PLAN.md)
- Контракт API: [openapi/openapi.yaml](openapi/openapi.yaml) (источник истины)
- Передача проекта: [docs/HANDOVER.md](docs/HANDOVER.md)
- Решения: [docs/decisions.md](docs/decisions.md)
- История изменений контракта: [docs/contract-diff.md](docs/contract-diff.md)
- Ручная приёмка: [scripts/acceptance.md](scripts/acceptance.md)

## Быстрый старт

```bash
cp .env.example .env        # секреты: JWT_SECRET, SEED_ADMIN_PASSWORD, VAPID_*
make up                     # MySQL 8 + API + frontend (nginx)
make seed-demo              # настройки, справочники, ADMIN + демо-данные
```

- Frontend: <http://localhost:8081> (nginx проксирует `/api` на API)
- API: <http://localhost:8080>
- Swagger UI: <http://localhost:8080/docs>
- Health: `/api/health`, `/api/health/ready`

Демо-данные (`make demo` = `up` + `seed-demo`): 4 здания (1 неактивное),
12 пользователей, 8 складов, 50 заявок во всех статусах, 25 единиц оборудования
с журналом перемещений, 25 узлов БЗ (глубина 5+, 2 скрытых корня), 5 праздников.
Сид идемпотентен — повторный запуск не создаёт дубликаты.

### Тестовые учётки

| Логин | Пароль | Роль |
|---|---|---|
| `admin` | `SEED_ADMIN_PASSWORD` (по умолчанию в compose `ChangeMe_12345`) | ADMIN |
| `demo.admin1`, `demo.admin2` | `Demo_12345` (`SEED_DEMO_PASSWORD`) | ADMIN |
| `demo.user01` … `demo.user10` | `Demo_12345` | USER (по зданиям) |

Анонимная заявка — через публичную форму; ссылка `/t/{token}` выдаётся при
создании заявки.

## Разработка

```bash
# Backend
cd backend
go build ./...
go vet ./...
go test ./...            # в CI с MySQL; локально на Windows — scripts/go-test-wasm.ps1

# Frontend (MSW, без backend)
cd frontend
pnpm install
pnpm dev                 # http://localhost:5173
pnpm typecheck && pnpm lint && pnpm test -- --run && pnpm build
```

Проверки качества: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`
(в CI добавляются MySQL-интеграционные тесты), `make openapi` (валидация
контракта без сети).

## Тесты и приёмка

```bash
make test                # unit + интеграционные (нужен MySQL)
make e2e                 # API smoke на поднятом стеке (curl + python3)
make acceptance          # памятка ручной приёмки
docker compose exec -T mysql mysql -upolar -ppolar polar_star < deploy/k6/seed-load.sql
k6 run deploy/k6/load.js # 10k заявок, 50→200 RPS, p95 ≤ 300 мс
```

## Поставка

- `docker-compose.yml` — локальный стек (MySQL, API, frontend на 8081).
- `deploy/docker-compose.prod.yml` — production: наружу только фронтенд (80),
  секреты обязательны, TLS — на внешнем reverse proxy.
- `deploy/backup.sh` — mysqldump + tar медиа, sha256, ротация 7 дней (`make backup`).
- `deploy/k6/` — нагрузочный профиль и данные.
- CI (`.github/workflows/ci.yml`): backend (gofmt/vet/build/тесты с MySQL),
  frontend (typecheck/lint/test/build), контракт (валидатор + redocly),
  сборка Docker-образов без публикации.

## Документация

`docs/` — `HANDOVER.md` (как поднять и продолжить), `decisions.md` (ADR),
`contract-diff.md` (аддитивные изменения контракта), `ENVIRONMENT.md`
(ограничения локальной машины), `PROGRESS.md` (статус блоков), `templates.md`
(шаблоны Word/Excel для Б11).
