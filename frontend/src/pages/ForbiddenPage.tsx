/** 403 — доступ запрещён (например, попытка открыть чужой закрытый раздел). */
import { Button, Result } from 'antd';
import { useNavigate } from 'react-router-dom';

export function ForbiddenPage() {
  const navigate = useNavigate();
  return (
    <div data-testid="forbidden-page">
      <Result
        status="403"
        title="Доступ запрещён"
        subTitle="У вас нет прав на этот раздел. Если это ошибка — обратитесь к администратору."
        extra={
          <Button type="primary" onClick={() => navigate(-1)}>
            Назад
          </Button>
        }
      />
    </div>
  );
}
