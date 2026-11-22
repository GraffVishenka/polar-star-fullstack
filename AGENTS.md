# Правила командной разработки

Этот файл — рабочий контракт между агентами. Полное ТЗ: [PLAN.md](PLAN.md).

## Роли и зоны ответственности

| Агент | Роль | Владеет |
|---|---|---|
| `lead` | координатор | PLAN.md, openapi/openapi.yaml (финальная сверка), merge, интеграция, docs/HANDOVER.md |
| `be-1` | backend | `backend/internal/auth`, `backend/internal/users`, `backend/internal/tickets`, `backend/internal/notifications`, `backend/internal/documents`, `backend/cmd/api` |
| `be-2` | backend | `backend/internal/files`, `backend/internal/kb`, `backend/internal/stats`, `backend/internal/equipment` (bulk-move) |
| `be-3` | backend | `backend/internal/buildings`, `backend/internal/equipment` (базовый CRUD/move), `backend/internal/settings`, `backend/internal/dictionaries`, `backend/internal/trash` |
| `fe-1` | frontend | `frontend/src/app`, `src/api`, `src/auth`, `src/components` (общие), `src/pages/tickets`, `src/pages/public`, `src/pages/dashboard` |
| `fe-2` | frontend | `src/pages/kb`, `src/pages/admin/users`, `src/pages/admin/dictionaries`, `src/pages/admin/settings`, `src/pages/admin/stats`, `public/sw.js` |
| `fe-3` | frontend | `src/pages/equipment`, `src/pages/admin/buildings`, `src/pages/admin/trash`, `src/pages/profile` |

Пересечение каталогов запрещено. Общие файлы (`openapi.yaml`, `cmd/api/main.go`, `src/api/types.ts`, `src/router`) меняются только аддитивно, по одной строке/секции, и только после согласования с `lead`.

## Ветки и коммиты

- `main` — только стабильные, собранные состояния.
- Каждый блок — отдельная ветка от актуального `main`:
  - back: `feat/back-b1-core`, `feat/back-b2-users`, ... `feat/back-b13-integration`
  - front: `feat/front-f1-shell`, ... `feat/front-f11-e2e`
- Изоляция рабочих копий — git worktree в `.worktrees/<agent>` (каталог в `.gitignore`).
- Формат коммита: `feat(b5): tickets public dashboard` / `fix(b3): reject spoofed mime` / `test(f2): ticket board`.
- Один коммит — один логический шаг. Незавершённый код не коммитится в `main`.
- Важные блоки и функции комментируются: назначение, бизнес-правило из PLAN.md (например `// TICKET-7: USER видит только заявки своего здания`), неочевидные решения.

## Порядок волн

Backend: `Б1` → (`Б2`,`Б3`,`Б4`) → (`Б5`,`Б6`,`Б7`) → (`Б8`,`Б9`,`Б10`) → (`Б11`,`Б12`) → `Б13`.
Frontend: `Ф1` (после публикации контракта) → (`Ф2`,`Ф3`,`Ф4`) → (`Ф5`,`Ф6`,`Ф7`) → (`Ф8`,`Ф9`,`Ф10`) → `Ф11`.

Пока зависимость не готова, агент работает по MSW/заглушкам и не блокирует других.

## Контракт

- `openapi/openapi.yaml` — единственный источник истины. Изменения только аддитивные (новые поля/эндпоинты), с пометкой в сообщении коммита.
- `frontend/src/api/types.ts` генерируется из контракта, вручную не редактируется.
- Формат ошибки: `{"code","message","details","request_id"}`.
- Все даты и дедлайны считает сервер в `timezone`; клиент только отображает.

## Правила качества

- Back: `gofmt`, `go vet ./...`, `go test ./...`; бизнес-логика покрыта unit-тестами.
- Front: `tsc --noEmit`, `eslint`, `vitest run`; сборка `pnpm build`.
- Перед merge: `lead` проверяет diff, конфликты, компиляцию/сборку и обновляет PLAN.md при отклонениях.
- Секреты — только через env, в репозиторий не коммитятся.
