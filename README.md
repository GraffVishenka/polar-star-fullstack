# Заявки · База знаний · Оборудование

Внутреннее веб-приложение: учёт заявок, база знаний и учёт оборудования с авторизацией
(`USER`, `ADMIN`, `anonymous`), Admin-панелью, push-уведомлениями и печатью документов.

- Полное техническое задание: [PLAN.md](PLAN.md)
- Backend: Go + MySQL 8 (`backend/`)
- Frontend: React 19 + TypeScript + Ant Design 5 (`frontend/`)
- Контракт API: `openapi/openapi.yaml`
- Документация: `docs/` (`HANDOVER.md`, `templates.md`, `decisions.md`)

## Быстрый старт

```bash
docker compose up -d --build
make seed        # демо-данные
```

- API: http://localhost:8080
- Swagger UI: http://localhost:8080/docs
- Frontend: http://localhost:5173 (dev) / http://localhost:8081 (docker)

## Разработка

Репозиторий ведётся по волнам из PLAN.md: каждая крупная функция — отдельная ветка
`feat/...`, изменения коммитятся, важные блоки и функции снабжаются комментариями.
