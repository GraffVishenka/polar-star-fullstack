#!/usr/bin/env bash
# Сквозная проверка API на поднятом стеке (Б13, `make e2e`).
#
# Проверяет, что собранный docker-стек отвечает, демо-данные засеяны и
# основные экраны отдают данные. Полноценный E2E в браузере (Playwright) —
# отдельная задача (см. HANDOVER, «Что осталось»).
#
# Требования: curl, python3. Запуск:
#   make demo            # стек + демо-данные
#   make e2e             # или ./scripts/e2e.sh
#
# Переменные: BASE_URL (http://localhost:8080), E2E_LOGIN/E2E_PASSWORD
# (по умолчанию admin/ChangeMe_12345 — как в docker-compose.yml).

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
BASE_URL="${BASE_URL%/}"
E2E_LOGIN="${E2E_LOGIN:-admin}"
E2E_PASSWORD="${E2E_PASSWORD:-ChangeMe_12345}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0
TOKEN=""

say() { printf '%s\n' "$*"; }
ok() { pass=$((pass + 1)); printf '  OK   %s\n' "$*"; }
bad() { fail=$((fail + 1)); printf '  FAIL %s\n' "$*"; }

# json_get <file> <python-выражение над data> — печатает значение или пусто.
json_get() {
  python3 - "$1" "$2" <<'PY' 2>/dev/null || true
import json, sys
try:
    data = json.load(open(sys.argv[1], encoding="utf-8"))
    value = eval(sys.argv[2], {"data": data})
    print("" if value is None else value)
except Exception:
    print("")
PY
}

# request <method> <path> <expected_status> <description> [body] [auth]
request() {
  local method="$1" path="$2" expected="$3" title="$4" body="${5:-}" auth="${6:-}"
  local args=(-sS -X "$method" -o "$TMP_DIR/body.json" -w '%{http_code}' "$BASE_URL$path")
  if [ -n "$auth" ]; then args+=(-H "Authorization: Bearer $TOKEN"); fi
  if [ -n "$body" ]; then args+=(-H 'Content-Type: application/json' -d "$body"); fi
  local status
  status="$(curl "${args[@]}" || true)"
  if [ "$status" = "$expected" ]; then
    ok "$title — HTTP $status"
  else
    bad "$title — HTTP $status, ожидался $expected ($(head -c 200 "$TMP_DIR/body.json" 2>/dev/null))"
  fi
}

say "e2e: $BASE_URL"

# 1. Готовность API (до 60 секунд).
ready=""
for _ in $(seq 1 30); do
  if curl -fsS "$BASE_URL/api/health" >/dev/null 2>&1; then ready=1; break; fi
  sleep 2
done
if [ -n "$ready" ]; then ok "GET /api/health"; else
  bad "GET /api/health — стек не отвечает, выполните make demo"
  say "e2e: провалено $fail, успешно $pass"
  exit 1
fi

# 2. Логин (ADMIN по умолчанию).
status="$(curl -sS -X POST "$BASE_URL/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login\":\"$E2E_LOGIN\",\"password\":\"$E2E_PASSWORD\"}" \
  -o "$TMP_DIR/login.json" -w '%{http_code}' || true)"
if [ "$status" = "200" ]; then
  TOKEN="$(json_get "$TMP_DIR/login.json" 'data["access_token"]')"
  if [ -n "$TOKEN" ]; then ok "POST /api/auth/login ($E2E_LOGIN)"; else bad "логин: нет access_token"; fi
else
  bad "POST /api/auth/login — HTTP $status (учётка $E2E_LOGIN; демо: E2E_LOGIN=demo.admin1)"
  say "e2e: провалено $fail, успешно $pass"
  exit 1
fi

# 3. Публичный периметр.
request GET /api/public/settings 200 "GET /api/public/settings"
request GET /api/public/tickets 200 "GET /api/public/tickets"
request GET "/api/public/buildings" 200 "GET /api/public/buildings"
request GET /api/public/problem-types 200 "GET /api/public/problem-types"

# 4. Авторизованные экраны.
request GET /api/auth/me 200 "GET /api/auth/me" "" auth
request GET "/api/tickets?page=1&size=5" 200 "GET /api/tickets" "" auth
total="$(json_get "$TMP_DIR/body.json" 'data.get("total")')"
if [ "${total:-0}" -gt 0 ]; then ok "в списке заявок есть данные (total=$total)"; else bad "список заявок пуст — нужны демо-данные (make seed-demo)"; fi
request GET "/api/equipment?page=1&size=5" 200 "GET /api/equipment" "" auth
request GET /api/kb/tree 200 "GET /api/kb/tree" "" auth
request GET /api/stats/me 200 "GET /api/stats/me" "" auth
request GET "/api/users/assignable" 200 "GET /api/users/assignable" "" auth

# 5. Валидация массового перемещения: пустой набор — 422.
request POST /api/equipment/bulk-move 422 "POST /api/equipment/bulk-move (пусто → 422)" '{}' auth

# 6. Нереализованные операции остаются 501 (Б10/Б11).
request GET /api/admin/trash 501 "GET /api/admin/trash (Б10 → 501)" "" auth
request GET /api/stats/export.xlsx 501 "GET /api/stats/export.xlsx (Б11 → 501)" "" auth

say "e2e: успешно $pass, провалено $fail"
[ "$fail" -eq 0 ]
