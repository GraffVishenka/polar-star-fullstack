# Ограничения окружения разработки

Эта рабочая машина (Windows 11) имеет включённый **Smart App Control** и не имеет
Docker/MySQL/WSL. Это влияет на то, что можно проверить локально.

## Что работает

| Инструмент | Статус | Путь |
|---|---|---|
| Git | работает | `C:\Program Files\Git\cmd\git.exe` |
| Go 1.27.4 | компилятор работает | `.tools\go\bin\go.exe` |
| Node.js 24 | работает | `.dsh`-runtime |
| pnpm 11 | работает | через `node pnpm.mjs` |
| `go build ./...` | работает | проверка типов и сборка |
| `go vet ./...` | работает | статический анализ |
| unit-тесты Go | через Node+wasm | `.\scripts\go-test-wasm.ps1` |

## Что заблокировано

- **Запуск свежесобранных `.exe`** (включая `go test`-бинарники и `api.exe`) —
  Smart App Control блокирует неподписанные исполняемые файлы.
- **MySQL/Docker** отсутствуют, поэтому интеграционные тесты с БД локально не идут.
- **`go run`/`go test` без wasm** падают на этапе запуска бинарника.

## Как проверять backend локально

```powershell
. .\scripts\dev-env.ps1
cd backend
gofmt -l .
go vet ./...
go build ./...
cd ..
.\scripts\go-test-wasm.ps1 ./internal/...   # чистая логика
```

Интеграционные тесты и запуск API выполняются в CI (GitHub Actions, Ubuntu +
MySQL service) или на машине без Smart App Control:

```bash
docker compose up -d --build
make migrate seed
make test
```

## Как проверять frontend локально

```powershell
. .\scripts\dev-env.ps1
cd frontend
pnpm install
pnpm typecheck
pnpm lint
pnpm test -- --run
pnpm build
pnpm dev        # MSW-моки, без backend
```

## Следствия для приёмки

- Сценарии «в браузере против реального API» проверяются в CI/на целевой машине.
- Локально frontend проверяется против MSW, backend — компиляцией, `vet` и
  wasm-тестами доменной логики.
