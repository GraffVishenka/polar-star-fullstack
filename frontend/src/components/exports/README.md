# Экспорт документов (Ф8, DOC-1..DOC-4 / EQ-8 / STATS-3)

Общий механизм скачивания Word/Excel: `GET`→ blob → сохранение как файл с именем из
`Content-Disposition`. Внутри — `apiDownload` (Bearer/skipAuth, single-flight refresh,
таймаут), `saveBlob` (attachment), тосты с `request_id` для 5xx и русские тексты ошибок.

## Файлы

| Модуль                                         | Назначение                                                                                                             |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `useFileDownload`                              | Базовый хук: `download()`, `loading`, `error`, `filename`, `canDownload`; защита от дублей, таймаут, 403/404/422/5xx   |
| `download.ts`                                  | `saveBlob`, `filenameFromPath`, `DEFAULT_DOWNLOAD_TIMEOUT_MS`, маппинг 422                                             |
| `useInventoryExport` / `InventoryExportButton` | Ведомость помещения `GET /api/warehouses/{id}/inventory.xlsx` (EQ-8), 422 при >1000 позиций                            |
| `useStatsExport` / `StatsExportButton`         | Excel-статистика `GET /api/stats/export.xlsx` (STATS-3): `user_id`, `building_id`, `from`, `to`; 403 для чужого отчёта |
| `components/ExportButton`                      | Универсальная кнопка (используется Word-выгрузками заявок)                                                             |

Всё экспортируется из `@/components/exports`.

## Ведомость помещения — для fe-3 (`/equipment`, склады)

Страницу не создаём и не правим; подключение на странице помещения/склада:

```tsx
import { InventoryExportButton } from '@/components/exports';

// внутри карточки помещения, где есть warehouseId
<InventoryExportButton warehouseId={warehouseId} />;
```

Если нужен свой тулбар/меню — хук:

```tsx
const { download, loading, error, canDownload } = useInventoryExport(warehouseId, {
  onError: (e) => analytics.track('inventory_export_failed', e.status),
});

<Button loading={loading} disabled={!canDownload} onClick={() => void download()}>
  Ведомость помещения
</Button>;
```

Поведение: имя `inventory_{id}.xlsx` (или из заголовка), 422 → «В помещении больше
1000 позиций — ведомость не формируется, выгрузите по частям», 404 → «Помещение не
найдено», 403 → нет прав. Пустой `warehouseId` → `canDownload=false` и подсказка в
`InventoryExportButton`.

## Excel-статистика — для fe-2 (`/profile`, `/admin/stats`, Ф10)

```tsx
import { StatsExportButton } from '@/components/exports';

<StatsExportButton params={{ user_id: employeeId, building_id, from, to }} />;
```

Хук для своей кнопки/меню:

```tsx
const { download, loading, error } = useStatsExport({ from, to, building_id });
```

- `user_id` не указан → свой отчёт (доступен всем авторизованным);
- чужой `user_id` и `building_id` — только ADMIN, иначе 403 → «Чужой отчёт доступен
  только администратору»;
- период — `YYYY-MM-DD` по `completed_at`; имя файла приходит как
  `stat_{login}_{from}_{to}.xlsx`;
- 422 → «Слишком много данных для выгрузки — сократите период или выборку».

## Word-заявка (DOC-1, DOC-3)

Уже подключено в карточке заявки (`/tickets`, `TicketDrawer`) и на публичной
`/t/{token}`:

```tsx
// авторизованная карточка
<ExportButton path={ticketExportPath(ticket.id)} filename={`ticket_${ticket.id}.docx`} />

// публичная карточка (без Bearer)
<ExportButton path={publicTicketExportPath(token)} skipAuth filename={`ticket_${ticket.id}.docx`} />
```

Имя `ticket_{id}.docx` берётся из `Content-Disposition`, fallback — проп `filename`.
Ошибка/таймаут не создают файл; повторное нажатие во время запроса игнорируется;
5xx показывает тост с `request_id`.

## Тесты

`src/components/exports/__tests__/` — имя из `Content-Disposition` (UTF-8), таймаут,
сеть, 500 с `request_id`, дубли, пустой blob, 422 ведомости (>1000), 403 статистики,
параметры запроса статистики, Word из карточки заявки и с `/t/{token}`.
