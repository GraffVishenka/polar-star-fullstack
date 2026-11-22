/**
 * `/admin/users` — пользователи, роли и здания (Ф6, AUTH-4…AUTH-9).
 *
 * Таблица с фильтрами (роль, здание, активность, поиск) и серверной
 * пагинацией. Создание/правка — модалка с проверками AUTH-4/AUTH-8;
 * блокировка и сброс пароля — немедленные действия с подтверждением.
 * Ошибки 409 `LAST_ADMIN`/`CANNOT_MODIFY_SELF` показываются текстом, 422
 * `BUILDING_REQUIRED_FOR_USER`/`BUILDING_INACTIVE` — у поля «Здание».
 */
import { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Flex,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import type { TableColumnsType } from 'antd';
import { KeyOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import dayjs from 'dayjs';
import {
  createAdminUser,
  fetchAdminUsers,
  resetAdminUserPassword,
  updateAdminUser,
} from '@/api/admin';
import { fetchBuildings } from '@/api/equipment';
import type {
  AdminUser,
  AdminUserCreateRequest,
  AdminUserListParams,
  AdminUserUpdateRequest,
  Building,
  Role,
} from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { queryKeys } from '@/app/queryClient';
import { toast } from '@/app/toast';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { PageHeader } from '@/components/PageHeader';
import { EmptyState } from '@/components/EmptyState';
import { ROLE_LABELS, adminErrorMessage, generateTemporaryPassword } from '../shared';
import { UserFormModal, type UserFormValues } from './UserFormModal';

type ActivityFilter = 'all' | 'active' | 'inactive';

function formatDate(value: string | null): string {
  if (!value) return '—';
  const parsed = dayjs(value);
  return parsed.isValid() ? parsed.format('DD.MM.YYYY HH:mm') : value;
}

export function UsersPage() {
  const { user: currentUser } = useAuth();
  const queryClient = useQueryClient();

  const [role, setRole] = useState<Role | 'all'>('all');
  const [buildingId, setBuildingId] = useState<number | 'all'>('all');
  const [activity, setActivity] = useState<ActivityFilter>('all');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(20);

  const [formState, setFormState] = useState<{ mode: 'create' | 'edit'; user: AdminUser | null } | null>(
    null,
  );
  const [resetTarget, setResetTarget] = useState<AdminUser | null>(null);
  const [resetResult, setResetResult] = useState<{ user: AdminUser; password: string } | null>(null);
  const [pageError, setPageError] = useState<string | null>(null);
  const [pendingUserId, setPendingUserId] = useState<number | null>(null);

  const params: AdminUserListParams = {
    page,
    size,
    role: role === 'all' ? undefined : role,
    building_id: buildingId === 'all' ? undefined : buildingId,
    is_active: activity === 'all' ? undefined : activity === 'active',
    search: search.trim() || undefined,
  };

  const usersQuery = useQuery({
    queryKey: queryKeys.adminUsers(params),
    queryFn: () => fetchAdminUsers(params),
  });
  const buildingsQuery = useQuery({
    queryKey: queryKeys.buildings,
    queryFn: () => fetchBuildings(),
  });

  const buildings = useMemo<Building[]>(() => buildingsQuery.data ?? [], [buildingsQuery.data]);
  const buildingMap = useMemo(
    () => new Map(buildings.map((building) => [building.id, building])),
    [buildings],
  );

  const saveMutation = useMutation({
    mutationFn: (input: {
      mode: 'create' | 'edit';
      userId?: number;
      values: UserFormValues;
    }) => {
      if (input.mode === 'create') {
        const body: AdminUserCreateRequest = {
          login: (input.values.login ?? '').trim(),
          full_name: input.values.full_name.trim(),
          role: input.values.role,
          building_id: input.values.role === 'USER' ? input.values.building_id : null,
          password: input.values.password ?? '',
        };
        return createAdminUser(body);
      }
      const body: AdminUserUpdateRequest = {
        full_name: input.values.full_name.trim(),
        role: input.values.role,
        building_id: input.values.role === 'USER' ? input.values.building_id : null,
      };
      return updateAdminUser(input.userId as number, body);
    },
    onSuccess: (saved, input) => {
      setFormState(null);
      toast.success(
        input.mode === 'create'
          ? `Пользователь ${saved.login} создан`
          : `Пользователь ${saved.login} сохранён`,
      );
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (input: { id: number; is_active: boolean }) =>
      updateAdminUser(input.id, { is_active: input.is_active }),
    onMutate: (input) => {
      setPendingUserId(input.id);
      setPageError(null);
    },
    onSuccess: () => {
      toast.success('Статус пользователя обновлён');
    },
    onError: (error) => {
      // AUTH-7: 409 CANNOT_MODIFY_SELF / LAST_ADMIN — текст в баннере.
      setPageError(adminErrorMessage(error, 'Не удалось изменить статус пользователя'));
    },
    onSettled: () => {
      setPendingUserId(null);
      void queryClient.invalidateQueries({ queryKey: ['admin', 'users'] });
    },
  });

  const resetPassword = async () => {
    const target = resetTarget;
    if (!target) return;
    const password = generateTemporaryPassword();
    try {
      await resetAdminUserPassword(target.id, { password });
      setResetResult({ user: target, password });
      setResetTarget(null);
      toast.success('Пароль сброшен');
    } catch (error) {
      setPageError(adminErrorMessage(error, 'Не удалось сбросить пароль'));
      setResetTarget(null);
    }
  };

  const openCreate = () => {
    saveMutation.reset();
    setFormState({ mode: 'create', user: null });
  };

  const openEdit = (user: AdminUser) => {
    saveMutation.reset();
    setFormState({ mode: 'edit', user });
  };

  const columns: TableColumnsType<AdminUser> = [
    {
      title: 'Логин',
      dataIndex: 'login',
      key: 'login',
      width: 140,
      render: (login: string, record) => (
        <Space size={4} wrap>
          <Typography.Text strong>{login}</Typography.Text>
          {record.id === currentUser?.id ? (
            <Tag color="blue" data-testid="user-self-tag">
              это вы
            </Tag>
          ) : null}
        </Space>
      ),
    },
    {
      title: 'ФИО',
      dataIndex: 'full_name',
      key: 'full_name',
      render: (name: string) => <Typography.Text>{name}</Typography.Text>,
    },
    {
      title: 'Роль',
      dataIndex: 'role',
      key: 'role',
      width: 150,
      render: (value: Role, record) => (
        <Tag
          color={value === 'ADMIN' ? 'gold' : 'blue'}
          data-testid={`user-role-${record.id}`}
        >
          {ROLE_LABELS[value]}
        </Tag>
      ),
    },
    {
      title: 'Здание',
      dataIndex: 'building_id',
      key: 'building',
      width: 200,
      render: (_value: number | null, record) => {
        if (record.role === 'ADMIN') {
          return (
            <Typography.Text type="secondary" data-testid={`user-building-${record.id}`}>
              Все здания (не закреплён)
            </Typography.Text>
          );
        }
        const building = record.building_id ? buildingMap.get(record.building_id) : undefined;
        if (!record.building_id) {
          return (
            <Typography.Text type="danger" data-testid={`user-building-${record.id}`}>
              Не назначено
            </Typography.Text>
          );
        }
        return (
          <Space size={4} wrap data-testid={`user-building-${record.id}`}>
            <Typography.Text>{building?.name ?? `#${record.building_id}`}</Typography.Text>
            {building && !building.is_active ? <Tag data-testid="user-building-inactive">неактивно</Tag> : null}
          </Space>
        );
      },
    },
    {
      title: 'Активен',
      dataIndex: 'is_active',
      key: 'is_active',
      width: 130,
      render: (isActive: boolean, record) => (
        <Switch
          checked={isActive}
          checkedChildren="активен"
          unCheckedChildren="заблокирован"
          loading={pendingUserId === record.id}
          onChange={(checked) => toggleMutation.mutate({ id: record.id, is_active: checked })}
          data-testid={`user-active-${record.id}`}
        />
      ),
    },
    {
      title: 'Пароль',
      dataIndex: 'must_change_password',
      key: 'must_change_password',
      width: 140,
      render: (mustChange: boolean) =>
        mustChange ? (
          <Tag color="orange" data-testid="user-temp-password-tag">
            временный
          </Tag>
        ) : (
          <Typography.Text type="secondary">постоянный</Typography.Text>
        ),
    },
    {
      title: 'Последний вход',
      dataIndex: 'last_login_at',
      key: 'last_login_at',
      width: 170,
      render: (value: string | null) => (
        <Typography.Text type="secondary">{formatDate(value)}</Typography.Text>
      ),
    },
    {
      title: 'Действия',
      key: 'actions',
      width: 190,
      render: (_value, record) => (
        <Space size={4} wrap>
          <Button size="small" onClick={() => openEdit(record)} data-testid={`user-edit-${record.id}`}>
            Изменить
          </Button>
          <Tooltip title="Сгенерировать временный пароль и показать его">
            <Button
              size="small"
              icon={<KeyOutlined />}
              onClick={() => setResetTarget(record)}
              data-testid={`user-reset-${record.id}`}
            >
              Пароль
            </Button>
          </Tooltip>
        </Space>
      ),
    },
  ];

  const total = usersQuery.data?.total ?? 0;
  const current = usersQuery.data?.items ?? [];

  return (
    <Flex vertical gap={16} data-testid="admin-users-page">
      <PageHeader
        title="Пользователи"
        description="Аккаунты создаёт только ADMIN. Для сотрудника здание обязательно (AUTH-4)."
        extra={
          <Space size={8} wrap>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => void usersQuery.refetch()}
              aria-label="Обновить список пользователей"
            >
              Обновить
            </Button>
            <Button type="primary" icon={<PlusOutlined />} onClick={openCreate} data-testid="user-create">
              Создать пользователя
            </Button>
          </Space>
        }
      />

      {pageError ? (
        <Alert
          type="error"
          showIcon
          closable
          message={pageError}
          data-testid="admin-users-error"
          onClose={() => setPageError(null)}
        />
      ) : null}

      <Card size="small">
        <Flex gap={8} wrap align="center" style={{ marginBottom: 12 }}>
          <Select
            value={role}
            onChange={(value) => {
              setRole(value);
              setPage(1);
            }}
            style={{ width: 170 }}
            aria-label="Фильтр по роли"
            data-testid="users-filter-role"
            options={[
              { value: 'all', label: 'Все роли' },
              { value: 'USER', label: ROLE_LABELS.USER },
              { value: 'ADMIN', label: ROLE_LABELS.ADMIN },
            ]}
          />
          <Select
            value={buildingId}
            onChange={(value) => {
              setBuildingId(value);
              setPage(1);
            }}
            style={{ width: 230 }}
            aria-label="Фильтр по зданию"
            data-testid="users-filter-building"
            options={[
              { value: 'all', label: 'Все здания' },
              ...buildings.map((building) => ({
                value: building.id,
                label: `${building.name}${building.is_active ? '' : ' (неактивно)'}`,
              })),
            ]}
          />
          <Select
            value={activity}
            onChange={(value: ActivityFilter) => {
              setActivity(value);
              setPage(1);
            }}
            style={{ width: 170 }}
            aria-label="Фильтр по активности"
            data-testid="users-filter-activity"
            options={[
              { value: 'all', label: 'Все статусы' },
              { value: 'active', label: 'Активные' },
              { value: 'inactive', label: 'Заблокированные' },
            ]}
          />
          <Input.Search
            allowClear
            placeholder="Поиск по логину или ФИО"
            defaultValue={search}
            onSearch={(value) => {
              setSearch(value);
              setPage(1);
            }}
            style={{ width: 260 }}
            aria-label="Поиск пользователей"
            data-testid="users-search"
          />
        </Flex>

        {usersQuery.error ? (
          <Alert
            type="error"
            showIcon
            message={adminErrorMessage(usersQuery.error, 'Не удалось загрузить пользователей')}
            data-testid="admin-users-load-error"
            style={{ marginBottom: 12 }}
          />
        ) : null}

        <Table<AdminUser>
          rowKey="id"
          size="middle"
          columns={columns}
          dataSource={current}
          loading={usersQuery.isLoading}
          data-testid="users-table"
          locale={{
            emptyText: (
              <EmptyState
                compact
                title="Пользователей не найдено"
                description="Измените фильтры или создайте нового пользователя."
              />
            ),
          }}
          pagination={{
            current: page,
            pageSize: size,
            total,
            showSizeChanger: true,
            pageSizeOptions: [10, 20, 50],
            showTotal: (count) => `Всего: ${count}`,
            onChange: (nextPage, nextSize) => {
              setPage(nextPage);
              setSize(nextSize);
            },
          }}
        />
      </Card>

      <UserFormModal
        open={formState !== null}
        mode={formState?.mode ?? 'create'}
        user={formState?.user ?? null}
        buildings={buildings}
        pending={saveMutation.isPending}
        error={saveMutation.error}
        onCancel={() => {
          setFormState(null);
          saveMutation.reset();
        }}
        onSubmit={(values) => {
          void saveMutation
            .mutateAsync({
              mode: formState?.mode ?? 'create',
              userId: formState?.user?.id,
              values,
            })
            .catch(() => undefined);
        }}
      />

      <ConfirmDialog
        open={resetTarget !== null}
        title="Сбросить пароль?"
        description={
          resetTarget
            ? `Для пользователя ${resetTarget.login} будет сгенерирован временный пароль. Он обязан сменить его при входе (AUTH-5).`
            : undefined
        }
        okText="Сбросить"
        onConfirm={resetPassword}
        onCancel={() => setResetTarget(null)}
      />

      <Modal
        open={resetResult !== null}
        title="Временный пароль"
        okText="Готово"
        cancelButtonProps={{ style: { display: 'none' } }}
        onOk={() => setResetResult(null)}
        onCancel={() => setResetResult(null)}
        data-testid="reset-password-modal"
      >
        <Flex vertical gap={12}>
          <Typography.Text>
            Передайте пароль пользователю <Typography.Text strong>{resetResult?.user.login}</Typography.Text>.
            При первом входе потребуется его смена (AUTH-4).
          </Typography.Text>
          <Space.Compact style={{ width: '100%' }}>
            <Input
              readOnly
              value={resetResult?.password ?? ''}
              data-testid="reset-password-value"
            />
            <Button
              type="default"
              data-testid="reset-password-copy"
              onClick={() => {
                const value = resetResult?.password ?? '';
                void navigator.clipboard?.writeText(value).catch(() => undefined);
              }}
            >
              Копировать
            </Button>
          </Space.Compact>
        </Flex>
      </Modal>
    </Flex>
  );
}
