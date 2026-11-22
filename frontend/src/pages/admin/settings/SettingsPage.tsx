/**
 * `/admin/settings` — настройки приложения, календарь и статусы (Ф6, CAL-1…5).
 *
 * Один экран с секциями: основные параметры, рабочие дни и праздники, часовой
 * пояс с живыми часами в выбранном поясе, время напоминаний и подписи/цвета
 * четырёх фиксированных статусов с живым превью `StatusTag`.
 * Ошибки 422 раскладываются по полям формы (`details` из ответа API),
 * а 409/сетевые — показываются общим баннером.
 */
import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Checkbox,
  Flex,
  Form,
  Input,
  InputNumber,
  Modal,
  Space,
  Switch,
  Table,
  Typography,
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import dayjs from 'dayjs';
import {
  createHoliday,
  deleteHoliday,
  fetchHolidays,
  fetchSettings,
  updateHoliday,
  updateSettings,
} from '@/api/admin';
import type {
  AppSettings,
  AppSettingsUpdateRequest,
  Holiday,
  TicketStatus,
  Weekday,
} from '@/api/types';
import { TICKET_STATUS_COLORS, TICKET_STATUS_LABELS, TICKET_STATUS_ORDER } from '@/app/config';
import { queryKeys } from '@/app/queryClient';
import { toast } from '@/app/toast';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { PageHeader } from '@/components/PageHeader';
import { StatusTag } from '@/components/StatusTag';
import {
  TIMEZONE_SUGGESTIONS,
  WEEKDAY_OPTIONS,
  adminErrorMessage,
  applyApiFieldErrors,
  formatTimeInZone,
  isKnownTimeZone,
} from '../shared';

interface StatusFormValue {
  code: TicketStatus;
  label: string;
  color: string;
}

interface SettingsFormValues {
  app_name: string;
  deadline_days: number | null;
  workdays: Weekday[];
  timezone: string;
  deadline_reminder_time: string;
  push_enabled: boolean;
  statuses: StatusFormValue[];
}

const FIELD_NAMES = [
  'app_name',
  'deadline_days',
  'workdays',
  'timezone',
  'deadline_reminder_time',
  'push_enabled',
  'statuses',
] as const;

/** Порядок статусов фиксирован (TICKET-1), поэтому нормализуем ответ сервера. */
function toFormValues(settings: AppSettings): SettingsFormValues {
  const statuses = TICKET_STATUS_ORDER.map((code) => {
    const stored = settings.statuses.find((status) => status.code === code);
    return {
      code,
      label: stored?.label ?? TICKET_STATUS_LABELS[code],
      color: stored?.color ?? TICKET_STATUS_COLORS[code],
    };
  });
  return {
    app_name: settings.app_name,
    deadline_days: settings.deadline_days,
    workdays: settings.workdays,
    timezone: settings.timezone,
    deadline_reminder_time: settings.deadline_reminder_time,
    push_enabled: settings.push_enabled,
    statuses,
  };
}

/** Живые часы в выбранном поясе (CAL-2): видно, что зона реально применяется. */
function CurrentTimeInZone({ timezone }: { timezone: string }) {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  if (!isKnownTimeZone(timezone)) {
    return (
      <Typography.Text
        type="danger"
        style={{ fontSize: 12 }}
        data-testid="settings-timezone-invalid"
      >
        Неизвестный часовой пояс — нужен IANA-идентификатор
      </Typography.Text>
    );
  }

  return (
    <Typography.Text type="secondary" style={{ fontSize: 12 }} data-testid="settings-current-time">
      Сейчас в поясе {timezone}: {formatTimeInZone(now, timezone)}
    </Typography.Text>
  );
}

