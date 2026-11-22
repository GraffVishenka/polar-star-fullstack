/**
 * Форма создания заявки (TICKET-2..TICKET-4, Ф2).
 *
 * USER создаёт только в своём здании (поле зафиксировано), ADMIN выбирает любое
 * активное здание. Валидация дублирует серверную: 403/404/422 показываем тостом.
 */
import { useEffect, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Flex, Form, Input, Modal, Select, Typography } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import { z } from 'zod';
import { createTicket } from '@/api/tickets';
import { fetchBuildings } from '@/api/equipment';
import { fetchProblemTypesForSelection } from '@/api/dictionaries';
import type { Ticket } from '@/api/types';
import { isApiError } from '@/api/errors';
import { queryKeys } from '@/app/queryClient';
import { toast } from '@/app/toast';
import { useAuth } from '@/auth/AuthContext';

const createTicketSchema = z.object({
  building_id: z.number().int().positive().optional(),
  applicant_name: z.string().trim().min(1, 'Укажите заявителя').max(120, 'Не более 120 символов'),
  cabinet: z.string().trim().min(1, 'Укажите кабинет').max(32, 'Не более 32 символов'),
  problem_type_id: z
    .number({ error: 'Выберите вид проблемы' })
    .int()
    .positive('Выберите вид проблемы'),
  description: z.string().trim().min(1, 'Добавьте описание').max(5000, 'Не более 5000 символов'),
});

type CreateTicketInput = z.input<typeof createTicketSchema>;
type CreateTicketValues = z.output<typeof createTicketSchema>;

export interface TicketCreateModalProps {
  open: boolean;
  onClose: () => void;
  onCreated: (ticket: Ticket) => void;
}

