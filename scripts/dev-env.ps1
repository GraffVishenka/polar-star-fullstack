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

# Общий тулчейн лежит в основной рабочей копии; worktree агентов используют его же.
$sharedTools = 'C:\tech_spec\polar-star-service\.tools'
$tools = if (Test-Path (Join-Path $repo '.tools\go\bin\go.exe')) { Join-Path $repo '.tools' } else { $sharedTools }

$goBin = Join-Path $tools 'go\bin'
$cacheRoot = Join-Path $repo '.cache'
if (-not (Test-Path $cacheRoot)) { New-Item -ItemType Directory -Force -Path $cacheRoot | Out-Null }

$env:Path = "$goBin;C:\Program Files\Git\cmd;$env:Path"
$env:GOPATH = Join-Path $tools 'gopath'
$env:GOMODCACHE = Join-Path $env:GOPATH 'pkg\mod'
$env:GOCACHE = Join-Path $cacheRoot 'go-build'
$env:GOTMPDIR = Join-Path $cacheRoot 'tmp'
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
$env:GOSUMDB = 'off'
$env:GOFLAGS = '-mod=mod'

$env:NODE_EXE = 'C:\Users\GraffVishenka\.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\node\bin\node.exe'
$env:PNPM_JS = 'C:\Users\GraffVishenka\.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\pnpm\bin\pnpm.mjs'
$env:PNPM_HOME = Join-Path $cacheRoot 'pnpm'
$env:NPM_CONFIG_STORE_DIR = Join-Path $cacheRoot 'pnpm-store'
$env:NPM_CONFIG_CACHE = Join-Path $cacheRoot 'npm-cache'

New-Item -ItemType Directory -Force -Path $env:GOPATH, $env:GOMODCACHE, $env:GOCACHE, $env:GOTMPDIR, $env:PNPM_HOME | Out-Null

function pnpm { & $env:NODE_EXE $env:PNPM_JS @args }
function nodejs { & $env:NODE_EXE @args }

Write-Host "dev-env: repo=$repo"
Write-Host "dev-env: go=$(go version)  node=$(& $env:NODE_EXE --version)"
