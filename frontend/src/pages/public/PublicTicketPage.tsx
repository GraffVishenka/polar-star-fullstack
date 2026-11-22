/**
 * `/t/:token` — карточка заявки по бессрочной ссылке (TICKET-13, PUB-6).
 *
 * Ф1: минимальная, но рабочая карточка (поля + дедлайн + комментарии + Word).
 * Расширенный вид (добавление комментария с видео, вложения) — блок Ф5.
 */
import { useQuery } from '@tanstack/react-query';
import { ApiOutlined, CalendarOutlined, UserOutlined } from '@ant-design/icons';
import { Alert, Card, Descriptions, Divider, Flex, Skeleton, Space, Tag, Typography } from 'antd';
import { Link, useParams } from 'react-router-dom';
import { fetchPublicTicket, publicTicketExportPath } from '@/api/public';
import { queryKeys } from '@/app/queryClient';
import { DeadlineBadge, formatDeadline } from '@/components/DeadlineBadge';
import { EmptyState } from '@/components/EmptyState';
import { ExportButton } from '@/components/ExportButton';
import { MediaGallery } from '@/components/MediaGallery';
import { StatusTag } from '@/components/StatusTag';
import dayjs from 'dayjs';

export function PublicTicketPage() {
  const { token } = useParams<{ token: string }>();
  const enabled = Boolean(token);

  const query = useQuery({
    queryKey: queryKeys.publicTicket(token ?? ''),
    queryFn: () => fetchPublicTicket(token!),
    enabled,
    retry: false,
  });

  if (!token) {
    return <EmptyState title="Ссылка не указана" description="Проверьте адрес страницы." />;
  }

  if (query.isLoading) {
    return (
      <Card>
        <Skeleton active paragraph={{ rows: 6 }} />
      </Card>
    );
  }

  if (query.isError) {
    const isNotFound = (query.error as { code?: string } | null)?.code === 'NOT_FOUND';
    return (
      <EmptyState
        title={isNotFound ? 'Заявка не найдена' : 'Не удалось открыть заявку'}
        description={
          isNotFound
            ? 'Ссылка недействительна или заявка удалена. Обратитесь к сотруднику, который её создал.'
            : 'Проверьте интернет-соединение и попробуйте ещё раз.'
        }
        action={{ label: 'К выбору здания', onClick: () => window.location.assign('/?change=1') }}
      />
    );
  }

  const ticket = query.data;
  if (!ticket) return null;

  return (
    <Flex vertical gap={16} data-testid="public-ticket-page">
      <Flex justify="space-between" align="flex-start" gap={12} wrap>
        <Flex vertical gap={6}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            <ApiOutlined /> Общедоступная ссылка
          </Typography.Text>
          <Typography.Title level={2} style={{ margin: 0, fontSize: 'clamp(20px, 5vw, 26px)' }}>
            Заявка №{ticket.id}
          </Typography.Title>
          <Space size={8} wrap>
            <StatusTag
              status={ticket.status}
              label={ticket.status_label}
              color={ticket.status_color}
            />
            <DeadlineBadge deadline={ticket.deadline} isOverdue={ticket.is_overdue} />
          </Space>
        </Flex>
        <ExportButton
          path={publicTicketExportPath(token)}
          skipAuth
          label="Скачать в Word"
          filename={`ticket_${ticket.id}.docx`}
        />
      </Flex>

      <Card>
        <Descriptions column={{ xs: 1, sm: 2 }} size="small" bordered>
          <Descriptions.Item label="Здание">{ticket.building_name}</Descriptions.Item>
          <Descriptions.Item label="Кабинет">{ticket.cabinet}</Descriptions.Item>
          <Descriptions.Item label="Заявитель">{ticket.applicant_name}</Descriptions.Item>
          <Descriptions.Item label="Вид проблемы">{ticket.problem_type_name}</Descriptions.Item>
          <Descriptions.Item label="Исполнитель">{ticket.executor_name ?? '—'}</Descriptions.Item>
          <Descriptions.Item label="Дедлайн">
            {ticket.deadline ? formatDeadline(ticket.deadline) : '—'}
          </Descriptions.Item>
          <Descriptions.Item label="Создана">
            <CalendarOutlined /> {dayjs(ticket.created_at).format('DD.MM.YYYY HH:mm')}
          </Descriptions.Item>
          <Descriptions.Item label="Завершена">
            {ticket.completed_at ? dayjs(ticket.completed_at).format('DD.MM.YYYY HH:mm') : '—'}
          </Descriptions.Item>
        </Descriptions>

        <Divider orientation="left" plain>
          Описание
        </Divider>
        <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
          {ticket.description || '—'}
        </Typography.Paragraph>

        {ticket.files.length ? (
          <>
            <Divider orientation="left" plain>
              Вложения
            </Divider>
            <MediaGallery files={ticket.files} emptyText={null} />
          </>
        ) : null}
      </Card>

      <Card
        title={
          <Space>
            Комментарии
            <Tag>{ticket.comments.length}</Tag>
          </Space>
        }
      >
        {ticket.comments.length === 0 ? (
          <Typography.Text type="secondary">
            Комментариев пока нет. Добавление комментариев появится в блоке Ф5.
          </Typography.Text>
        ) : (
          <Flex vertical gap={12} data-testid="public-comments">
            {ticket.comments.map((comment) => (
              <Card key={comment.id} size="small" type="inner">
                <Flex justify="space-between" gap={8} wrap>
                  <Typography.Text strong>
                    <UserOutlined /> {comment.author_name}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {dayjs(comment.created_at).format('DD.MM.YYYY HH:mm')}
                  </Typography.Text>
                </Flex>
                <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', margin: '8px 0 0' }}>
                  {comment.body}
                </Typography.Paragraph>
                {comment.files.length ? (
                  <Flex style={{ marginTop: 8 }}>
                    <MediaGallery files={comment.files} compact emptyText={null} />
                  </Flex>
                ) : null}
              </Card>
            ))}
          </Flex>
        )}
      </Card>

      <Alert
        type="info"
        showIcon
        message="Ссылка постоянная"
        description="Сохраните адрес страницы: по нему доступна только эта заявка (карточка, комментарии и Word). Пересылайте её только тем, кому доверяете."
      />

      <Typography.Text type="secondary" style={{ fontSize: 12 }}>
        <Link to="/?change=1">Выбрать другое здание</Link>
      </Typography.Text>
    </Flex>
  );
}
