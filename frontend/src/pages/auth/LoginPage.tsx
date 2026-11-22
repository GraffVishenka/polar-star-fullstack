/**
 * `/login` — вход сотрудника (AUTH-1).
 *
 * Форма на antd `Form` + `Form.useForm` + `onFinish` (как `/change-password`
 * и `/profile`): связка RHF `register` с antd `Input` не передавала значения
 * в состояние формы, из-за чего валидация видела пустые поля и submit уходил
 * с пустыми `login/password`. Решение о допуске принимает сервер
 * (`401 INVALID_CREDENTIALS` без раскрытия существования логина).
 * После входа с `must_change_password` сотрудник попадает на смену пароля
 * (AUTH-4), иначе — на `state.from` или `/tickets`.
 */
import { useState } from 'react';
import { LockOutlined, LoginOutlined, UserOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Flex, Form, Input, Typography } from 'antd';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { ApiError, isApiError } from '@/api/errors';
import { APP_DESCRIPTION, APP_NAME } from '@/app/config';
import { useAuth } from '@/auth/AuthContext';
import { toast } from '@/app/toast';

interface LoginFormValues {
  login: string;
  password: string;
}

interface LocationState {
  from?: string;
}

export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const { login } = useAuth();
  const [form] = Form.useForm<LoginFormValues>();
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const onFinish = async (values: LoginFormValues) => {
    setFormError(null);
    setSubmitting(true);
    try {
      // AUTH-1: логин без пробелов по краям, решение о допуске — на сервере.
      const user = await login({ login: values.login.trim(), password: values.password });
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
    } finally {
      setSubmitting(false);
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

        <Form<LoginFormValues>
          form={form}
          layout="vertical"
          requiredMark={false}
          onFinish={onFinish}
          initialValues={{ login: '', password: '' }}
          data-testid="login-form"
        >
          <Form.Item
            label="Логин"
            name="login"
            rules={[
              { required: true, whitespace: true, message: 'Введите логин' },
              { max: 120, message: 'Логин — не более 120 символов' },
            ]}
          >
            <Input
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
            name="password"
            rules={[{ required: true, message: 'Введите пароль' }]}
          >
            <Input.Password
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
            loading={submitting}
            data-testid="login-submit"
          >
            Войти
          </Button>
        </Form>

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
