# Приёмка: команды и ожидаемые результаты

Проверка **без человека-разработчика**: выполните команды и сравните результат.
Полный чек-лист продукта — PLAN §7; здесь — воспроизводимый минимум и
автоматизированные проверки.

Зависимости: Docker + Compose, `curl`, `python3`, `make` (Linux/macOS/WSL;
на Windows — Git Bash/WSL).

## 0. Подготовка

```bash
cp .env.example .env
make up
make seed-demo
docker compose ps
```

Ожидается: `mysql`, `api`, `frontend` — `healthy` (у api start_period 20 c).
В логах `docker compose logs api`:

```
seed: создан первичный администратор        login=admin
demo seed: план                             ... зданий=4 ... заявок=50 ...
demo seed: готово                           users_created=12 tickets_created=50 ...
demo seed: пользователь                     login=demo.admin1 role=ADMIN ...
```

Учётные записи: `admin` (+ `SEED_ADMIN_PASSWORD`), `demo.admin1/2`,
`demo.user01…10` с паролем `Demo_12345` (`SEED_DEMO_PASSWORD`).

## 1. Автоматические проверки

```bash
make openapi
# ожидается: openapi: OK — 64 путей, 87 операций, ... заглушек 501: 0

cd backend && gofmt -l . && go vet ./... && go build ./...
# ожидается: пустой вывод gofmt, exit 0

make test          # в CI или на машине с MySQL; локально без MySQL часть тестов skip
make e2e           # API smoke на поднятом стеке
# ожидается: e2e: успешно 17, провалено 0
```

`make e2e` проверяет: health, логин ADMIN, публичные настройки/заявки/здания/
справочники, `/api/auth/me`, непустой список заявок, оборудование, дерево БЗ,
личную статистику, `assignable`, валидацию bulk-move (`422`), документы
(`export.docx`, `export.xlsx`) и корзину.

## 2. Публичный периметр (браузер: <http://localhost:8081>)

1. `/` — 4 здания, у трёх активных счётчики заявок; неактивное здание без
   кнопки создания заявки.
2. Выберите «Демо-корпус А» → дашборд здания; перезагрузите через 5 минут —
   выбор сохранён (cookie 30 дней).
3. «Новая заявка»: заполните поля, приложите фото ≤10 МБ → откроется карточка
   с бессрочной ссылкой `/t/{token}`; ссылка работает в приватном окне.
4. Поле дедлайна: дата = сегодня + 2 **рабочих** дня (Пн–Сб, без праздников из
   `GET /api/admin/holidays`).

CLI-эквивалент:

```bash
curl -s http://localhost:8080/api/public/settings
curl -s 'http://localhost:8080/api/public/tickets?building_id=1' | head -c 300
```

## 3. Роли и видимость заявок

```bash
# логин USER здания А
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"login":"demo.user01","password":"Demo_12345"}' | python3 -c 'import json,sys;print(json.load(sys.stdin)["access_token"])')

curl -s -H "Authorization: Bearer $TOKEN" 'http://localhost:8080/api/tickets?page=1&size=5' | head -c 300
# ожидается: только заявки «Демо-корпуса А»

# запрос чужого здания — 403
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/tickets?building_id=2'
# ожидается: 403
```

## 4. Дедлайн, календарь, часовой пояс

```bash
# праздники
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  'http://localhost:8080/api/admin/holidays?year=2026' | head -c 300

# настройки: deadline_days=2, workdays Пн–Сб, timezone Asia/Yekaterinburg
curl -s -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8080/api/admin/settings
```

Создайте заявку в пятницу — дедлайн должен перейти через воскресенье (и через
праздник, если он попал в интервал). Время напоминаний — `10:00` по
Екатеринбургу (`deadline_reminder_time`, `timezone`).

## 5. База знаний

1. `/kb` — дерево: 4 корня, из них 2 скрытых (`Демо: Скрытый раздел`,
   `Демо: Архив (скрытый)`).
2. Аноним в приватном окне: скрытых корней и их потомков нет.
3. `demo.user01`: видит все разделы; статья «Демо: Инструкции» открывается.
4. Медиа статьи (если приложено) открывается по `/api/public/files/{id}` без
   токена; файл не из публичной статьи — `404`.

```bash
curl -s http://localhost:8080/api/public/kb/tree | python3 -c \
 'import json,sys;d=json.load(sys.stdin);print("корней:",len(d))'
```

## 6. Оборудование и массовое перемещение

