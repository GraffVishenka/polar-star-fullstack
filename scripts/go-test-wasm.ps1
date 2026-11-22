# Запуск unit-тестов Go через Node.js + WebAssembly.
# Причина: Smart App Control блокирует исполнение свежесобранных .exe,
# но Node.js разрешён. Чистая бизнес-логика (без MySQL/сети) тестируется так.
#
# Использование:
#   .\scripts\go-test-wasm.ps1                 # все пакеты
#   .\scripts\go-test-wasm.ps1 ./internal/tickets/...

param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Packages)

. (Join-Path $PSScriptRoot 'dev-env.ps1')

$root = Split-Path -Parent $PSScriptRoot
$wrapper = Join-Path $root '.tools\wasm_exec.bat'
$wasmDir = Join-Path $root '.tools\go\lib\wasm'

if (-not (Test-Path $wrapper)) {
    @"
@echo off
"$env:NODE_EXE" --stack-size=8192 "$wasmDir\wasm_exec_node.js" %*
"@ | Set-Content -Encoding ascii $wrapper
}

if (-not $Packages -or $Packages.Count -eq 0) { $Packages = @('./...') }

$env:GOOS = 'js'
$env:GOARCH = 'wasm'
Push-Location (Join-Path $root 'backend')
try {
    go test -exec $wrapper @Packages
    exit $LASTEXITCODE
} finally {
    Pop-Location
}
