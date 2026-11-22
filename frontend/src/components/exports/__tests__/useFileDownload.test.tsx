/**
 * `useFileDownload` — общий механизм скачивания (DOC-3, Ф8):
 * имя из Content-Disposition, таймаут, сеть, 5xx с request_id, отсутствие дублей
 * и «пустых» файлов.
 */
import { act, renderHook } from '@testing-library/react';
import { HttpResponse, delay, http } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { registerToastApi } from '@/app/toast';
import { server } from '@/mocks/server';
import { useFileDownload } from '../useFileDownload';

const DOCX = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';

const downloads: string[] = [];
let toast: {
  success: ReturnType<typeof vi.fn>;
  error: ReturnType<typeof vi.fn>;
  warning: ReturnType<typeof vi.fn>;
  info: ReturnType<typeof vi.fn>;
};

beforeEach(() => {
  downloads.length = 0;
  Object.defineProperty(URL, 'createObjectURL', {
    writable: true,
    value: vi.fn(() => 'blob:mock-file'),
  });
  Object.defineProperty(URL, 'revokeObjectURL', { writable: true, value: vi.fn() });
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    downloads.push(this.download);
  });
  toast = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() };
  registerToastApi(toast);
});

function docxResponse(body = 'docx-bytes', headers: Record<string, string> = {}) {
  return new HttpResponse(new Blob([body]), {
    status: 200,
    headers: { 'Content-Type': DOCX, 'Cache-Control': 'no-store', ...headers },
  });
}

describe('useFileDownload', () => {
  it('берёт имя из Content-Disposition (UTF-8), сохраняет файл и показывает тост', async () => {
    server.use(
      http.get('*/api/exports/report.docx', () =>
        docxResponse('word', {
          'Content-Disposition':
            "attachment; filename*=UTF-8''%D0%B7%D0%B0%D1%8F%D0%B2%D0%BA%D0%B0.docx",
        }),
      ),
    );

    const { result } = renderHook(() => useFileDownload({ path: '/exports/report.docx' }));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error).toBeNull();
    expect(result.current.filename).toBe('заявка.docx');
    expect(downloads).toEqual(['заявка.docx']);
    expect(toast.success).toHaveBeenCalledWith('Файл готов', 'заявка.docx');
  });

  it('сообщает о таймауте и не создаёт файл', async () => {
    server.use(
      http.get('*/api/exports/slow.docx', async () => {
        await delay(300);
        return docxResponse();
      }),
    );

    const { result } = renderHook(() =>
      useFileDownload({ path: '/exports/slow.docx', timeoutMs: 50 }),
    );
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.code).toBe('TIMEOUT');
    expect(downloads).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith('Сервер не ответил вовремя');
  });

  it('сетевой сбой не оставляет «пустой» файл', async () => {
    server.use(http.get('*/api/exports/offline.docx', () => HttpResponse.error()));

    const { result } = renderHook(() => useFileDownload({ path: '/exports/offline.docx' }));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.isNetworkError).toBe(true);
    expect(downloads).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith('Нет связи с сервером');
  });

  it('для 5xx показывает тост с request_id', async () => {
    server.use(
      http.get('*/api/exports/broken.docx', () =>
        HttpResponse.json(
          { code: 'INTERNAL_ERROR', message: 'panic', details: null, request_id: 'req-500' },
          { status: 500 },
        ),
      ),
    );

    const { result } = renderHook(() => useFileDownload({ path: '/exports/broken.docx' }));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.status).toBe(500);
    expect(downloads).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith(
      'Ошибка сервера, попробуйте позже',
      'Код запроса: req-500',
    );
  });

  it('404 объясняется и не создаёт файл', async () => {
    server.use(
      http.get('*/api/exports/missing.docx', () =>
        HttpResponse.json(
          { code: 'NOT_FOUND', message: 'gone', details: null, request_id: 'req-404' },
          { status: 404 },
        ),
      ),
    );

    const { result } = renderHook(() => useFileDownload({ path: '/exports/missing.docx' }));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.status).toBe(404);
    expect(downloads).toHaveLength(0);
    expect(toast.error).toHaveBeenCalledWith('Запись не найдена');
  });

  it('пустой blob не превращается в файл', async () => {
    server.use(
      http.get(
        '*/api/exports/empty.docx',
        () =>
          new HttpResponse(null, {
            status: 200,
            headers: { 'Content-Type': DOCX, 'Content-Length': '0' },
          }),
      ),
    );

    const { result } = renderHook(() => useFileDownload({ path: '/exports/empty.docx' }));
    await act(async () => {
      await result.current.download();
    });

    expect(result.current.error?.status).toBe(500);
    expect(downloads).toHaveLength(0);
  });

  it('повторный вызов во время загрузки не дублирует запрос', async () => {
    let calls = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });

    server.use(
      http.get('*/api/exports/gated.docx', async () => {
        calls += 1;
        await gate;
        return docxResponse('ok', { 'Content-Disposition': 'attachment; filename="gated.docx"' });
      }),
    );

    const { result } = renderHook(() => useFileDownload({ path: '/exports/gated.docx' }));
    await act(async () => {
      const first = result.current.download();
      const second = result.current.download();
      release();
      await Promise.all([first, second]);
    });

    expect(calls).toBe(1);
    expect(downloads).toEqual(['gated.docx']);
  });

  it('не запускает запрос без пути (canDownload=false)', async () => {
    let calls = 0;
    server.use(
      http.get('*/api/never', () => {
        calls += 1;
        return docxResponse();
      }),
    );

    const { result } = renderHook(() => useFileDownload({ path: '' }));
    expect(result.current.canDownload).toBe(false);

    await act(async () => {
      await result.current.download();
    });

    expect(calls).toBe(0);
    expect(result.current.error).toBeNull();
  });
});
