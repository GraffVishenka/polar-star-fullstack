# Единое окружение разработки для Windows (DSH-песочница).
# Использование:  . .\scripts\dev-env.ps1
#
# ВАЖНО: на этой машине включён Smart App Control, поэтому свежесобранные
# .exe блокируются. Go-компилятор разрешён, но сервер и тестовые бинарники
# запускать нельзя. Для проверки используем:
#   go build ./...                 — компиляция
#   go vet ./...                   — статический анализ
#   .\scripts\go-test-wasm.ps1     — unit-тесты чистой логики через Node+wasm

$repo = Split-Path -Parent $PSScriptRoot
$tools = Join-Path $repo '.tools'
$goBin = Join-Path $tools 'go\bin'

$env:Path = "$goBin;C:\Program Files\Git\cmd;$env:Path"
$env:GOPATH = Join-Path $tools 'gopath'
$env:GOMODCACHE = Join-Path $env:GOPATH 'pkg\mod'
$env:GOCACHE = Join-Path $tools 'gocache'
$env:GOTMPDIR = Join-Path $tools 'tmp'
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
# Корпоративный/локальный прокси не используется; sumdb отключён, т.к. кеш
# суммы лежит вне разрешённой песочницей области.
$env:GOSUMDB = 'off'
$env:GOFLAGS = '-mod=mod'

$env:NODE_EXE = 'C:\Users\GraffVishenka\.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\node\bin\node.exe'
$env:PNPM_JS = 'C:\Users\GraffVishenka\.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\pnpm\bin\pnpm.mjs'

New-Item -ItemType Directory -Force -Path $env:GOPATH, $env:GOMODCACHE, $env:GOCACHE, $env:GOTMPDIR | Out-Null

function pnpm { & $env:NODE_EXE $env:PNPM_JS @args }
function nodejs { & $env:NODE_EXE @args }

Write-Host "dev-env: go=$(go version)  node=$(& $env:NODE_EXE --version)  repo=$repo"
