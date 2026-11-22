/**
 * Карточка заявки в Drawer (Ф2, §3.3).
 *
 * Матрица прав (TICKET-7..TICKET-12):
 *  - статус: USER своего здания / ADMIN;
 *  - «Назначить себя»: USER в своём здании; ADMIN — любой USER здания заявки или ADMIN, и снятие;
 *  - дедлайн и здание: только ADMIN; смена здания предупреждает о сбросе USER-исполнителя;
 *  - правка полей: исполнитель или ADMIN;
 *  - удаление в корзину: только ADMIN, с подтверждением.
 */
import { useEffect, useMemo, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DeleteOutlined,
  EditOutlined,
  ReloadOutlined,
  SaveOutlined,
  StopOutlined,
  UserAddOutlined,
} from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  DatePicker,
  Descriptions,
  Drawer,
  Flex,
  Form,
  Input,
  Segmented,
  Select,
  Skeleton,
  Space,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import { fetchAssignableUsers, fetchTicket, ticketExportPath } from '@/api/tickets';
import { fetchBuildings } from '@/api/equipment';
import { fetchProblemTypesForSelection } from '@/api/dictionaries';
import type { TicketStatus, TicketUpdateRequest } from '@/api/types';
import { isApiError } from '@/api/errors';
import { useAuth } from '@/auth/AuthContext';
import { queryKeys } from '@/app/queryClient';
import { BuildingBadge } from '@/components/BuildingBadge';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { DeadlineBadge } from '@/components/DeadlineBadge';
import { ExportButton } from '@/components/ExportButton';
import { StatusTag } from '@/components/StatusTag';
import { TicketComments } from './TicketComments';
import { getTicketPermissions, willResetExecutor } from './ticketPermissions';
import { useUpdateTicket, useDeleteTicket } from '../hooks/useUpdateTicket';
import { findCachedTicket } from '../hooks/optimistic';
import { useTicketStatusMeta } from '../hooks/useTicketStatusMeta';

export interface TicketDrawerProps {
  ticketId: number | null;
  open: boolean;
  onClose: () => void;
}

interface EditValues {
  applicant_name: string;
  cabinet: string;
  problem_type_id: number;
  description: string;
}

