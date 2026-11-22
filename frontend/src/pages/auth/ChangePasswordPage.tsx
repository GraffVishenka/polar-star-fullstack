/**
 * `/change-password` — обязательная/добровольная смена своего пароля (AUTH-4, AUTH-5).
 *
 * Пока `must_change_password=true`, остальные экраны закрыты guard'ом
 * `RequireAuth`: сервер отвечает 428 PASSWORD_CHANGE_REQUIRED.
 */
import { useState } from 'react';
import { LockOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Flex, Form, Input, Typography } from 'antd';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useNavigate } from 'react-router-dom';
import { z } from 'zod';
import { changeOwnPassword } from '@/api/auth';
import { isApiError } from '@/api/errors';
import { toast } from '@/app/toast';
import { useAuth } from '@/auth/AuthContext';

const passwordSchema = z
  .object({
    current_password: z.string().min(1, 'Введите текущий пароль'),
    new_password: z.string().min(8, 'Новый пароль — не короче 8 символов'),
    confirm_password: z.string().min(1, 'Повторите новый пароль'),
  })
  .refine((values) => values.new_password === values.confirm_password, {
    message: 'Пароли не совпадают',
    path: ['confirm_password'],
  })
  .refine((values) => values.current_password !== values.new_password, {
    message: 'Новый пароль должен отличаться от текущего',
    path: ['new_password'],
  });

type PasswordFormValues = z.infer<typeof passwordSchema>;

export function ChangePasswordPage() {
  const navigate = useNavigate();
  const { user, refreshUser } = useAuth();
  const [formError, setFormError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<PasswordFormValues>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { current_password: '', new_password: '', confirm_password: '' },
  });

  const onSubmit = async (values: PasswordFormValues) => {
    setFormError(null);
    try {
      await changeOwnPassword({
        current_password: values.current_password,
        new_password: values.new_password,
      });
      setDone(true);
      toast.success('Пароль изменён');
      await refreshUser();
      navigate('/tickets', { replace: true });
    } catch (error) {
      setFormError(isApiError(error) ? error.userMessage : 'Не удалось изменить пароль');
    }
  };

  return (
    <Card style={{ maxWidth: 520, margin: '0 auto' }} data-testid="change-password-page">
      <Typography.Title level={3} style={{ marginTop: 0 }}>
        Смена пароля
      </Typography.Title>
      <Typography.Paragraph type="secondary">
        {user?.must_change_password
          ? 'Вы вошли с временным паролем. Задайте новый пароль — после этого откроются остальные разделы.'
          : 'Новый пароль должен содержать не менее 8 символов.'}
      </Typography.Paragraph>

      {formError ? (
        <Alert type="error" showIcon message={formError} style={{ marginBottom: 16 }} />
      ) : null}
      {done ? (
        <Alert type="success" showIcon message="Пароль обновлён" style={{ marginBottom: 16 }} />
      ) : null}

      <form onSubmit={handleSubmit(onSubmit)} noValidate>
        <Form.Item
          label="Текущий пароль"
          validateStatus={errors.current_password ? 'error' : undefined}
          help={errors.current_password?.message}
        >
          <Input.Password
            {...register('current_password')}
            prefix={<LockOutlined />}
            autoComplete="current-password"
            aria-label="Текущий пароль"
          />
        </Form.Item>
        <Form.Item
          label="Новый пароль"
          validateStatus={errors.new_password ? 'error' : undefined}
          help={errors.new_password?.message}
        >
          <Input.Password
            {...register('new_password')}
            prefix={<LockOutlined />}
            autoComplete="new-password"
            aria-label="Новый пароль"
          />
        </Form.Item>
        <Form.Item
          label="Повторите новый пароль"
          validateStatus={errors.confirm_password ? 'error' : undefined}
          help={errors.confirm_password?.message}
        >
          <Input.Password
            {...register('confirm_password')}
            prefix={<LockOutlined />}
            autoComplete="new-password"
            aria-label="Повторите новый пароль"
          />
        </Form.Item>
        <Flex gap={8} wrap>
          <Button type="primary" htmlType="submit" loading={isSubmitting}>
            Сохранить пароль
          </Button>
          {!user?.must_change_password ? (
            <Button onClick={() => navigate(-1)}>Отмена</Button>
          ) : null}
        </Flex>
      </form>
    </Card>
  );
}
