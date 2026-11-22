# Продолжение после перезагрузки

Перезагрузка нужна, потому что компоненты WSL2 включены, но не активированы
(`RestartNeeded=True`). SAC не отключается.

## Что уже сделано

- Node.js 24.22.3, npm 11.19.1, pnpm 11.28.5 установлены в
  `%LOCALAPPDATA%\Programs\nodejs` и добавлены в пользовательский PATH.
- Frontend: 359 unit-тестов — зелёные; Go-тесты в WASM — все пакеты `ok`.
- WSL-компоненты включены (нужна перезагрузка), Docker/MySQL ещё нет.

## После входа в Windows

1. Открыть DSH и продолжить эту сессию (написать «продолжай»).
2. Агент выполнит:
   ```powershell
   wsl --install -d Ubuntu --no-launch     # с правами администратора
   wsl --update
   wsl -d Ubuntu -u root -- bash -c "apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y mysql-server curl ca-certificates"
   ```
3. Внутри Ubuntu: установить Go (Linux tarball), поднять MySQL, создать БД
   `polar_star` и пользователя `polar/polar`.
4. Запустить backend:
   ```bash
   cd /mnt/c/tech_spec/polar-star-service/backend
   DATABASE_URL='polar:polar@tcp(127.0.0.1:3306)/polar_star?parseTime=true&multiStatements=true' \
   JWT_SECRET='dev-only-secret-change-me-please-32' \
   TZ='Asia/Yekaterinburg' \
   go test ./...
   go run ./cmd/api migrate
   go run ./cmd/api seed --demo
   go run ./cmd/api serve
   ```
5. Frontend на Windows — **без MSW**, с прокси на `localhost:8080`:
   ```powershell
   cd frontend
   $env:VITE_ENABLE_MSW='false'
   pnpm dev
   ```
6. Проверить: <http://127.0.0.1:5173>, API <http://127.0.0.1:8080/health>,
   Swagger <http://127.0.0.1:8080/docs>.

## Альтернатива (Docker)

Если Ubuntu-путь не сработает: установить Docker Engine внутри Ubuntu
(`apt-get install -y docker.io docker-compose-v2`), затем
`docker compose up -d --build` из корня репозитория (путь `/mnt/c/...`).