export function TicketDrawer({ ticketId, open, onClose }: TicketDrawerProps) {
  const { user, isAdmin } = useAuth();
  const qc = useQueryClient();
  const { statuses } = useTicketStatusMeta();
  const [editForm] = Form.useForm<EditValues>();

  const [editing, setEditing] = useState(false);
  const [pendingBuildingId, setPendingBuildingId] = useState<number | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const cached = useMemo(
    () => (ticketId !== null ? findCachedTicket(qc, ticketId) : undefined),
    [qc, ticketId],
  );

  const ticketQuery = useQuery({
    queryKey: queryKeys.ticket(ticketId ?? 0),
    queryFn: () => fetchTicket(ticketId!),
    enabled: open && ticketId !== null,
    placeholderData: cached,
    retry: false,
  });

  const ticket = ticketQuery.data;
  const permissions = getTicketPermissions(ticket ?? null, user);

  const assignableQuery = useQuery({
    queryKey: queryKeys.assignableUsers(ticketId ?? undefined),
    queryFn: () => fetchAssignableUsers(ticketId!),
    enabled: open && isAdmin && ticketId !== null,
    staleTime: 60_000,
  });

  const buildingsQuery = useQuery({
    queryKey: queryKeys.buildings,
    queryFn: () => fetchBuildings(),
    enabled: open && isAdmin,
    staleTime: 5 * 60_000,
  });

  const problemTypesQuery = useQuery({
    queryKey: ['dictionaries', 'problem-types'],
    queryFn: fetchProblemTypesForSelection,
    enabled: open && ticketId !== null,
    staleTime: 5 * 60_000,
  });

  const update = useUpdateTicket();
  const remove = useDeleteTicket();

  // Сбрасываем локальное состояние при переходе к другой заявке.
  useEffect(() => {
    setEditing(false);
    setPendingBuildingId(null);
    setConfirmDelete(false);
  }, [ticketId]);

  useEffect(() => {
    if (!editing || !ticket) return;
    editForm.setFieldsValue({
      applicant_name: ticket.applicant_name,
      cabinet: ticket.cabinet,
      problem_type_id: ticket.problem_type_id,
      description: ticket.description,
    });
  }, [editing, editForm, ticket]);

  const patch = (payload: TicketUpdateRequest) => {
    if (ticketId === null) return;
    update.mutate({ id: ticketId, patch: payload });
  };

  const saveFields = async (values: EditValues) => {
    if (ticketId === null) return;
    try {
      await update.mutateAsync({ id: ticketId, patch: values });
      setEditing(false);
    } catch {
      // Тост и откат делает мутация.
    }
  };

  const executorOptions = (assignableQuery.data ?? []).map((candidate) => ({
    value: candidate.id,
    label: `${candidate.full_name} · ${candidate.role === 'ADMIN' ? 'администратор' : (candidate.building_name ?? 'без здания')}`,
  }));

  const buildingOptions = (buildingsQuery.data ?? []).map((building) => ({
    value: building.id,
    label: building.is_active ? building.name : `${building.name} · неактивно`,
  }));

  const targetBuilding = pendingBuildingId
    ? buildingsQuery.data?.find((building) => building.id === pendingBuildingId)
    : undefined;

  return (
    <Drawer
      open={open}
      onClose={onClose}
      width="min(720px, 100%)"
      title={
        ticket ? (
          <Flex align="center" gap={8} wrap>
            <span>Заявка №{ticket.id}</span>
            <StatusTag
              status={ticket.status}
              label={ticket.status_label}
              color={ticket.status_color}
            />
            {ticket.is_anonymous ? <Tag>аноним</Tag> : null}
          </Flex>
        ) : (
          'Заявка'
        )
      }
      extra={
        <Space size={4}>
          <Tooltip title="Обновить">
            <Button
              type="text"
              icon={<ReloadOutlined />}
              onClick={() => void ticketQuery.refetch()}
              aria-label="Обновить заявку"
            />
          </Tooltip>
          {ticket ? (
            <ExportButton
              path={ticketExportPath(ticket.id)}
              filename={`ticket_${ticket.id}.docx`}
              label="Word"
              size="small"
            />
          ) : null}
        </Space>
      }
      data-testid="ticket-drawer"
    >
      {ticketQuery.isError ? (
        <Alert
          type="error"
          showIcon
          message="Не удалось открыть заявку"
          description={
            isApiError(ticketQuery.error) ? ticketQuery.error.userMessage : 'Попробуйте ещё раз'
          }
          action={
            <Button size="small" onClick={() => void ticketQuery.refetch()}>
              Повторить
            </Button>
          }
        />
      ) : null}

      {!ticket && ticketQuery.isLoading ? <Skeleton active paragraph={{ rows: 8 }} /> : null}

      {ticket ? (
        <Flex vertical gap={12} data-testid="ticket-drawer-content">
          <Flex gap={8} wrap align="center">
            <BuildingBadge buildingId={ticket.building_id} buildingName={ticket.building_name} />
            <DeadlineBadge deadline={ticket.deadline} isOverdue={ticket.is_overdue} />
            {ticket.completed_at ? (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Завершена: {dayjs(ticket.completed_at).format('DD.MM.YYYY HH:mm')}
              </Typography.Text>
            ) : null}
          </Flex>

          {update.data?.executor_reset ? (
            <Alert
              type="warning"
              showIcon
              message="Исполнитель снят"
              description="Он относится к другому зданию и не может вести заявку после смены здания"
              data-testid="executor-reset-notice"
            />
          ) : null}

          <Card size="small" title="Управление" data-testid="ticket-controls">
            <Flex vertical gap={12}>
              <Flex align="center" gap={12} wrap>
                <Typography.Text type="secondary" style={{ minWidth: 96 }}>
                  Статус
                </Typography.Text>
                <div data-testid="drawer-status">
                  <Segmented<TicketStatus>
                    value={ticket.status}
                    disabled={!permissions.canChangeStatus || update.isPending}
                    options={statuses.map((status) => ({
                      value: status.code,
                      label: status.label,
                    }))}
                    onChange={(value) => patch({ status: value })}
                  />
                </div>
                {!permissions.canChangeStatus ? (
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    Статус меняют сотрудники здания и администратор
                  </Typography.Text>
                ) : null}
              </Flex>

              <Flex align="center" gap={12} wrap>
                <Typography.Text type="secondary" style={{ minWidth: 96 }}>
                  Исполнитель
                </Typography.Text>
                {isAdmin ? (
                  <Select
                    value={ticket.executor_id ?? undefined}
                    options={executorOptions}
                    allowClear
                    showSearch
                    optionFilterProp="label"
                    placeholder="Не назначен"
                    loading={assignableQuery.isLoading}
                    onChange={(value) =>
                      patch({ executor_id: value === undefined ? null : Number(value) })
                    }
                    disabled={update.isPending}
                    style={{ minWidth: 280, maxWidth: '100%' }}
                    data-testid="drawer-executor-select"
                    aria-label="Исполнитель заявки"
                  />
                ) : (
                  <Flex align="center" gap={8} wrap>
                    <Typography.Text data-testid="drawer-executor-name">
                      {ticket.executor_name ?? 'Не назначен'}
                    </Typography.Text>
                    {permissions.canAssignSelf ? (
                      <Button
                        size="small"
                        icon={<UserAddOutlined />}
                        loading={update.isPending}
                        onClick={() => patch({ executor_id: user?.id ?? null })}
                        data-testid="assign-self"
                      >
                        Назначить себя
                      </Button>
                    ) : null}
                    {ticket.executor_id === user?.id ? (
                      <Tag color="green">вы исполнитель</Tag>
                    ) : null}
                  </Flex>
                )}
              </Flex>

              <Flex align="center" gap={12} wrap>
                <Typography.Text type="secondary" style={{ minWidth: 96 }}>
                  Дедлайн
                </Typography.Text>
                {permissions.canChangeDeadline ? (
                  <DatePicker
                    value={
                      ticket.deadline && dayjs(ticket.deadline).isValid()
                        ? dayjs(ticket.deadline)
                        : null
                    }
                    format="DD.MM.YYYY"
                    allowClear
                    placeholder="Без дедлайна"
                    onChange={(value) =>
                      patch({
                        deadline: value && value.isValid() ? value.format('YYYY-MM-DD') : null,
                      })
                    }
                    disabled={update.isPending}
                    data-testid="drawer-deadline"
                    aria-label="Дедлайн заявки"
                  />
                ) : (
                  <Flex align="center" gap={8}>
                    <DeadlineBadge deadline={ticket.deadline} isOverdue={ticket.is_overdue} />
                    <Tooltip title="Дедлайн меняет только администратор">
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                        только ADMIN
                      </Typography.Text>
                    </Tooltip>
                  </Flex>
                )}
              </Flex>

              <Flex align="center" gap={12} wrap>
                <Typography.Text type="secondary" style={{ minWidth: 96 }}>
                  Здание
                </Typography.Text>
                {permissions.canChangeBuilding ? (
                  <Select
                    value={ticket.building_id}
                    options={buildingOptions}
                    loading={buildingsQuery.isLoading}
                    showSearch
                    optionFilterProp="label"
                    onChange={(value) => setPendingBuildingId(Number(value))}
                    disabled={update.isPending || ticket.building_id === undefined}
                    style={{ minWidth: 280, maxWidth: '100%' }}
                    data-testid="drawer-building-select"
                    aria-label="Здание заявки"
                  />
                ) : (
                  <BuildingBadge
                    buildingId={ticket.building_id}
                    buildingName={ticket.building_name}
                  />
                )}
              </Flex>
            </Flex>
          </Card>

          <Card
            size="small"
            title="Данные заявки"
            extra={
              permissions.canEditFields ? (
                <Button
                  size="small"
                  type="text"
                  icon={<EditOutlined />}
                  onClick={() => setEditing((value) => !value)}
                  data-testid="toggle-edit"
                >
                  {editing ? 'Отменить' : 'Изменить'}
                </Button>
              ) : (
                <Tooltip title="Править поля может исполнитель заявки или администратор">
                  <Button size="small" type="text" icon={<EditOutlined />} disabled>
                    Изменить
                  </Button>
                </Tooltip>
              )
            }
          >
            {editing ? (
              <Form<EditValues>
                form={editForm}
                layout="vertical"
                onFinish={(values) => void saveFields(values)}
              >
                <Form.Item
                  name="applicant_name"
                  label="Заявитель"
                  rules={[
                    { required: true, message: 'Укажите заявителя' },
                    { max: 120, message: 'Не более 120 символов' },
                  ]}
                >
                  <Input aria-label="Заявитель" />
                </Form.Item>
                <Form.Item
                  name="cabinet"
                  label="Кабинет"
                  rules={[
                    { required: true, message: 'Укажите кабинет' },
                    { max: 32, message: 'Не более 32 символов' },
                  ]}
                >
                  <Input aria-label="Кабинет" />
                </Form.Item>
                <Form.Item
                  name="problem_type_id"
                  label="Вид проблемы"
                  rules={[{ required: true, message: 'Выберите вид проблемы' }]}
                >
                  <Select
                    options={(problemTypesQuery.data ?? []).map((type) => ({
                      value: type.id,
                      label: type.name,
                    }))}
                    loading={problemTypesQuery.isLoading}
                    aria-label="Вид проблемы"
                  />
                </Form.Item>
                <Form.Item
                  name="description"
                  label="Описание"
                  rules={[
                    { required: true, message: 'Добавьте описание' },
                    { max: 5000, message: 'Не более 5000 символов' },
                  ]}
                >
                  <Input.TextArea autoSize={{ minRows: 3, maxRows: 8 }} aria-label="Описание" />
                </Form.Item>
                <Space>
                  <Button
                    type="primary"
                    htmlType="submit"
                    icon={<SaveOutlined />}
                    loading={update.isPending}
                    data-testid="save-edit"
                  >
                    Сохранить
                  </Button>
                  <Button icon={<StopOutlined />} onClick={() => setEditing(false)}>
                    Отмена
                  </Button>
                </Space>
              </Form>
            ) : (
              <Descriptions column={1} size="small" bordered>
                <Descriptions.Item label="Заявитель">{ticket.applicant_name}</Descriptions.Item>
                <Descriptions.Item label="Кабинет">{ticket.cabinet}</Descriptions.Item>
                <Descriptions.Item label="Вид проблемы">
                  {ticket.problem_type_name}
                </Descriptions.Item>
                <Descriptions.Item label="Исполнитель">
                  {ticket.executor_name ?? '—'}
                </Descriptions.Item>
                <Descriptions.Item label="Создана">
                  {dayjs(ticket.created_at).format('DD.MM.YYYY HH:mm')}
                </Descriptions.Item>
                <Descriptions.Item label="Описание">
                  <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', margin: 0 }}>
                    {ticket.description}
                  </Typography.Paragraph>
                </Descriptions.Item>
              </Descriptions>
            )}
          </Card>

          <TicketComments ticketId={ticket.id} canComment={permissions.canComment} />

          {permissions.canDelete ? (
            <Card size="small" title="Опасная зона">
              <Flex justify="space-between" align="center" gap={8} wrap>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  Заявка попадёт в корзину и будет доступна для восстановления 90 дней.
                </Typography.Text>
                <Button
                  danger
                  icon={<DeleteOutlined />}
                  onClick={() => setConfirmDelete(true)}
                  data-testid="delete-ticket"
                >
                  В корзину
                </Button>
              </Flex>
            </Card>
          ) : null}
        </Flex>
      ) : null}

      <ConfirmDialog
        open={pendingBuildingId !== null}
        title="Сменить здание заявки?"
        description={
          targetBuilding
            ? `Заявка переедет в «${targetBuilding.name}».`
            : 'Здание заявки будет изменено.'
        }
        warning={
          willResetExecutor(ticket ?? null, pendingBuildingId)
            ? `Исполнитель ${ticket?.executor_name ?? ''} относится к другому зданию и будет снят автоматически (TICKET-9).`
            : undefined
        }
        okText="Сменить здание"
        loading={update.isPending}
        onCancel={() => setPendingBuildingId(null)}
        onConfirm={async () => {
          if (pendingBuildingId === null) return;
          await update.mutateAsync({
            id: ticketId ?? 0,
            patch: { building_id: pendingBuildingId },
          });
          setPendingBuildingId(null);
        }}
      />

      <ConfirmDialog
        open={confirmDelete}
        title={`Удалить заявку №${ticketId ?? ''} в корзину?`}
        description="Заявка исчезнет из доски. Восстановить её можно в разделе «Корзина» в течение 90 дней."
        okText="В корзину"
        danger
        loading={remove.isPending}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={async () => {
          if (ticketId === null) return;
          await remove.mutateAsync(ticketId);
          setConfirmDelete(false);
          onClose();
        }}
      />
    </Drawer>
  );
}
