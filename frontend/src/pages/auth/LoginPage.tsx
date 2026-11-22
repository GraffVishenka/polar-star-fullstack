/**
 * `/login` — вход сотрудника (AUTH-1).
 *
 * Форма на react-hook-form + zod: клиентская валидация только для UX,
 * решение о допуске принимает сервер (`401 INVALID_CREDENTIALS` без раскрытия
 * существования логина). После входа с `must_change_password` сотрудник
 * попадает на смену пароля (AUTH-4).
 */
import { useState } from 'react';
import { LockOutlined, LoginOutlined, UserOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Flex, Form, Input, Typography } from 'antd';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { z } from 'zod';
import { ApiError, isApiError } from '@/api/errors';
import { APP_DESCRIPTION, APP_NAME } from '@/app/config';
import { useAuth } from '@/auth/AuthContext';
import { toast } from '@/app/toast';

const loginSchema = z.object({
  login: z.string().trim().min(1, 'Введите логин'),
  password: z.string().min(1, 'Введите пароль'),
});

type LoginFormValues = z.infer<typeof loginSchema>;

interface LocationState {
  from?: string;
}

export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { login } = useAuth();
  const [formError, setFormError] = useState<string | null>(null);

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { login: '', password: '' },
  });

  const onSubmit = async (values: LoginFormValues) => {
    setFormError(null);
    try {
      const user = await login(values);
      toast.success(`Добро пожаловать, ${user.full_name}`);
      const state = location.state as LocationState | null;
      if (user.must_change_password) {
        navigate('/change-password', { replace: true });
        return;
      }
      navigate(state?.from && state.from !== '/login' ? state.from : '/tickets', {
        replace: true,
      });
    } catch (error) {
      if (isApiError(error)) {
        // Сервер не раскрывает, существует ли логин (AUTH-1).
        setFormError(error.userMessage);
      } else {
        setFormError('Не удалось войти, попробуйте позже');
      }
      if (error instanceof ApiError && error.status === 429) {
        setFormError('Слишком много попыток входа. Подождите 10 минут.');
      }
    }
  };

  return (
    <Flex
      align="center"
      justify="center"
      className="login-shell"
      data-testid="login-page"
      style={{ minHeight: '100vh', padding: 16 }}
    >
      <Card style={{ width: '100%', maxWidth: 420 }}>
        <Flex vertical gap={4} style={{ marginBottom: 20 }}>
          <Typography.Title level={3} style={{ margin: 0 }}>
            Вход в {APP_NAME}
          </Typography.Title>
          <Typography.Text type="secondary">{APP_DESCRIPTION}</Typography.Text>
        </Flex>

        {formError ? (
          <Alert
            type="error"
            showIcon
            message={formError}
            data-testid="login-error"
            style={{ marginBottom: 16 }}
          />
        ) : null}

        <form onSubmit={handleSubmit(onSubmit)} noValidate>
          <Form.Item
            label="Логин"
            validateStatus={errors.login ? 'error' : undefined}
            help={errors.login?.message}
          >
            <Input
              {...register('login')}
              prefix={<UserOutlined />}
              placeholder="ivanov"
              autoComplete="username"
              size="large"
              aria-label="Логин"
              data-testid="login-input"
            />
          </Form.Item>

          <Form.Item
            label="Пароль"
            validateStatus={errors.password ? 'error' : undefined}
            help={errors.password?.message}
          >
            <Input.Password
              {...register('password')}
              prefix={<LockOutlined />}
              placeholder="••••••••"
              autoComplete="current-password"
              size="large"
              aria-label="Пароль"
              data-testid="password-input"
            />
          </Form.Item>

          <Button
            type="primary"
            htmlType="submit"
            block
            size="large"
            icon={<LoginOutlined />}
            loading={isSubmitting}
            data-testid="login-submit"
          >
            Войти
          </Button>
        </form>

        {import.meta.env.DEV ? (
          <Alert
            style={{ marginTop: 16 }}
            type="info"
            message="Демо-доступ (MSW)"
            description="admin / admin12345 — администратор; user / user12345 — сотрудник здания."
            data-testid="login-demo-hint"
          />
        ) : null}

        <Flex justify="center" style={{ marginTop: 12 }}>
          <Link to="/" style={{ fontSize: 13 }}>
            ← Публичный выбор здания
          </Link>
        </Flex>
      </Card>
    </Flex>
  );
}
