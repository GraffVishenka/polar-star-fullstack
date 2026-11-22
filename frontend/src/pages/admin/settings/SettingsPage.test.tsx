/**
 * Тесты `/admin/settings` (Ф6, CAL-1…5): 422 по полям, живые часы в поясе,
 * превью статусов, рабочие дни и праздники.
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { db } from '@/mocks/db';
import { authenticateAs, makeAuthUser, makeAuthValue, renderWithProviders } from '@/test/utils';
import { SettingsPage } from './SettingsPage';

const ADMIN = makeAuthUser({
  id: 1,
  login: 'admin',
  full_name: 'Администратор Системы',
  role: 'ADMIN',
  building_id: null,
  building_name: null,
});

function renderPage() {
  return renderWithProviders(<SettingsPage />, {
    auth: makeAuthValue({ user: ADMIN }),
  });
}

async function waitForLoaded() {
  await waitFor(() =>
    expect(screen.getByTestId('settings-app-name')).toHaveValue('Сервисный центр'),
  );
}

async function clearAndType(element: HTMLElement, value: string) {
  const user = userEvent.setup();
  await user.clear(element);
  if (value) await user.type(element, value);
}

beforeEach(() => {
  authenticateAs(1);
});

describe('SettingsPage', () => {
  it('загружает настройки, показывает часы в поясе и превью статусов', async () => {
    renderPage();
    await waitForLoaded();

    expect(screen.getByTestId('settings-timezone')).toHaveValue('Asia/Yekaterinburg');
    expect(await screen.findByTestId('settings-current-time')).toHaveTextContent(
      'Asia/Yekaterinburg',
    );
    expect(screen.getByTestId('status-tag-new')).toHaveTextContent('Новая');
    expect(screen.getByTestId('settings-workdays')).toBeInTheDocument();
  });

  it('показывает 422 по полю deadline_days и не сохраняет настройки', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    const days = screen.getByTestId('settings-deadline-days');
    await clearAndType(days, '0');
    await user.click(screen.getByTestId('settings-save'));

    expect(await screen.findByText('Срок дедлайна — от 1 до 30 рабочих дней')).toBeInTheDocument();
    expect(db.settings.deadline_days).toBe(2);
  });

  it('показывает 422 по полю deadline_reminder_time (HH:MM)', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    await clearAndType(screen.getByTestId('settings-reminder-time'), '25:00');
    await user.click(screen.getByTestId('settings-save'));

    expect(await screen.findByText('Время напоминания в формате HH:MM')).toBeInTheDocument();
    expect(db.settings.deadline_reminder_time).toBe('10:00');
  });

  it('показывает 422 по полю timezone и предупреждение о неизвестном поясе', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    await clearAndType(screen.getByTestId('settings-timezone'), 'Europe/Mordor');
    await waitFor(() =>
      expect(screen.getByTestId('settings-timezone-invalid')).toBeInTheDocument(),
    );

    await user.click(screen.getByTestId('settings-save'));
    expect(await screen.findByText('Неизвестный часовой пояс')).toBeInTheDocument();
    expect(db.settings.timezone).toBe('Asia/Yekaterinburg');
  });

  it('обновляет превью статуса сразу и сохраняет новое название', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    const labelInput = screen.getByTestId('status-label-new');
    await clearAndType(labelInput, 'Свежая заявка');

    await waitFor(() =>
      expect(screen.getByTestId('status-tag-new')).toHaveTextContent('Свежая заявка'),
    );

    await user.click(screen.getByTestId('settings-save'));

    await waitFor(() => {
      const status = db.settings.statuses.find((item) => item.code === 'new');
      expect(status?.label).toBe('Свежая заявка');
    });
  });

  it('сохраняет маску рабочих дней (Пн–Сб → без субботы)', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    const workdays = screen.getByTestId('settings-workdays');
    await user.click(within(workdays).getByText('Сб'));
    await user.click(screen.getByTestId('settings-save'));

    await waitFor(() => {
      expect(db.settings.workdays).not.toContain('sat');
      expect(db.settings.workdays).toContain('mon');
      expect(db.settings.workdays).toContain('fri');
    });
  });

  it('добавляет и удаляет праздник (CAL-1)', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    fireEvent.change(screen.getByTestId('holiday-date'), {
      target: { value: '2026-12-31' },
    });
    await user.type(screen.getByTestId('holiday-name'), 'Новогодний корпоратив');
    await user.click(screen.getByTestId('holiday-add'));

    expect(await screen.findByText('31.12.2026')).toBeInTheDocument();
    expect(db.holidays.some((holiday) => holiday.date === '2026-12-31')).toBe(true);

    const created = db.holidays.find((holiday) => holiday.date === '2026-12-31');
    await user.click(await screen.findByTestId(`holiday-delete-${created?.id}`));
    await user.click(await screen.findByTestId('confirm-dialog-ok'));

    await waitFor(() =>
      expect(db.holidays.some((holiday) => holiday.date === '2026-12-31')).toBe(false),
    );
  });

  it('редактирует праздник: дата и название (CAL-1)', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    const existing = db.holidays[0]!;
    await user.click(await screen.findByTestId(`holiday-edit-${existing.id}`));
    const modal = await screen.findByTestId('holiday-edit-modal');
    const nameInput = within(modal).getByTestId('holiday-edit-name');
    await clearAndType(nameInput, 'Новогодние каникулы');
    await user.click(within(modal).getByRole('button', { name: 'Сохранить' }));

    await waitFor(() => {
      const updated = db.holidays.find((holiday) => holiday.id === existing.id);
      expect(updated?.name).toBe('Новогодние каникулы');
    });
  });

  it('показывает 422 по полю app_name для пустого названия', async () => {
    renderPage();
    await waitForLoaded();
    const user = userEvent.setup();

    await clearAndType(screen.getByTestId('settings-app-name'), '');
    await user.click(screen.getByTestId('settings-save'));

    // Клиентское required-правило не даёт отправить пустое значение.
    expect(await screen.findByText('Укажите название приложения')).toBeInTheDocument();
  });
});