function StatusRow({ index, code }: { index: number; code: TicketStatus }) {
  const form = Form.useFormInstance<SettingsFormValues>();
  const label = Form.useWatch(['statuses', index, 'label'], form) as string | undefined;
  const color = Form.useWatch(['statuses', index, 'color'], form) as string | undefined;

  return (
    <Flex gap={8} align="flex-start" wrap data-testid={`status-row-${code}`}>
      <Form.Item name={['statuses', index, 'code']} hidden>
        <Input />
      </Form.Item>
      <Form.Item label="Превью" style={{ marginBottom: 0 }}>
        <div style={{ paddingTop: 4 }}>
          <StatusTag status={code} label={label} color={color} />
        </div>
      </Form.Item>
      <Form.Item
        name={['statuses', index, 'label']}
        label="Название"
        style={{ minWidth: 180, flex: '1 1 200px', marginBottom: 0 }}
        rules={[{ required: true, message: 'Укажите название статуса' }]}
      >
        <Input
          maxLength={40}
          data-testid={`status-label-${code}`}
          aria-label={`Название статуса ${code}`}
        />
      </Form.Item>
      <Form.Item
        name={['statuses', index, 'color']}
        label="Цвет (HEX)"
        style={{ width: 170, marginBottom: 0 }}
        rules={[{ pattern: /^#[0-9a-fA-F]{6}$/, message: 'HEX вида #RRGGBB' }]}
      >
        <Input
          prefix={
            <span
              aria-hidden
              data-testid={`status-color-swatch-${code}`}
              style={{
                display: 'inline-block',
                width: 14,
                height: 14,
                borderRadius: 3,
                background: /^#[0-9a-fA-F]{6}$/.test(color ?? '') ? color : '#d9d9d9',
              }}
            />
          }
          data-testid={`status-color-${code}`}
          aria-label={`Цвет статуса ${code}`}
        />
      </Form.Item>
    </Flex>
  );
}

function HolidayEditModal({
  open,
  holiday,
  pending,
  onCancel,
  onSubmit,
}: {
  open: boolean;
  holiday: Holiday | null;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (values: { date: string; name: string }) => void;
}) {
  const [date, setDate] = useState(holiday?.date ?? '');
  const [name, setName] = useState(holiday?.name ?? '');

  return (
    <Modal
      open={open}
      title="Изменить праздник"
      okText="Сохранить"
      cancelText="Отмена"
      confirmLoading={pending}
      onOk={() => onSubmit({ date, name })}
      onCancel={onCancel}
      destroyOnHidden
      data-testid="holiday-edit-modal"
    >
      <Flex vertical gap={10} style={{ paddingTop: 8 }}>
        <Input
          type="date"
          value={date}
          onChange={(event) => setDate(event.target.value)}
          data-testid="holiday-edit-date"
          aria-label="Дата праздника"
        />
        <Input
          value={name}
          onChange={(event) => setName(event.target.value)}
          maxLength={120}
          placeholder="Название праздника"
          data-testid="holiday-edit-name"
          aria-label="Название праздника"
        />
      </Flex>
    </Modal>
  );
}

function HolidaysSection() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: queryKeys.holidays, queryFn: fetchHolidays });
  const [date, setDate] = useState('');
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Holiday | null>(null);
  const [editTarget, setEditTarget] = useState<Holiday | null>(null);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.holidays });
    void queryClient.invalidateQueries({ queryKey: queryKeys.settings });
  };

  const createMutation = useMutation({
    mutationFn: () => createHoliday({ date, name: name.trim() }),
    onSuccess: () => {
      setDate('');
      setName('');
      setError(null);
      toast.success('Праздник добавлен');
    },
    onError: (cause) => setError(adminErrorMessage(cause, 'Не удалось добавить праздник')),
    onSettled: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteHoliday(id),
    onSuccess: () => toast.success('Праздник удалён'),
    onError: (cause) => setError(adminErrorMessage(cause, 'Не удалось удалить праздник')),
    onSettled: invalidate,
  });

  const editMutation = useMutation({
    mutationFn: (values: { id: number; date: string; name: string }) =>
      updateHoliday(values.id, { date: values.date, name: values.name }),
    onSuccess: () => {
      setEditTarget(null);
      toast.success('Праздник обновлён');
    },
    onError: (cause) => setError(adminErrorMessage(cause, 'Не удалось изменить праздник')),
    onSettled: invalidate,
  });

  const addHoliday = () => {
    if (!date) {
      setError('Укажите дату праздника');
      return;
    }
    if (!dayjs(date, 'YYYY-MM-DD', true).isValid()) {
      setError('Дата праздника в формате YYYY-MM-DD');
      return;
    }
    createMutation.mutate();
  };

  return (
    <Card
      size="small"
      title="Праздники"
      data-testid="holidays-section"
      extra={
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Исключаются из рабочих дней (CAL-4)
        </Typography.Text>
      }
    >
      <Flex vertical gap={12}>
        <Flex gap={8} wrap align="end">
          <Flex vertical gap={2}>
            <Typography.Text style={{ fontSize: 12 }}>Дата</Typography.Text>
            <Input
              type="date"
              value={date}
              onChange={(event) => setDate(event.target.value)}
              style={{ width: 170 }}
              data-testid="holiday-date"
              aria-label="Дата праздника"
            />
          </Flex>
          <Flex vertical gap={2} style={{ flex: '1 1 220px' }}>
            <Typography.Text style={{ fontSize: 12 }}>Название</Typography.Text>
            <Input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Например, День города"
              maxLength={120}
              data-testid="holiday-name"
              aria-label="Название праздника"
            />
          </Flex>
          <Button
            type="primary"
            icon={<PlusOutlined />}
            loading={createMutation.isPending}
            onClick={addHoliday}
            data-testid="holiday-add"
          >
            Добавить
          </Button>
        </Flex>

        {error ? (
          <Alert
            type="error"
            showIcon
            closable
            message={error}
            data-testid="holiday-error"
            onClose={() => setError(null)}
          />
        ) : null}

        <Table<Holiday>
          rowKey="id"
          size="small"
          data-testid="holidays-table"
          loading={query.isLoading}
          pagination={false}
          dataSource={[...(query.data ?? [])].sort((a, b) => a.date.localeCompare(b.date))}
          locale={{ emptyText: 'Праздников нет' }}
          columns={[
            {
              title: 'Дата',
              dataIndex: 'date',
              key: 'date',
              width: 130,
              render: (value: string) => (
                <Typography.Text>
                  {dayjs(value).isValid() ? dayjs(value).format('DD.MM.YYYY') : value}
                </Typography.Text>
              ),
            },
            {
              title: 'Название',
              dataIndex: 'name',
              key: 'name',
              // Контракт отдаёт только дату; подпись — аддитивное поле.
              render: (value: string | undefined) => (
                <Typography.Text type={value ? undefined : 'secondary'}>
                  {value ?? 'Праздник'}
                </Typography.Text>
              ),
            },
            {
              title: '',
              key: 'actions',
              width: 110,
              render: (_value, holiday) => (
                <Space size={2}>
                  <Button
                    size="small"
                    type="text"
                    icon={<EditOutlined />}
                    aria-label={`Изменить праздник ${holiday.name ?? holiday.date}`}
                    data-testid={`holiday-edit-${holiday.id}`}
                    onClick={() => setEditTarget(holiday)}
                  />
                  <Button
                    size="small"
                    danger
                    type="text"
                    icon={<DeleteOutlined />}
                    aria-label={`Удалить праздник ${holiday.name ?? holiday.date}`}
                    data-testid={`holiday-delete-${holiday.id}`}
                    onClick={() => setDeleteTarget(holiday)}
                  />
                </Space>
              ),
            },
          ]}
        />

        <ConfirmDialog
          open={deleteTarget !== null}
          title="Удалить праздник?"
          description={
            deleteTarget
              ? `«${deleteTarget.name ?? 'Праздник'}» (${dayjs(deleteTarget.date).format('DD.MM.YYYY')}) снова станет рабочим днём.`
              : undefined
          }
          okText="Удалить"
          danger
          loading={deleteMutation.isPending}
          onConfirm={() => {
            const target = deleteTarget;
            setDeleteTarget(null);
            if (target) deleteMutation.mutate(target.id);
          }}
          onCancel={() => setDeleteTarget(null)}
        />

        <HolidayEditModal
          key={editTarget?.id ?? 'none'}
          open={editTarget !== null}
          holiday={editTarget}
          pending={editMutation.isPending}
          onCancel={() => setEditTarget(null)}
          onSubmit={(values) => {
            if (!editTarget) return;
            if (!dayjs(values.date, 'YYYY-MM-DD', true).isValid()) {
              setError('Дата праздника в формате YYYY-MM-DD');
              return;
            }
            editMutation.mutate({ id: editTarget.id, ...values });
          }}
        />
      </Flex>
    </Card>
  );
}

