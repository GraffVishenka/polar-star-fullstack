/** Публичная 404-страница: неизвестное здание, чужой токен, скрытый раздел БЗ (PUB-2, KB-7). */
import { Button, Result } from 'antd';
import { Link, useNavigate } from 'react-router-dom';

export interface PublicNotFoundProps {
  title?: string;
  description?: string;
}

export function PublicNotFound({
  title = 'Страница не найдена',
  description = 'Возможно, ссылка устарела. Выберите здание из списка — там всегда свежие заявки.',
}: PublicNotFoundProps) {
  const navigate = useNavigate();

  return (
    <div data-testid="public-not-found">
      <Result
        status="404"
        title={title}
        subTitle={description}
        extra={[
          <Button key="home" type="primary" onClick={() => navigate('/', { replace: true })}>
            К выбору здания
          </Button>,
          <Link key="kb" to="/public/kb">
            <Button>База знаний</Button>
          </Link>,
        ]}
      />
    </div>
  );
}
