/**
 * Модалка создания/правки пользователя (AUTH-4, AUTH-8).
 *
 * Правила, которые проверяются и на клиенте, и на сервере:
 *  - для роли USER здание обязательно (иначе 422 BUILDING_REQUIRED_FOR_USER);
 *  - неактивное здание нельзя назначить (422 BUILDING_INACTIVE), но можно
 *    оставить текущее, если оно деактивировано позже;
 *  - ADMIN не закреплён за зданием: при роли ADMIN поле очищается и блокируется;
 *  - временный пароль — не короче 8 символов (AUTH-4), можно сгенерировать.
 */
import { useEffect, useState } from 'react';
import { Alert, Button, Form, Input, Modal, Select, Space, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import type { AdminUser, Building, Role } from '@/api/types';
import { ApiError } from '@/api/errors';
import {
  ROLE_LABELS,
  adminErrorMessage,
  generateTemporaryPassword,
} from '../shared';

export interface UserFormValues {
  login?: string;
  full_name: string;
  role: Role;
  building_id: number | null;
  password?: string;
}

export interface UserFormModalProps {
  open: boolean;
  mode: 'create' | 'edit';
  user: AdminUser | null;
  buildings: Building[];
  pending?: boolean;
  error?: unknown;
  onCancel: () => void;
  onSubmit: (values: UserFormValues) => void | Promise<void>;
}

export function UserFormModal({
  open,
  mode,
  user,
  buildings,
  pending = false,
  error,
  onCancel,
  onSubmit,
}: UserFormModalProps) {
  const [form] = Form.useForm<UserFormValues>();
  const role = Form.useWatch('role', form) ?? (mode === 'edit' ? (user?.role ?? 'USER') : 'USER');
  const [generalError, setGeneralError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setGeneralError(null);
    form.setFieldsValue({
      login: user?.login ?? '',
      full_name: user?.full_name ?? '',
      role: user?.role ?? 'USER',
      building_id: user?.building_id ?? null,
      password: '',
    });
    form.setFields([
      { name: 'building_id', errors: [] },
      { name: 'login', errors: [] },
      { name: 'password', errors: [] },
    ]);
  }, [open, user, mode, form]);

  useEffect(() => {
    if (!error) {
      setGeneralError(null);
      return;
    }
    // 422 адресные коды → ошибка у поля «Здание» (AUTH-4/AUTH-8).
    if (error instanceof ApiError && (error.code === 'BUILDING_REQUIRED_FOR_USER' || error.code === 'BUILDING_INACTIVE')) {
      form.setFields([{ name: 'building_id', errors: [error.message] }]);
      setGeneralError(null);
      return;
    }
    // 409 LAST_ADMIN / CANNOT_MODIFY_SELF и прочее — общий текст в модалке.
    setGeneralError(adminErrorMessage(error, 'Не удалось сохранить пользователя'));
  }, [error, form]);

  const buildingOptions = buildings.map((building) => ({
    value: building.id,
    label: `${building.name}${building.is_active ? '' : ' (неактивно)'}`,
    // Текущее (уже назначенное) неактивное здание можно сохранить без изменений.
    disabled: !building.is_active && building.id !== (user?.building_id ?? null),
  }));

  return (
    <Modal
      open={open}
      title={mode === 'create' ? 'Новый пользователь' : `Пользователь: ${user?.login ?? ''}`}
      okText={mode === 'create' ? 'Создать' : 'Сохранить'}
      cancelText="Отмена"
      confirmLoading={pending}
      onOk={() => void form.submit()}
      onCancel={onCancel}
      destroyOnHidden
      data-testid="user-form-modal"
    >
      {generalError ? (
        <Alert
          type="error"
          showIcon
          message={generalError}
          data-testid="user-form-error"
          style={{ marginBottom: 12 }}
        />
      ) : null}

      <Form<UserFormValues>
        form={form}
        layout="vertical"
        preserve={false}
        onFinish={(values) =>
          void onSubmit({
            ...values,
            // AUTH-8: у ADMIN здания нет; у USER значение обязательно.
            building_id: values.role === 'ADMIN' ? null : (values.building_id ?? null),
          })
        }
      >
        <Form.Item
          name="login"
          label="Логин"
          rules={
            mode === 'create'
              ? [
                  { required: true, message: 'Укажите логин' },
                  { min: 3, max: 64, message: 'Логин — от 3 до 64 символов' },
                ]
              : []
          }
        >
          <Input
            placeholder="ivanov"
            disabled={mode === 'edit'}
            data-testid="user-login-input"
            autoComplete="off"
          />
        </Form.Item>

        <Form.Item
          name="full_name"
          label="ФИО"
          rules={[
            { required: true, message: 'Укажите ФИО' },
            { max: 200, message: 'Не больше 200 символов' },
          ]}
        >
          <Input placeholder="Иванов Иван Иванович" data-testid="user-fullname-input" />
        </Form.Item>

        <Form.Item name="role" label="Роль" rules={[{ required: true }]}>
          <Select
            data-testid="user-role-select"
            classNames={{ popup: { root: 'user-role-dropdown' } }}
            onChange={(value: Role) => {
              // ADMIN не закреплён за зданием (AUTH-8): очищаем и блокируем.
              if (value === 'ADMIN') form.setFieldValue('building_id', null);
            }}
            options={[
              { value: 'USER', label: ROLE_LABELS.USER },
              { value: 'ADMIN', label: ROLE_LABELS.ADMIN },
            ]}
          />
        </Form.Item>

        <Form.Item
          name="building_id"
          label="Здание"
          dependencies={['role']}
          rules={[
            ({ getFieldValue }) => ({
              validator: (_rule, value: number | null) => {
                if (getFieldValue('role') !== 'USER') return Promise.resolve();
                if (value === null || value === undefined) {
                  // AUTH-4: у сотрудника здание обязательно.
                  return Promise.reject(new Error('Для сотрудника обязательно выберите здание'));
                }
                const building = buildings.find((item) => item.id === value);
                if (building && !building.is_active && value !== (user?.building_id ?? null)) {
                  return Promise.reject(new Error('Здание деактивировано'));
                }
                return Promise.resolve();
              },
            }),
          ]}
          extra={
            role === 'ADMIN' ? (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Администратор не закрепляется за зданием (AUTH-8)
              </Typography.Text>
            ) : undefined
          }
        >
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="Выберите здание"
            disabled={role === 'ADMIN'}
            options={buildingOptions}
            classNames={{ popup: { root: 'user-building-dropdown' } }}
            data-testid="user-building-select"
          />
        </Form.Item>

        {mode === 'create' ? (
          <Form.Item
            name="password"
            label="Временный пароль"
            rules={[
              { required: true, message: 'Укажите временный пароль' },
              { min: 8, message: 'Пароль — не короче 8 символов' },
            ]}
            extra="Сотрудник сменит пароль при первом входе (AUTH-4)"
          >
            <Input
              placeholder="Не короче 8 символов"
              data-testid="user-password-input"
              autoComplete="new-password"
              suffix={
                <Space size={4}>
                  <Button
                    type="text"
                    size="small"
                    icon={<ReloadOutlined />}
                    aria-label="Сгенерировать временный пароль"
                    data-testid="user-password-generate"
                    onClick={() => form.setFieldValue('password', generateTemporaryPassword())}
                  />
                </Space>
              }
            />
          </Form.Item>
        ) : null}
      </Form>
    </Modal>
  );
}
