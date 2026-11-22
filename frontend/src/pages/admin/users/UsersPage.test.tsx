/**
 * Тесты `/admin/users` (Ф6, AUTH-4…AUTH-9): создание, роли/здания,
 * блокировка с 409 и сброс пароля.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, http } from 'msw';
import type { AdminUser } from '@/api/types';
import { db } from '@/mocks/db';
import { server } from '@/mocks/server';
import { authenticateAs, makeAuthUser, makeAuthValue, renderWithProviders } from '@/test/utils';
import { UsersPage } from './UsersPage';

const ADMIN = makeAuthUser({
  id: 1,
  login: 'admin',
  full_name: 'Администратор Системы',
  role: 'ADMIN',
  building_id: null,
  building_name: null,
});

function renderUsers() {
  return renderWithProviders(<UsersPage />, {
    auth: makeAuthValue({ user: ADMIN }),
  });
}

function makeAdminUser(overrides: Partial<AdminUser> = {}): AdminUser {
  return {
    ...ADMIN,
    last_login_at: null,
    ...overrides,
  };
}

async function openCreateModal() {
  const user = userEvent.setup();
  await screen.findByTestId('users-table');
  await user.click(screen.getByTestId('user-create'));
  await screen.findByTestId('user-login-input');
  return user;
}

/** Открывает выпадающий список antd Select по его popup-классу. */
async function openSelect(
  user: ReturnType<typeof userEvent.setup>,
  testId: string,
  popupClass: string,
) {
  const root = screen.getByTestId(testId);
  // rc-select открывает список по mousedown на `.ant-select-selector`,
  // поэтому кликаем по нему, а не по внешнему контейнеру.
  await user.click(root.querySelector('.ant-select-selector') ?? root);
  return waitFor(() => {
    const dropdown = document.querySelector(`.${popupClass}`);
    if (!dropdown) throw new Error('Выпадающий список не открылся');
    return dropdown as HTMLElement;
  });
}

async function pickOption(
  user: ReturnType<typeof userEvent.setup>,
  testId: string,
  popupClass: string,
  optionText: string,
) {
  const dropdown = await openSelect(user, testId, popupClass);
  await user.click(within(dropdown).getByText(optionText));
}

beforeEach(() => {
  authenticateAs(1);
});

