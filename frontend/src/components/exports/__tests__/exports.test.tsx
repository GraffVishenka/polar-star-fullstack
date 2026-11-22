/**
 * Экспортёры документов (Ф8): ведомость помещения (EQ-8) и Excel-статистика (STATS-3).
 * Проверяем имена файлов, параметры запроса, 422 (>1000 позиций) и 403.
 */
import { act, render, renderHook, screen } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { registerToastApi } from '@/app/toast';
import { db } from '@/mocks/db';
import { server } from '@/mocks/server';
import { authenticateAs } from '@/test/utils';
import { InventoryExportButton } from '../InventoryExportButton';
import { useInventoryExport } from '../useInventoryExport';
import { useStatsExport } from '../useStatsExport';

const XLSX = 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';

const downloads: string[] = [];
let toast: {
  success: ReturnType<typeof vi.fn>;
  error: ReturnType<typeof vi.fn>;
  warning: ReturnType<typeof vi.fn>;
  info: ReturnType<typeof vi.fn>;
};

function xlsxResponse(filename: string) {
  return new HttpResponse(new Blob(['xlsx-demo']), {
    status: 200,
    headers: {
      'Content-Type': XLSX,
      'Cache-Control': 'no-store',
      'Content-Disposition': `attachment; filename="${filename}"`,
    },
  });
}

beforeEach(() => {
  downloads.length = 0;
  Object.defineProperty(URL, 'createObjectURL', {
    writable: true,
    value: vi.fn(() => 'blob:mock-xlsx'),
  });
  Object.defineProperty(URL, 'revokeObjectURL', { writable: true, value: vi.fn() });
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    downloads.push(this.download);
  });
  toast = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() };
  registerToastApi(toast);
  authenticateAs(2);
});

describe('useInventoryExport (EQ-8)', () => {
  it('скачивает ведомость помещения с именем из заголовка', async () => {
    server.use(
      http.get('*/api/warehouses/2/inventory.xlsx', () => xlsxResponse('inventory_2.xlsx')),
    );

    const { result } = renderHook(() => useInventoryExport(2));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error).toBeNull();
    expect(result.current.filename).toBe('inventory_2.xlsx');
    expect(downloads).toEqual(['inventory_2.xlsx']);
    expect(toast.success).toHaveBeenCalledWith('Ведомость готова', 'inventory_2.xlsx');
  });

  it('объясняет 422 при >1000 позиций в помещении', async () => {
    // Сид: в помещении 1 две единицы; добиваем до 1001 — сервер отказывает (EQ-8).
    for (let index = 0; index < 999; index += 1) {
      db.equipment.push({
        id: 10_000 + index,
        name: `Единица ${index}`,
        equipment_type_id: 1,
        serial_number: `SN-${index}`,
        registration_number: `REG-${index}`,
        warehouse_id: 1,
        created_at: '2026-01-01T00:00:00Z',
        deleted_at: null,
      });
    }

    const { result } = renderHook(() => useInventoryExport(1));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.status).toBe(422);
    expect(downloads).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('больше 1000 позиций'));
  });

  it('без выбранного помещения выгрузка недоступна', () => {
    const { result } = renderHook(() => useInventoryExport(null));

    expect(result.current.canDownload).toBe(false);
    expect(result.current.warehouseId).toBeNull();
  });

  it('InventoryExportButton активна только при валидном warehouseId', () => {
    const { rerender } = render(<InventoryExportButton warehouseId={null} />);
    expect(screen.getByTestId('export-button')).toBeDisabled();

    rerender(<InventoryExportButton warehouseId={3} />);
    expect(screen.getByTestId('export-button')).toBeEnabled();
    expect(screen.getByTestId('export-button')).toHaveTextContent('Ведомость помещения');
  });
});

describe('useStatsExport (STATS-3)', () => {
  it('передаёт user_id/building_id/период и берёт имя из Content-Disposition', async () => {
    let captured: URLSearchParams | null = null;
    server.use(
      http.get('*/api/stats/export.xlsx', ({ request }) => {
        captured = new URL(request.url).searchParams;
        return xlsxResponse('stat_user_2026-01-01_2026-01-31.xlsx');
      }),
    );

    const { result } = renderHook(() =>
      useStatsExport({
        user_id: 2,
        building_id: 1,
        from: '2026-01-01',
        to: '2026-01-31',
      }),
    );
    await act(async () => {
      await result.current.download();
    });

    expect(captured).not.toBeNull();
    expect(captured!.get('user_id')).toBe('2');
    expect(captured!.get('building_id')).toBe('1');
    expect(captured!.get('from')).toBe('2026-01-01');
    expect(captured!.get('to')).toBe('2026-01-31');
    expect(result.current.filename).toBe('stat_user_2026-01-01_2026-01-31.xlsx');
    expect(toast.success).toHaveBeenCalledWith(
      'Файл статистики готов',
      'stat_user_2026-01-01_2026-01-31.xlsx',
    );
  });

  it('чужой отчёт для USER — 403 с понятным текстом', async () => {
    // MSW: USER (id 2) не может выгрузить отчёт user_id=3 (STATS-3).
    const { result } = renderHook(() => useStatsExport({ user_id: 3 }));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.status).toBe(403);
    expect(downloads).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith('Чужой отчёт доступен только администратору');
  });

  it('422 при слишком большой выборке объясняется', async () => {
    // MSW валидирует обратный период (from > to) — реальный 422 от «сервера».
    const { result } = renderHook(() => useStatsExport({ from: '2026-12-31', to: '2026-01-01' }));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.status).toBe(422);
    expect(downloads).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith(
      'Слишком много данных для выгрузки — сократите период или выборку',
    );
  });
});