export function SettingsPage() {
  const queryClient = useQueryClient();
  const [form] = Form.useForm<SettingsFormValues>();
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const settingsQuery = useQuery({ queryKey: queryKeys.settings, queryFn: fetchSettings });

  useEffect(() => {
    if (settingsQuery.data) form.setFieldsValue(toFormValues(settingsQuery.data));
  }, [settingsQuery.data, form]);

  const watchedTimezone = Form.useWatch('timezone', form);
  const timezone = watchedTimezone ?? settingsQuery.data?.timezone ?? '';

  const timezoneHint = useMemo(
    () => (
      <Space direction="vertical" size={2} style={{ marginTop: 4 }}>
        <CurrentTimeInZone timezone={timezone} />
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          IANA-идентификатор, например {TIMEZONE_SUGGESTIONS[0]} (CAL-2). В этом поясе считаются
          дедлайны и напоминания.
        </Typography.Text>
      </Space>
    ),
    [timezone],
  );

  const onFinish = async (values: SettingsFormValues) => {
    setSaveError(null);
    setSaving(true);
    const payload: AppSettingsUpdateRequest = {
      app_name: values.app_name.trim(),
      deadline_days: Number(values.deadline_days),
      workdays: values.workdays,
      timezone: values.timezone.trim(),
      deadline_reminder_time: values.deadline_reminder_time.trim(),
      push_enabled: values.push_enabled,
      statuses: values.statuses.map((status) => ({
        code: status.code,
        label: status.label.trim(),
        color: status.color.trim(),
      })),
    };
    try {
      const updated = await updateSettings(payload);
      queryClient.setQueryData(queryKeys.settings, updated);
      void queryClient.invalidateQueries({ queryKey: queryKeys.settings });
      void queryClient.invalidateQueries({ queryKey: queryKeys.holidays });
      setSaveError(null);
      toast.success('Настройки сохранены');
    } catch (error) {
      const applied = applyApiFieldErrors(form, error, FIELD_NAMES);
      if (!applied.length) {
        setSaveError(adminErrorMessage(error, 'Не удалось сохранить настройки'));
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <Flex vertical gap={16} data-testid="settings-page">
      <PageHeader
        title="Настройки"
        description="Название приложения, календарь, часовой пояс, напоминания и статусы заявок."
        extra={
          <Space size={8} wrap>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => {
                if (settingsQuery.data) form.setFieldsValue(toFormValues(settingsQuery.data));
                setSaveError(null);
              }}
              data-testid="settings-reset"
            >
              Отменить изменения
            </Button>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={saving}
              onClick={() => void form.submit()}
              data-testid="settings-save"
            >
              Сохранить
            </Button>
          </Space>
        }
      />

      {settingsQuery.error ? (
        <Alert
          type="error"
          showIcon
          message={adminErrorMessage(settingsQuery.error, 'Не удалось загрузить настройки')}
          data-testid="settings-load-error"
        />
      ) : null}

      {saveError ? (
        <Alert
          type="error"
          showIcon
          closable
          message={saveError}
          data-testid="settings-error"
          onClose={() => setSaveError(null)}
        />
      ) : null}

      <Form<SettingsFormValues>
        form={form}
        layout="vertical"
        onFinish={(values) => void onFinish(values)}
        data-testid="settings-form"
        disabled={settingsQuery.isLoading}
      >
        <Flex vertical gap={16}>
          <Card size="small" title="Основное">
            <Flex gap={12} wrap>
              <Form.Item
                name="app_name"
                label="Название приложения"
                style={{ minWidth: 260, flex: '1 1 280px' }}
                rules={[{ required: true, message: 'Укажите название приложения' }]}
              >
                <Input maxLength={120} data-testid="settings-app-name" />
              </Form.Item>
              <Form.Item
                name="deadline_days"
                label="Дедлайн, рабочих дней"
                style={{ width: 220 }}
                extra="От 1 до 30 (CAL-3, TICKET-3)"
              >
                <InputNumber
                  style={{ width: '100%' }}
                  step={1}
                  data-testid="settings-deadline-days"
                  aria-label="Срок дедлайна в рабочих днях"
                />
              </Form.Item>
              <Form.Item
                name="push_enabled"
                label="Push-уведомления"
                valuePropName="checked"
                style={{ width: 200 }}
                extra="Глобальный выключатель (NOTIF-6)"
              >
                <Switch
                  checkedChildren="включены"
                  unCheckedChildren="выключены"
                  data-testid="settings-push-enabled"
                />
              </Form.Item>
            </Flex>
          </Card>

          <Card size="small" title="Рабочий календарь">
            <Form.Item
              name="workdays"
              label="Рабочие дни"
              style={{ marginBottom: 8 }}
              rules={[
                {
                  validator: (_rule, value: Weekday[]) =>
                    Array.isArray(value) && value.length > 0
                      ? Promise.resolve()
                      : Promise.reject(new Error('Выберите хотя бы один рабочий день')),
                },
              ]}
              extra="По умолчанию Пн–Сб; выходные и праздники не считаются (CAL-1, CAL-4)"
            >
              <Checkbox.Group
                data-testid="settings-workdays"
                options={WEEKDAY_OPTIONS.map((day) => ({ value: day.value, label: day.short }))}
              />
            </Form.Item>
          </Card>

          <Card size="small" title="Время">
            <Flex gap={12} wrap>
              <Form.Item
                name="timezone"
                label="Часовой пояс (IANA)"
                style={{ minWidth: 280, flex: '1 1 320px' }}
                extra={timezoneHint}
              >
                <Input
                  placeholder={TIMEZONE_SUGGESTIONS[0]}
                  data-testid="settings-timezone"
                  aria-label="Часовой пояс"
                  list="settings-timezone-options"
                />
              </Form.Item>
              <datalist id="settings-timezone-options">
                {TIMEZONE_SUGGESTIONS.map((zone) => (
                  <option key={zone} value={zone} />
                ))}
              </datalist>
              <Form.Item
                name="deadline_reminder_time"
                label="Время напоминаний"
                style={{ width: 200 }}
                extra="HH:MM в выбранном поясе (CAL-3); сервер проверит формат"
              >
                <Input
                  placeholder="10:00"
                  data-testid="settings-reminder-time"
                  aria-label="Время напоминаний"
                />
              </Form.Item>
            </Flex>
          </Card>

          <Card size="small" title="Статусы заявок">
            <Flex vertical gap={12}>
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Коды статусов фиксированы (TICKET-1), названия и цвета настраиваются. Превью
                обновляется сразу.
              </Typography.Text>
              {TICKET_STATUS_ORDER.map((code, index) => (
                <StatusRow key={code} index={index} code={code} />
              ))}
            </Flex>
          </Card>
        </Flex>
      </Form>

      <HolidaysSection />
    </Flex>
  );
}
