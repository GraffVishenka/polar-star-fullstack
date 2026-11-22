/**
 * `/change-password` — обязательная/добровольная смена своего пароля (AUTH-4, AUTH-5).
 *
 * Пока `must_change_password=true`, остальные экраны закрыты guard'ом
 * `RequireAuth`: сервер отвечает 428 PASSWORD_CHANGE_REQUIRED.
 *
 * Форма собрана на antd `Form` с правилами (а не на react-hook-form):
 * `Input.Password` + RHF `register` не передавал значения ни в jsdom, ни в браузере
 * (ref/onChange конфликтовали), из-за чего валидация всегда видела пустые поля.
 */
import { useState } from 'react';
import { LockOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Flex, Form, Input, Typography } from 'antd';
import { useNavigate } from 'react-router-dom';
import { changeOwnPassword } from '@/api/auth';
import { isApiError } from '@/api/errors';
import { toast } from '@/app/toast';
import { useAuth } from '@/auth/AuthContext';

interface PasswordFormValues {
  current_password: string;
  new_password: string;
  confirm_password: string;
}

export function ChangePasswordPage() {
  const navigate = useNavigate();
  const { user, refreshUser } = useAuth();
  const [form] = Form.useForm<PasswordFormValues>();
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const onFinish = async (values: PasswordFormValues) => {
    setFormError(null);
    setSubmitting(true);
    try {
      await changeOwnPassword({
        current_password: values.current_password,
        new_password: values.new_password,
      });
      toast.success('Пароль изменён');
      await refreshUser();
      navigate('/tickets', { replace: true });
    } catch (error) {
      setFormError(isApiError(error) ? error.userMessage : 'Не удалось изменить пароль');
    } finally {
      setSubmitting(false);
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
        <Alert
          type="error"
          showIcon
          message={formError}
          style={{ marginBottom: 16 }}
          data-testid="change-password-error"
        />
      ) : null}

      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        onFinish={onFinish}
        initialValues={{ current_password: '', new_password: '', confirm_password: '' }}
      >
        <Form.Item
          label="Текущий пароль"
          name="current_password"
          rules={[{ required: true, message: 'Введите текущий пароль' }]}
        >
          <Input.Password
            prefix={<LockOutlined />}
            autoComplete="current-password"
            data-testid="change-password-current"
          />
        </Form.Item>

        <Form.Item
          label="Новый пароль"
          name="new_password"
          rules={[
            { required: true, message: 'Введите новый пароль' },
            { min: 8, message: 'Новый пароль — не короче 8 символов' },
            ({ getFieldValue }) => ({
              validator: (_rule, value: string) =>
                value && value === getFieldValue('current_password')
                  ? Promise.reject(new Error('Новый пароль должен отличаться от текущего'))
                  : Promise.resolve(),
            }),
          ]}
        >
          <Input.Password
            prefix={<LockOutlined />}
            autoComplete="new-password"
            data-testid="change-password-new"
          />
        </Form.Item>

        <Form.Item
          label="Повторите новый пароль"
          name="confirm_password"
          dependencies={['new_password']}
          rules={[
            { required: true, message: 'Подтвердите новый пароль' },
            ({ getFieldValue }) => ({
              validator: (_rule, value: string) =>
                !value || value === getFieldValue('new_password')
                  ? Promise.resolve()
                  : Promise.reject(new Error('Пароли не совпадают')),
            }),
          ]}
        >
          <Input.Password
            prefix={<LockOutlined />}
            autoComplete="new-password"
            data-testid="change-password-confirm"
          />
        </Form.Item>

        <Flex gap={8} wrap>
          <Button
            type="primary"
            htmlType="submit"
            loading={submitting}
            data-testid="change-password-submit"
          >
            Сохранить пароль
          </Button>
          {!user?.must_change_password ? (
            <Button onClick={() => navigate(-1)} data-testid="change-password-cancel">
              Отмена
            </Button>
          ) : null}
        </Flex>
      </Form>
    </Card>
  );
}
