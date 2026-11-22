/** 404 — неизвестный маршрут (в т.ч. неактивное здание и чужой токен, PUB-2). */
import { Button, Result } from 'antd';
import { useNavigate } from 'react-router-dom';

export function NotFoundPage() {
  const navigate = useNavigate();
  return (
    <Result
      status="404"
      title="Страница не найдена"
      subTitle="Проверьте адрес или вернитесь в начало: возможно, здание деактивировано, а ссылка устарела."
      extra={
        <Button type="primary" onClick={() => navigate('/', { replace: true })}>
          На главную
        </Button>
      }
      data-testid="not-found-page"
    />
  );
}