describe('UsersPage', () => {
  it('показывает список пользователей и фильтры', async () => {
    renderUsers();

    await waitFor(() => expect(screen.getByTestId('users-table')).toBeInTheDocument());
    expect(await screen.findByText('Иванов Иван Иванович')).toBeInTheDocument();
    // ADMIN не закреплён за зданием, USER — за «Центральным офисом».
    expect(screen.getAllByText('Центральный офис').length).toBeGreaterThan(0);
    expect(screen.getByTestId('users-filter-role')).toBeInTheDocument();
    expect(screen.getByTestId('users-filter-building')).toBeInTheDocument();
    expect(screen.getByTestId('users-filter-activity')).toBeInTheDocument();
    expect(screen.getByTestId('users-search')).toBeInTheDocument();
  });

  it('не даёт создать USER без здания — ошибка у поля, запрос не уходит', async () => {
    const createSpy = vi.fn(() => HttpResponse.json(makeAdminUser(), { status: 201 }));
    server.use(http.post('*/api/admin/users', createSpy));

    renderUsers();
    const user = await openCreateModal();

    await user.type(screen.getByTestId('user-login-input'), 'newuser');
    await user.type(screen.getByTestId('user-fullname-input'), 'Новый Сотрудник');
    await user.type(screen.getByTestId('user-password-input'), 'secret12345');
    await user.click(screen.getByRole('button', { name: 'Создать' }));

    expect(
      await screen.findByText('Для сотрудника обязательно выберите здание'),
    ).toBeInTheDocument();
    expect(createSpy).not.toHaveBeenCalled();
  });

  it('показывает неактивные здания как недоступные при создании USER', async () => {
    renderUsers();
    const user = await openCreateModal();

    const dropdown = await openSelect(user, 'user-building-select', 'user-building-dropdown');
    const inactive = within(dropdown)
      .getByText('Старый корпус (неактивно)')
      .closest('.ant-select-item-option');
    expect(inactive).toHaveClass('ant-select-item-option-disabled');
    const active = within(dropdown)
      .getByText('Центральный офис')
      .closest('.ant-select-item-option');
    expect(active).not.toHaveClass('ant-select-item-option-disabled');
  });

  it('создаёт USER с активным зданием и временным паролем', async () => {
    renderUsers();
    const user = await openCreateModal();

    await user.type(screen.getByTestId('user-login-input'), 'newuser');
    await user.type(screen.getByTestId('user-fullname-input'), 'Новый Сотрудник');
    await user.click(screen.getByTestId('user-password-generate'));
    await pickOption(user, 'user-building-select', 'user-building-dropdown', 'Центральный офис');
    await user.click(screen.getByRole('button', { name: 'Создать' }));

    await waitFor(() => {
      const created = db.users.find((entry) => entry.login === 'newuser');
      expect(created).toBeTruthy();
      expect(created?.building_id).toBe(1);
      expect(created?.must_change_password).toBe(true);
      expect(created?.role).toBe('USER');
    });
  });

  it('USER→ADMIN очищает здание (AUTH-8)', async () => {
    renderUsers();
    const user = userEvent.setup();

    await screen.findByTestId('users-table');
    await user.click(await screen.findByTestId('user-edit-2'));
    await screen.findByTestId('user-role-select');

    await pickOption(user, 'user-role-select', 'user-role-dropdown', 'Администратор');

    // ADMIN не закрепляется за зданием: поле заблокировано и пустое.
    const buildingPicker = screen.getByTestId('user-building-select').closest('.ant-select');
    await waitFor(() => expect(buildingPicker).toHaveClass('ant-select-disabled'));

    await user.click(screen.getByRole('button', { name: 'Сохранить' }));

    await waitFor(() => {
      const updated = db.users.find((entry) => entry.id === 2);
      expect(updated?.role).toBe('ADMIN');
      expect(updated?.building_id).toBeNull();
    });
  });

  it('ADMIN→USER без здания — ошибка валидации, роль не меняется', async () => {
    db.users.push({
      id: 99,
      login: 'admin2',
      password: 'admin12345',
      full_name: 'Второй Администратор',
      role: 'ADMIN',
      building_id: null,
      is_active: true,
      must_change_password: false,
      created_at: new Date().toISOString(),
      last_login_at: null,
    });

    renderUsers();
    const user = userEvent.setup();
    await screen.findByTestId('users-table');
    await user.click(await screen.findByTestId('user-edit-99'));
    await screen.findByTestId('user-role-select');

    await pickOption(user, 'user-role-select', 'user-role-dropdown', 'Сотрудник');
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));

    expect(
      await screen.findByText('Для сотрудника обязательно выберите здание'),
    ).toBeInTheDocument();
    expect(db.users.find((entry) => entry.id === 99)?.role).toBe('ADMIN');
  });

  it('самоблокировка возвращает 409 CANNOT_MODIFY_SELF и не меняет статус', async () => {
    renderUsers();
    const user = userEvent.setup();
    await screen.findByTestId('users-table');

    await user.click(await screen.findByTestId('user-active-1'));

    expect(
      await screen.findByText('Нельзя изменить собственную учётную запись'),
    ).toBeInTheDocument();
    expect(db.users.find((entry) => entry.id === 1)?.is_active).toBe(true);
  });

  it('показывает 409 LAST_ADMIN в модалке редактирования', async () => {
    server.use(
      http.patch('*/api/admin/users/:id', () =>
        HttpResponse.json(
          {
            code: 'LAST_ADMIN',
            message: 'Нельзя понизить или заблокировать последнего администратора',
            details: null,
            request_id: 'mock-last-admin',
          },
          { status: 409 },
        ),
      ),
    );

    renderUsers();
    const user = userEvent.setup();
    await screen.findByTestId('users-table');
    await user.click(await screen.findByTestId('user-edit-2'));
    await screen.findByTestId('user-role-select');
    await user.click(screen.getByRole('button', { name: 'Сохранить' }));

    await waitFor(() =>
      expect(screen.getByTestId('user-form-error')).toHaveTextContent(
        'Нельзя понизить или заблокировать последнего администратора',
      ),
    );
  });

  it('сбрасывает пароль и показывает временный пароль администратору', async () => {
    renderUsers();
    const user = userEvent.setup();
    await screen.findByTestId('users-table');

    await user.click(await screen.findByTestId('user-reset-2'));
    await user.click(await screen.findByTestId('confirm-dialog-ok'));

    const value = await screen.findByTestId('reset-password-value');
    const password = (value as HTMLInputElement).value;
    expect(password.length).toBeGreaterThanOrEqual(8);
    expect(db.users.find((entry) => entry.id === 2)?.password).toBe(password);
    expect(db.users.find((entry) => entry.id === 2)?.must_change_password).toBe(true);
  });
});
