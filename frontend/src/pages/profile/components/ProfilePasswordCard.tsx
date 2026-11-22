/**
 * Смена своего пароля в `/profile` (AUTH-5).
 *
 * Правила: текущий пароль обязателен, новый — не короче 8 символов и отличается
 * от текущего; подтверждение должно совпадать. Ошибки валидации показываются
 * у полей, серверные (400/422) — отдельным алертом.
 */
import { useState } from 'react';
import { LockOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Flex, Form, Input } from 'antd';
import { changeOwnPassword } from '@/api/auth';
import { isApiError } from '@/api/errors';

interface PasswordFormValues {
  current_password: string;
  new_password: string;
  confirm_password: string;
}

export function ProfilePasswordCard() {
  const [form] = Form.useForm<PasswordFormValues>();
  const [formError, setFormError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  const onFinish = async (values: PasswordFormValues) => {
    setFormError(null);
    setDone(false);
    setSubmitting(true);
    try {
      await changeOwnPassword({
        current_password: values.current_password,
        new_password: values.new_password,
      });
      form.resetFields();
      setDone(true);
    } catch (error) {
      setFormError(isApiError(error) ? error.userMessage : 'Не удалось изменить пароль');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Card
      size="small"
      title={
        <span>
          <LockOutlined /> Смена пароля
        </span>
      }
      data-testid="profile-password"
    >
      {formError ? (
        <Alert
          type="error"
          showIcon
          message={formError}
          style={{ marginBottom: 12 }}
          data-testid="profile-password-error"
        />
      ) : null}
      {done ? (
        <Alert
          type="success"
          showIcon
          message="Пароль обновлён"
          style={{ marginBottom: 12 }}
          data-testid="profile-password-success"
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
            data-testid="profile-current-password"
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
            data-testid="profile-new-password"
          />
        </Form.Item>

        <Form.Item
          label="Повторите новый пароль"
          name="confirm_password"
          dependencies={['new_password']}
          rules={[
            { required: true, message: 'Повторите новый пароль' },
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
            data-testid="profile-confirm-password"
          />
        </Form.Item>

        <Flex gap={8}>
          <Button
            type="primary"
            htmlType="submit"
            loading={submitting}
            data-testid="profile-password-submit"
          >
            Сохранить пароль
          </Button>
        </Flex>
      </Form>
    </Card>
  );
}
