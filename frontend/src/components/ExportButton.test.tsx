/**
 * ExportButton: скачивание файла, состояния и ошибки (DOC-3, Ф8).
 * Проверяем, что повторный клик не создаёт дублей и что 422 объясняется.
 */
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { registerToastApi } from '@/app/toast';
import { server } from '@/mocks/server';
import { ExportButton, filenameFromPath } from './ExportButton';

const DOCX = 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';

function makeToastSpy() {
  const toastApi = {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  };
  registerToastApi(toastApi);
  return toastApi;
}

describe('filenameFromPath', () => {
  it('достаёт имя файла из пути', () => {
    expect(filenameFromPath('/tickets/12/export.docx')).toBe('export.docx');
    expect(filenameFromPath('/warehouses/3/inventory.xlsx?x=1')).toBe('inventory.xlsx');
    expect(filenameFromPath('/tickets')).toBe('download');
  });
});

describe('ExportButton', () => {
  beforeEach(() => {
    Object.defineProperty(URL, 'createObjectURL', {
      writable: true,
      value: vi.fn(() => 'blob:mock-url'),
    });
    Object.defineProperty(URL, 'revokeObjectURL', { writable: true, value: vi.fn() });
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  });

  it('скачивает файл и берёт имя из Content-Disposition', async () => {
    const toastApi = makeToastSpy();
    const onSuccess = vi.fn();

    server.use(
      http.get('*/api/tickets/1/export.docx', () =>
        new HttpResponse(new Blob(['docx-bytes']), {
          status: 200,
          headers: { 'Content-Type': DOCX, 'Content-Disposition': 'attachment; filename="ticket_1.docx"' },
        }),
      ),
    );

    render(<ExportButton path="/tickets/1/export.docx" label="Скачать в Word" onSuccess={onSuccess} />);
    await userEvent.click(screen.getByTestId('export-button'));

    await waitFor(() => expect(onSuccess).toHaveBeenCalledWith('ticket_1.docx'));
    expect(HTMLAnchorElement.prototype.click).toHaveBeenCalledTimes(1);
    expect(toastApi.success).toHaveBeenCalledWith('Файл готов', 'ticket_1.docx');
  });

  it('объясняет 422 «слишком много позиций» и обнуляет состояние загрузки', async () => {
    const toastApi = makeToastSpy();
    const onError = vi.fn();

    server.use(
      http.get('*/api/warehouses/1/inventory.xlsx', () =>
        HttpResponse.json(
          { code: 'VALIDATION_ERROR', message: 'too many', details: null, request_id: 'r-422' },
          { status: 422 },
        ),
      ),
    );

    render(
      <ExportButton
        path="/warehouses/1/inventory.xlsx"
        label="Ведомость"
        onError={onError}
      />,
    );
    await userEvent.click(screen.getByTestId('export-button'));

    await waitFor(() => expect(onError).toHaveBeenCalledTimes(1));
    expect(toastApi.error).toHaveBeenCalledWith(
      'Слишком много позиций для выгрузки — сократите выборку',
    );
    expect(screen.getByTestId('export-button')).not.toHaveClass('ant-btn-loading');
  });

  it('игнорирует повторный клик, пока запрос выполняется', async () => {
    makeToastSpy();
    let calls = 0;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });

    server.use(
      http.get('*/api/tickets/2/export.docx', async () => {
        calls += 1;
        await gate;
        return new HttpResponse(new Blob(['x']), {
          status: 200,
          headers: { 'Content-Type': DOCX },
        });
      }),
    );

    render(<ExportButton path="/tickets/2/export.docx" label="Word" />);
    const button = screen.getByTestId('export-button');

    const first = userEvent.click(button);
    await userEvent.click(button);
    await userEvent.click(button);

    expect(calls).toBe(1);

    release();
    await first;
    await waitFor(() => expect(calls).toBe(1));
  });
});
