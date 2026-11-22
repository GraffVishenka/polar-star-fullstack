/**
 * Форма создания заявки (Ф2): у USER здание зафиксировано, у ADMIN — выбор;
 * валидация полей и появление новой заявки на доске.
 */
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { db } from '@/mocks/db';
import {
  ADMIN_AUTH,
  USER_AUTH,
  authenticateAs,
  makeAuthValue,
  renderWithProviders,
} from '@/test/utils';
import { TicketsPage } from '../TicketsPage';
import { pickSelectOption } from './testHelpers';

const adminAuth = makeAuthValue({ user: ADMIN_AUTH });
const userAuth = makeAuthValue({ user: USER_AUTH });

async function openCreateModal() {
  await userEvent.click(await screen.findByTestId('open-create'));
  await screen.findByTestId('ticket-create-modal');
}

describe('TicketCreateModal: USER', () => {
  it('фиксирует здание сотрудника и не даёт выбрать чужое', async () => {
    authenticateAs(2);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: userAuth });

    await openCreateModal();

    expect(screen.getByTestId('create-building-fixed')).toHaveValue('Центральный офис');
    expect(screen.getByTestId('create-building-fixed')).toBeDisabled();
    expect(screen.queryByTestId('create-building')).not.toBeInTheDocument();
  });

  it('валидирует обязательные поля', async () => {
    authenticateAs(2);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: userAuth });

    await openCreateModal();
    await userEvent.clear(screen.getByTestId('create-applicant'));
    await userEvent.click(screen.getByTestId('create-submit'));

    expect(await screen.findByText('Укажите заявителя')).toBeInTheDocument();
    expect(screen.getByText('Укажите кабинет')).toBeInTheDocument();
    expect(screen.getByText('Добавьте описание')).toBeInTheDocument();
    // Текст встречается и в placeholder, и в сообщении об ошибке.
    expect(screen.getAllByText('Выберите вид проблемы').length).toBeGreaterThan(1);
    expect(db.tickets).toHaveLength(13);
  });

  it('создаёт заявку в здании сотрудника и открывает её карточку', async () => {
    authenticateAs(2);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: userAuth });

    await openCreateModal();
    await userEvent.clear(screen.getByTestId('create-applicant'));
    await userEvent.type(screen.getByTestId('create-applicant'), 'Петров Пётр');
    await userEvent.type(screen.getByTestId('create-cabinet'), 'каб. 12');
    await pickSelectOption('create-problem-type', 'Освещение');
    await userEvent.type(screen.getByTestId('create-description'), 'Перегорела лампа');
    await userEvent.click(screen.getByTestId('create-submit'));

    await waitFor(() => expect(db.tickets).toHaveLength(14));
    const created = db.tickets.at(-1)!;
    expect(created.building_id).toBe(1);
    expect(created.applicant_name).toBe('Петров Пётр');
    expect(created.status).toBe('new');
    expect(created.executor_id).toBeNull();

    // После создания открывается карточка новой заявки.
    expect(await screen.findByText(`Заявка №${created.id}`)).toBeInTheDocument();
  });
});

describe('TicketCreateModal: ADMIN', () => {
  it('позволяет выбрать активное здание', async () => {
    authenticateAs(1);
    renderWithProviders(<TicketsPage />, { route: '/tickets', auth: adminAuth });

    await openCreateModal();

    expect(screen.getByTestId('create-building')).toBeInTheDocument();
    await pickSelectOption('create-building', 'Производственный корпус');

    await userEvent.clear(screen.getByTestId('create-applicant'));
    await userEvent.type(screen.getByTestId('create-applicant'), 'Гость');
    await userEvent.type(screen.getByTestId('create-cabinet'), 'цех Б');
    await pickSelectOption('create-problem-type', 'Прочее');
    await userEvent.type(screen.getByTestId('create-description'), 'Нужна помощь');
    await userEvent.click(screen.getByTestId('create-submit'));

    await waitFor(() => expect(db.tickets).toHaveLength(14));
    expect(db.tickets.at(-1)?.building_id).toBe(2);
  });
});
