/**
 * Word-выгрузка заявки (DOC-1, DOC-3, Ф8): кнопка в карточке `/tickets` и на
 * публичной `/t/{token}`, имя `ticket_{id}.docx`, ошибка без «пустого» файла.
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import { Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ExportButton } from '@/components/ExportButton';
import { PublicTicketPage } from '@/pages/public/PublicTicketPage';
import { TicketsPage } from '@/pages/tickets/TicketsPage';
import { registerToastApi } from '@/app/toast';
import { server } from '@/mocks/server';
import {
  ADMIN_AUTH,
  authenticateAs,
  makeAnonymousAuthValue,
  makeAuthValue,
  renderWithProviders,
} from '@/test/utils';

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
    value: vi.fn(() => 'blob:mock-docx'),
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

async function openTicketFromBoard(id: number) {
  await userEvent.click(await screen.findByTestId(`ticket-card-${id}`));
  await screen.findByTestId('ticket-drawer-content');
}

describe('Word-выгрузка заявки (DOC-1)', () => {
  it('скачивает Word из карточки заявки с именем ticket_{id}.docx', async () => {
    authenticateAs(1);
    renderWithProviders(<TicketsPage />, {
      route: '/tickets',
      auth: makeAuthValue({ user: ADMIN_AUTH }),
    });

    await openTicketFromBoard(1);
    await userEvent.click(screen.getByTestId('export-button'));

    await waitFor(() => expect(downloads).toEqual(['ticket_1.docx']));
    expect(toast.success).toHaveBeenCalledWith('Файл готов', 'ticket_1.docx');
    // DOC-3: файл скачивается как attachment с корректным Content-Type.
    expect(URL.createObjectURL).toHaveBeenCalled();
  });

  it('скачивает Word с публичной карточки /t/{token}', async () => {
    renderWithProviders(
      <Routes>
        <Route path="/t/:token" element={<PublicTicketPage />} />
      </Routes>,
      { route: '/t/demo-token-1', auth: makeAnonymousAuthValue() },
    );

    await userEvent.click(await screen.findByTestId('export-button'));

    await waitFor(() => expect(downloads).toEqual(['ticket_4.docx']));
    expect(toast.success).toHaveBeenCalledWith('Файл готов', 'ticket_4.docx');
  });

  it('ошибка 500 показывает request_id и не создаёт файл', async () => {
    server.use(
      http.get('*/api/tickets/1/export.docx', () =>
        HttpResponse.json(
          { code: 'INTERNAL_ERROR', message: 'docx failed', details: null, request_id: 'req-docx' },
          { status: 500 },
        ),
      ),
    );

    renderWithProviders(<ExportButton path="/tickets/1/export.docx" label="Word" />, {
      auth: makeAuthValue({ user: ADMIN_AUTH }),
    });

    await userEvent.click(screen.getByTestId('export-button'));

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        'Ошибка сервера, попробуйте позже',
        'Код запроса: req-docx',
      ),
    );
    expect(downloads).toHaveLength(0);
    expect(URL.createObjectURL).not.toHaveBeenCalled();
  });
});
