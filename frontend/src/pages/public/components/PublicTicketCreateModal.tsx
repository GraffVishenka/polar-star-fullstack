/**
 * Публичная форма создания заявки (TICKET-4, PUB-5).
 *
 * - здание подставлено из текущего дашборда и может быть изменено;
 * - после успеха показывается экран со ссылкой `/t/{token}` и кнопкой
 *   копирования (токен выдаётся один раз, TICKET-13);
 * - cookie выбранного здания обновляет вызывающая страница (`onCreated`),
 *   переход в дашборд — по кнопке (`onGoToDashboard`).
 */
import { useEffect, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Flex, Form, Input, Modal, Result, Select, Space, Typography } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { Link } from 'react-router-dom';
import { z } from 'zod';
import { createPublicTicket, fetchPublicProblemTypes, publicTicketPath } from '@/api/public';
import type { PublicTicketCreated } from '@/api/types';
import { isApiError } from '@/api/errors';
import { copyText } from '@/app/clipboard';
import { toast } from '@/app/toast';
import { DeadlineBadge } from '@/components/DeadlineBadge';

const createSchema = z.object({
  building_id: z.number({ error: 'Выберите здание' }).int().positive('Выберите здание'),
  applicant_name: z.string().trim().min(1, 'Укажите заявителя').max(120, 'Не более 120 символов'),
  cabinet: z.string().trim().min(1, 'Укажите кабинет').max(32, 'Не более 32 символов'),
  problem_type_id: z
    .number({ error: 'Выберите вид проблемы' })
    .int()
    .positive('Выберите вид проблемы'),
  description: z.string().trim().min(1, 'Добавьте описание').max(5000, 'Не более 5000 символов'),
});

type CreateInput = z.input<typeof createSchema>;
type CreateValues = z.output<typeof createSchema>;

export interface PublicTicketCreateModalProps {
  open: boolean;
  /** Здание текущего дашборда (подставляется в форму). */
  defaultBuildingId: number | null;
  buildings: Array<{ id: number; name: string }>;
  buildingsLoading?: boolean;
  onClose: () => void;
  /** Вызывается сразу после успешного создания (обновить cookie). */
  onCreated: (result: { created: PublicTicketCreated; buildingId: number }) => void;
  /** Переход в дашборд выбранного здания. */
  onGoToDashboard: (buildingId: number) => void;
}

export function PublicTicketCreateModal({
  open,
  defaultBuildingId,
  buildings,
  buildingsLoading,
  onClose,
  onCreated,
  onGoToDashboard,
}: PublicTicketCreateModalProps) {
  const qc = useQueryClient();
  const [created, setCreated] = useState<{
    created: PublicTicketCreated;
    buildingId: number;
  } | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const problemTypesQuery = useQuery({
    queryKey: ['public', 'problem-types'],
    queryFn: fetchPublicProblemTypes,
    enabled: open,
    staleTime: 5 * 60_000,
  });

  const {
    control,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<CreateInput, unknown, CreateValues>({
    resolver: zodResolver(createSchema),
    defaultValues: {
      building_id: defaultBuildingId ?? undefined,
      applicant_name: '',
      cabinet: '',
      problem_type_id: undefined,
      description: '',
    },
  });

  useEffect(() => {
    if (!open) return;
    setCreated(null);
    setFormError(null);
    reset({
      building_id: defaultBuildingId ?? undefined,
      applicant_name: '',
      cabinet: '',
      problem_type_id: undefined,
      description: '',
    });
  }, [open, defaultBuildingId, reset]);

  const onSubmit = async (values: CreateValues) => {
    setFormError(null);
    try {
      const result = await createPublicTicket({
        building_id: values.building_id,
        applicant_name: values.applicant_name,
        cabinet: values.cabinet,
        problem_type_id: values.problem_type_id,
        description: values.description,
      });
      setCreated({ created: result, buildingId: values.building_id });
      onCreated({ created: result, buildingId: values.building_id });
      // Cookie выбранного здания и данные доски обновляются сразу.
      await qc.invalidateQueries({ queryKey: ['public', 'buildings'] });
      await qc.invalidateQueries({ queryKey: ['public', 'tickets'] });
      toast.success(`Заявка №${result.id} создана`);
    } catch (error) {
      const message = isApiError(error) ? error.userMessage : 'Не удалось создать заявку';
      setFormError(message);
      toast.error(message);
    }
  };

  const shareUrl = created
    ? `${typeof window !== 'undefined' ? window.location.origin : ''}${publicTicketPath(created.created.access_token)}`
    : '';

  const handleCopy = async () => {
    const ok = await copyText(shareUrl);
    if (ok) toast.success('Ссылка скопирована');
    else toast.error('Не удалось скопировать ссылку — выделите её вручную');
  };

  return (
    <Modal
      open={open}
      title={created ? 'Заявка создана' : 'Новая заявка'}
      onCancel={onClose}
      footer={null}
      width={620}
      data-testid="public-create-modal"
    >
      {created ? (
        <Flex vertical gap={12} data-testid="public-create-success">
          <Result
            status="success"
            title={`Заявка №${created.created.id} создана`}
            subTitle="Сохраните ссылку: по ней доступны карточка, комментарии и Word. Токен выдаётся один раз."
          />
          <Flex vertical gap={6}>
            <Typography.Text strong>Ссылка на заявку</Typography.Text>
            <Typography.Text data-testid="public-create-link" copyable>
              {shareUrl}
            </Typography.Text>
            <Space wrap>
              <Button
                icon={<CopyOutlined />}
                onClick={() => void handleCopy()}
                data-testid="public-create-copy"
              >
                Скопировать ссылку
              </Button>
              <Link to={publicTicketPath(created.created.access_token)}>
                <Button type="link">Открыть карточку</Button>
              </Link>
            </Space>
          </Flex>
          <DeadlineBadge deadline={created.created.deadline} withLabel isOverdue={false} />
          <Flex justify="flex-end">
            <Button
              type="primary"
              onClick={() => onGoToDashboard(created.buildingId)}
              data-testid="public-create-go-dashboard"
            >
              Перейти в дашборд здания
            </Button>
          </Flex>
        </Flex>
      ) : (
        <Flex vertical gap={8}>
          {formError ? (
            <Alert type="error" showIcon message={formError} data-testid="public-create-error" />
          ) : null}

          <form onSubmit={handleSubmit(onSubmit)} noValidate>
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
                    options={buildings.map((building) => ({
                      value: building.id,
                      label: building.name,
                    }))}
                    loading={buildingsLoading}
                    placeholder="Выберите здание"
                    showSearch
                    optionFilterProp="label"
                    aria-label="Здание"
                    data-testid="public-create-building"
                  />
                )}
              />
            </Form.Item>

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
                    placeholder="Как к вам обращаться"
                    data-testid="public-create-applicant"
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
                    data-testid="public-create-cabinet"
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
                    data-testid="public-create-problem"
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
                    data-testid="public-create-description"
                  />
                )}
              />
            </Form.Item>

            <Flex justify="flex-end" gap={8}>
              <Button onClick={onClose}>Отмена</Button>
              <Button
                type="primary"
                htmlType="submit"
                loading={isSubmitting}
                data-testid="public-create-submit"
              >
                Создать заявку
              </Button>
            </Flex>
          </form>
        </Flex>
      )}
    </Modal>
  );
}