export function TicketCreateModal({ open, onClose, onCreated }: TicketCreateModalProps) {
  const { user, isAdmin } = useAuth();
  const qc = useQueryClient();
  const [formError, setFormError] = useState<string | null>(null);

  const buildingsQuery = useQuery({
    queryKey: queryKeys.buildings,
    queryFn: () => fetchBuildings({ is_active: true }),
    enabled: open && isAdmin,
    staleTime: 5 * 60_000,
  });

  const problemTypesQuery = useQuery({
    queryKey: ['dictionaries', 'problem-types'],
    queryFn: fetchProblemTypesForSelection,
    enabled: open,
    staleTime: 5 * 60_000,
  });

  const {
    handleSubmit,
    control,
    reset,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<CreateTicketInput, unknown, CreateTicketValues>({
    resolver: zodResolver(createTicketSchema),
    defaultValues: {
      building_id: undefined,
      applicant_name: '',
      cabinet: '',
      problem_type_id: undefined,
      description: '',
    },
  });

  useEffect(() => {
    if (!open) return;
    setFormError(null);
    reset({
      building_id: isAdmin ? undefined : (user?.building_id ?? undefined),
      applicant_name: user?.full_name ?? '',
      cabinet: '',
      problem_type_id: undefined,
      description: '',
    });
  }, [open, isAdmin, reset, user?.building_id, user?.full_name]);

  const onSubmit = async (values: CreateTicketValues) => {
    setFormError(null);
    const buildingId = isAdmin ? values.building_id : (user?.building_id ?? undefined);
    if (!buildingId) {
      setError('building_id', { message: 'Выберите здание' });
      return;
    }

    try {
      const ticket = await createTicket({
        building_id: buildingId,
        applicant_name: values.applicant_name,
        cabinet: values.cabinet,
        problem_type_id: values.problem_type_id,
        description: values.description,
      });
      toast.success(`Заявка №${ticket.id} создана`);
      await qc.invalidateQueries({ queryKey: ['tickets'] });
      onCreated(ticket);
      reset();
      onClose();
    } catch (error) {
      // 403/404/422 — понятный тост, форма остаётся открытой.
      const message = isApiError(error) ? error.userMessage : 'Не удалось создать заявку';
      setFormError(message);
      toast.error(message);
    }
  };

  return (
    <Modal
      open={open}
      title="Новая заявка"
      onCancel={onClose}
      footer={null}
      width={620}
      data-testid="ticket-create-modal"
    >
      <Flex vertical gap={8}>
        {formError ? (
          <Alert type="error" showIcon message={formError} data-testid="create-error" />
        ) : null}

        <form onSubmit={handleSubmit(onSubmit)} noValidate>
          {isAdmin ? (
            <Form.Item
              label="Здание"
              required
              validateStatus={errors.building_id ? 'error' : undefined}
              help={errors.building_id?.message}
            >
              <Controller
                name="building_id"
                control={control}
                render={({ field }) => (
                  <Select
                    value={field.value as number | undefined}
                    onChange={field.onChange}
                    options={(buildingsQuery.data ?? []).map((building) => ({
                      value: building.id,
                      label: building.name,
                    }))}
                    loading={buildingsQuery.isLoading}
                    placeholder="Выберите активное здание"
                    showSearch
                    optionFilterProp="label"
                    aria-label="Здание"
                    data-testid="create-building"
                  />
                )}
              />
            </Form.Item>
          ) : (
            <Form.Item label="Здание">
              <Input
                value={user?.building_name ?? 'Здание не назначено'}
                disabled
                data-testid="create-building-fixed"
                aria-label="Здание"
              />
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Сотрудник создаёт заявки только в своём здании (TICKET-7).
              </Typography.Text>
            </Form.Item>
          )}

          <Form.Item
            label="Заявитель"
            required
            validateStatus={errors.applicant_name ? 'error' : undefined}
            help={errors.applicant_name?.message}
          >
            <Controller
              name="applicant_name"
              control={control}
              render={({ field }) => (
                <Input
                  {...field}
                  value={field.value ?? ''}
                  maxLength={120}
                  placeholder="ФИО или имя посетителя"
                  data-testid="create-applicant"
                />
              )}
            />
          </Form.Item>

          <Form.Item
            label="Кабинет"
            required
            validateStatus={errors.cabinet ? 'error' : undefined}
            help={errors.cabinet?.message}
          >
            <Controller
              name="cabinet"
              control={control}
              render={({ field }) => (
                <Input
                  {...field}
                  value={field.value ?? ''}
                  maxLength={32}
                  placeholder="каб. 204"
                  data-testid="create-cabinet"
                />
              )}
            />
          </Form.Item>

          <Form.Item
            label="Вид проблемы"
            required
            validateStatus={errors.problem_type_id ? 'error' : undefined}
            help={errors.problem_type_id?.message}
          >
            <Controller
              name="problem_type_id"
              control={control}
              render={({ field }) => (
                <Select
                  value={field.value as number | undefined}
                  onChange={field.onChange}
                  options={(problemTypesQuery.data ?? []).map((type) => ({
                    value: type.id,
                    label: type.name,
                  }))}
                  loading={problemTypesQuery.isLoading}
                  placeholder="Выберите вид проблемы"
                  aria-label="Вид проблемы"
                  data-testid="create-problem-type"
                />
              )}
            />
          </Form.Item>

          <Form.Item
            label="Описание"
            required
            validateStatus={errors.description ? 'error' : undefined}
            help={errors.description?.message}
          >
            <Controller
              name="description"
              control={control}
              render={({ field }) => (
                <Input.TextArea
                  {...field}
                  value={field.value ?? ''}
                  autoSize={{ minRows: 3, maxRows: 8 }}
                  maxLength={5000}
                  showCount
                  placeholder="Что случилось и где"
                  data-testid="create-description"
                />
              )}
            />
          </Form.Item>

          <Flex justify="flex-end" gap={8}>
            <Button onClick={onClose}>Отмена</Button>
            <Button
              type="primary"
              htmlType="submit"
              icon={<PlusOutlined />}
              loading={isSubmitting}
              data-testid="create-submit"
            >
              Создать заявку
            </Button>
          </Flex>
        </form>
      </Flex>
    </Modal>
  );
}
