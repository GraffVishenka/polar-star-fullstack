#!/usr/bin/env bash
# Бэкап БД и медиа (Б13, NFR-5, PLAN §9 п. 11).
#
# Что делает:
#   1. mysqldump базы в сжатый файл (--single-transaction, без блокировок);
#   2. tar.gz каталога загрузок (медиа заявок и БЗ);
#   3. кладёт sha256-контрольные суммы рядом;
#   4. удаляет бэкапы старше RETENTION_DAYS (по умолчанию 7).
#
# Использование:
#   deploy/backup.sh                       # локальный MySQL (mysqldump в PATH)
#   BACKUP_MODE=compose deploy/backup.sh   # MySQL внутри docker compose
#   BACKUP_MODE=compose COMPOSE_FILE=deploy/docker-compose.prod.yml deploy/backup.sh
#
# Переменные окружения:
#   BACKUP_DIR        каталог бэкапов (по умолчанию ./backups)
#   RETENTION_DAYS    сколько дней хранить (по умолчанию 7)
#   MYSQL_DATABASE    имя БД (polar_star)
#   MYSQL_USER        пользователь (polar)
#   MYSQL_PASSWORD    пароль
#   MYSQL_HOST        хост (127.0.0.1)
#   MYSQL_PORT        порт (3306)
#   UPLOAD_DIR        каталог медиа (по умолчанию ./data/uploads)
#   COMPOSE_SERVICE   имя сервиса MySQL в compose (mysql)
#
# Восстановление:
#   gunzip -c backups/db-YYYYmmdd-HHMMSS.sql.gz | mysql -u"$MYSQL_USER" -p"$MYSQL_DATABASE"
#   tar -xzf backups/uploads-YYYYmmdd-HHMMSS.tar.gz -C /path/to/data
#
# Скрипт намеренно не удаляет старые бэкапы, если не удалось создать новый:
# ротация выполняется только после успешного бэкапа.

set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"
MYSQL_DATABASE="${MYSQL_DATABASE:-polar_star}"
MYSQL_USER="${MYSQL_USER:-polar}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-}"
MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
UPLOAD_DIR="${UPLOAD_DIR:-./data/uploads}"
BACKUP_MODE="${BACKUP_MODE:-direct}"
COMPOSE_SERVICE="${COMPOSE_SERVICE:-mysql}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"

log() { printf '[backup] %s\n' "$*" >&2; }
die() { printf '[backup] ОШИБКА: %s\n' "$*" >&2; exit 1; }

command -v tar >/dev/null 2>&1 || die "не найден tar"
command -v gzip >/dev/null 2>&1 || die "не найден gzip"

mkdir -p "$BACKUP_DIR"
STAMP="$(date +%Y%m%d-%H%M%S)"
DB_FILE="$BACKUP_DIR/db-$STAMP.sql.gz"
UPLOADS_FILE="$BACKUP_DIR/uploads-$STAMP.tar.gz"
TMP_DB="$DB_FILE.part"
TMP_UPLOADS="$UPLOADS_FILE.part"
trap 'rm -f "$TMP_DB" "$TMP_UPLOADS"' EXIT

# --- 1. Дамп базы ---------------------------------------------------------
log "дамп базы $MYSQL_DATABASE ($BACKUP_MODE)"
if [ "$BACKUP_MODE" = "compose" ]; then
  command -v docker >/dev/null 2>&1 || die "BACKUP_MODE=compose, но docker не найден"
  docker compose -f "$COMPOSE_FILE" exec -T "$COMPOSE_SERVICE" \
    sh -c 'exec mysqldump --single-transaction --quick --no-tablespaces \
      --default-character-set=utf8mb4 -u"$0" -p"$1" "$2"' \
    "$MYSQL_USER" "$MYSQL_PASSWORD" "$MYSQL_DATABASE" | gzip -9 > "$TMP_DB"
else
  command -v mysqldump >/dev/null 2>&1 || die "не найден mysqldump (или задайте BACKUP_MODE=compose)"
  MYSQL_PWD="$MYSQL_PASSWORD" mysqldump \
    --host="$MYSQL_HOST" --port="$MYSQL_PORT" \
    --single-transaction --quick --no-tablespaces \
    --default-character-set=utf8mb4 \
    -u"$MYSQL_USER" "$MYSQL_DATABASE" | gzip -9 > "$TMP_DB"
fi
[ -s "$TMP_DB" ] || die "дамп базы пуст — бэкап не создан"
mv "$TMP_DB" "$DB_FILE"
log "база: $DB_FILE"

# --- 2. Медиа -------------------------------------------------------------
if [ -d "$UPLOAD_DIR" ]; then
  log "архив медиа $UPLOAD_DIR"
  tar -czf "$TMP_UPLOADS" -C "$(dirname "$UPLOAD_DIR")" "$(basename "$UPLOAD_DIR")"
  [ -s "$TMP_UPLOADS" ] || die "архив медиа пуст — бэкап не создан"
  mv "$TMP_UPLOADS" "$UPLOADS_FILE"
  log "медиа: $UPLOADS_FILE"
else
  log "каталог медиа $UPLOAD_DIR отсутствует — пропуск"
fi

# --- 3. Контрольные суммы --------------------------------------------------
( cd "$BACKUP_DIR" && sha256sum "$(basename "$DB_FILE")" > "$(basename "$DB_FILE").sha256" )
if [ -f "$UPLOADS_FILE" ]; then
  ( cd "$BACKUP_DIR" && sha256sum "$(basename "$UPLOADS_FILE")" > "$(basename "$UPLOADS_FILE").sha256" )
fi

# --- 4. Ротация ------------------------------------------------------------
log "ротация: удаляю файлы старше $RETENTION_DAYS дней"
find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'db-*.sql.gz' -o -name 'uploads-*.tar.gz' \
  -o -name 'db-*.sql.gz.sha256' -o -name 'uploads-*.tar.gz.sha256' \) \
  -mtime "+$RETENTION_DAYS" -print -delete

log "готово: $(ls -1 "$BACKUP_DIR" | wc -l) файл(ов) в $BACKUP_DIR"