```bash
# список
curl -s -H "Authorization: Bearer $TOKEN" 'http://localhost:8080/api/equipment?page=1&size=5' | head -c 300

# журнал первой единицы
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/equipment/1/movements' | head -c 300

# массовое перемещение (id из списка, склад назначения — существующий)
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"equipment_ids":[1,2],"to_warehouse_id":2,"comment":"приёмка"}' \
  http://localhost:8080/api/equipment/bulk-move
# ожидается: {"moved_count":N,"skipped_count":M,"movements":[...]}

# пустой набор — 422
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{}' http://localhost:8080/api/equipment/bulk-move
# ожидается: 422
```

## 7. Статистика

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/stats/me?from=2026-01-01&to=2026-12-31' | head -c 400
# ожидается: total_done, done_in_period, avg/median/on_time (null при пустом
# периоде), by_problem_type[], by_month[]

curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/admin/stats
# ожидается: 403 (ADMIN-only)

curl -s -H "Authorization: Bearer $ADMIN_TOKEN" \
  'http://localhost:8080/api/admin/stats?building_id=1' | head -c 400
# ожидается: {"items":[...]} — по сотрудникам здания
```

## 8. Push (если заданы VAPID-ключи и HTTPS)

1. `GET /api/push/public-key` возвращает ключ.
2. В браузере разрешите уведомления; подписка появляется в
   `push_subscriptions`.
3. Создайте заявку в здании — push получают USER этого здания и ADMIN
   (только они).
4. Напоминание о дедлайне приходит в `deadline_reminder_time`; повторный запуск
   воркера не дублирует доставку (idempotency_key).

Без ключей push — no-op (события не создаются), API работает.

## 9. Бэкап и восстановление

```bash
make backup
ls -lh backups/
# ожидается: db-YYYYmmdd-HHMMSS.sql.gz, uploads-*.tar.gz и .sha256

# восстановление в отдельную БД
gunzip -c backups/db-*.sql.gz | mysql -u"$MYSQL_USER" -p"$MYSQL_DATABASE"
```

Ротация: файлы старше `RETENTION_DAYS` (7) удаляются после успешного бэкапа.

## 10. Нагрузка (k6)

```bash
docker compose exec -T mysql mysql -upolar -ppolar polar_star < deploy/k6/seed-load.sql
k6 run deploy/k6/load.js
# ожидается: http_req_failed < 1%, p(95) < 300 ms, checks > 99%
```

## 11. Документы и корзина (Б11, Б10)

```bash
# Word по заявке (ID возьмите из GET /api/tickets: items[0].id)
TICKET_ID=1
curl -s -o /tmp/ticket.docx -w '%{http_code} %{content_type}\n' \
  -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/tickets/$TICKET_ID/export.docx"
# ожидается: 200 application/vnd.openxmlformats-officedocument.wordprocessingml.document

# Excel статистики
curl -s -o /tmp/stats.xlsx -w '%{http_code} %{content_type}\n' \
  -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/stats/export.xlsx?from=2026-01-01&to=2026-12-31'

# Ведомость помещения (WAREHOUSE_ID из GET /api/warehouses: items[0].id)
WAREHOUSE_ID=1
curl -s -o /tmp/inventory.xlsx -w '%{http_code}\n' \
  -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/warehouses/$WAREHOUSE_ID/inventory.xlsx"

# Корзина: удалить заявку ADMIN'ом и проверить список/восстановление
curl -s -X DELETE -H "Authorization: Bearer $TOKEN" \
  -o /dev/null -w '%{http_code}\n' "http://localhost:8080/api/tickets/$TICKET_ID"
curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/admin/trash | head -c 400
curl -s -X POST -H "Authorization: Bearer $TOKEN" \
  -o /dev/null -w '%{http_code}\n' \
  "http://localhost:8080/api/admin/trash/ticket/$TICKET_ID/restore"
# ожидается: 204/200 (удаление), items с deleted_by_* в списке, восстановление 200
```

USER не может скачать чужой Word/Excel (`403`); аноним скачивает Word только по
токену своей заявки. Ограничение ведомости — 1000 позиций (`422` при
превышении).

## 12. Чек-лист «проект поднимается с нуля»

- [ ] `git clone` → `cp .env.example .env` → `make up` → `make seed-demo` — без
      ручных SQL.
- [ ] Миграции применяются с нуля и повторно (`make migrate` дважды — без ошибок).
- [ ] `make openapi`, `go vet`, `go build`, `make test` — зелёные.
- [ ] `make e2e` — 0 провалов.
- [ ] `make backup` создаёт два архива и контрольные суммы.
- [ ] `deploy/docker-compose.prod.yml` валиден (`docker compose -f ... config`).
