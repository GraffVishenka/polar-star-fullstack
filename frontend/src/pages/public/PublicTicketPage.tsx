/**
 * `/t/:token` — карточка заявки по бессрочной ссылке (TICKET-13, PUB-6).
 *
 * Доступ только по токену: поля, здание, дедлайн (`is_overdue` с сервера),
 * статус, вложения, комментарии с серверной пагинацией и добавлением файлов
 * (до 5), скачивание Word. Чужой/битый токен → 404.
 */
import { useMemo, useState } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiOutlined, CalendarOutlined, SendOutlined, UserOutlined } from '@ant-design/icons';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Divider,
  Flex,
  Input,
  Skeleton,
  Space,
  Tag,
  Typography,
} from 'antd';
import dayjs from 'dayjs';
import { Link, useParams } from 'react-router-dom';
import type { FileMeta, PublicCommentCreateRequest } from '@/api/types';
import {
  createPublicComment,
  fetchPublicComments,
  fetchPublicTicket,
  publicTicketExportPath,
  uploadPublicCommentFile,
} from '@/api/public';
import { isApiError } from '@/api/errors';
import { queryKeys } from '@/app/queryClient';
import { toast } from '@/app/toast';
import { DeadlineBadge, formatDeadline } from '@/components/DeadlineBadge';
import { EmptyState } from '@/components/EmptyState';
import { ExportButton } from '@/components/ExportButton';
import { FileUploader } from '@/components/FileUploader';
import { MediaGallery } from '@/components/MediaGallery';
import { StatusTag } from '@/components/StatusTag';
import { PublicNotFound } from './components/PublicNotFound';

const COMMENTS_PAGE_SIZE = 50;

export function PublicTicketPage() {
  const { token } = useParams<{ token: string }>();
  const qc = useQueryClient();
  const hasToken = Boolean(token);

  const [authorName, setAuthorName] = useState('');
  const [commentBody, setCommentBody] = useState('');
  const [commentFiles, setCommentFiles] = useState<FileMeta[]>([]);

  const ticketQuery = useQuery({
    queryKey: queryKeys.publicTicket(token ?? ''),
    queryFn: () => fetchPublicTicket(token!),
    enabled: hasToken,
    retry: false,
  });

  const commentsQuery = useInfiniteQuery({
    queryKey: ['public', 'ticket-comments', token ?? ''] as const,
    queryFn: ({ pageParam }) =>
      fetchPublicComments(token!, { page: pageParam, size: COMMENTS_PAGE_SIZE }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.size < lastPage.total ? lastPage.page + 1 : undefined,
    enabled: hasToken,
    retry: false,
  });

  const addComment = useMutation({
    mutationFn: (payload: PublicCommentCreateRequest) => createPublicComment(token!, payload),
    onSuccess: () => {
      setCommentBody('');
      setCommentFiles([]);
      toast.success('Комментарий добавлен');
      void qc.invalidateQueries({ queryKey: ['public', 'ticket-comments', token ?? ''] });
      void qc.invalidateQueries({ queryKey: queryKeys.publicTicket(token ?? '') });
    },
    onError: (error) => {
      toast.error(isApiError(error) ? error.userMessage : 'Не удалось добавить комментарий');
    },
  });

  const comments = useMemo(
    () => commentsQuery.data?.pages.flatMap((page) => page.items) ?? [],
    [commentsQuery.data],
  );
  const commentsTotal = commentsQuery.data?.pages[0]?.total ?? 0;

  if (!token) {
    return <PublicNotFound title="Ссылка не указана" description="Проверьте адрес страницы." />;
  }

  if (ticketQuery.isLoading) {
    return (
      <Card>
        <Skeleton active paragraph={{ rows: 6 }} />
      </Card>
    );
  }

  if (ticketQuery.isError) {
    if (isApiError(ticketQuery.error) && ticketQuery.error.status === 404) {
      return (
        <PublicNotFound
          title="Заявка не найдена"
          description="Ссылка недействительна или заявка удалена. Обратитесь к сотруднику, который её создал."
        />
      );
    }
    return (
      <Alert
        type="error"
        showIcon
        message="Не удалось открыть заявку"
        description="Проверьте интернет-соединение и попробуйте ещё раз."
        action={
          <Button size="small" onClick={() => void ticketQuery.refetch()}>
            Повторить
          </Button>
        }
        data-testid="public-ticket-error"
      />
    );
  }

  const ticket = ticketQuery.data;
  if (!ticket) return null;

  const canSubmit = Boolean(commentBody.trim()) || commentFiles.length > 0;

  return (
    <Flex vertical gap={16} data-testid="public-ticket-page">
      <Flex justify="space-between" align="flex-start" gap={12} wrap>
        <Flex vertical gap={6}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            <ApiOutlined /> Общедоступная ссылка
          </Typography.Text>
          <Typography.Title level={1} style={{ margin: 0, fontSize: 'clamp(20px, 5vw, 26px)' }}>
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
            <Tag data-testid="public-comments-total">{commentsTotal}</Tag>
          </Space>
        }
        data-testid="public-comments-card"
      >
        {commentsQuery.isLoading ? (
          <Typography.Text type="secondary">Загружаем комментарии…</Typography.Text>
        ) : comments.length === 0 ? (
          <EmptyState compact title="Комментариев пока нет" description="Напишите первый." />
        ) : (
          <Flex vertical gap={12} data-testid="public-comments">
            {comments.map((comment) => (
              <Card
                key={comment.id}
                size="small"
                type="inner"
                data-testid={`public-comment-${comment.id}`}
              >
                <Flex justify="space-between" gap={8} wrap>
                  <Typography.Text strong>
                    <UserOutlined /> {comment.author_name}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {dayjs(comment.created_at).format('DD.MM.YYYY HH:mm')}
                  </Typography.Text>
                </Flex>
                {comment.body ? (
                  <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', margin: '8px 0 0' }}>
                    {comment.body}
                  </Typography.Paragraph>
                ) : null}
                {comment.files.length ? (
                  <Flex style={{ marginTop: 8 }}>
                    <MediaGallery files={comment.files} compact emptyText={null} />
                  </Flex>
                ) : null}
              </Card>
            ))}
          </Flex>
        )}

        {commentsQuery.hasNextPage ? (
          <Flex justify="center" style={{ marginTop: 12 }}>
            <Button
              size="small"
              loading={commentsQuery.isFetchingNextPage}
              onClick={() => void commentsQuery.fetchNextPage()}
              data-testid="public-comments-more"
            >
              Показать ещё ({comments.length} из {commentsTotal})
            </Button>
          </Flex>
        ) : null}

        <Divider orientation="left" plain>
          Добавить комментарий
        </Divider>
        <Flex vertical gap={8}>
          <Input
            value={authorName}
            onChange={(event) => setAuthorName(event.target.value)}
            maxLength={120}
            placeholder="Ваше имя (необязательно)"
            aria-label="Ваше имя"
            data-testid="public-comment-author"
          />
          <Input.TextArea
            value={commentBody}
            onChange={(event) => setCommentBody(event.target.value)}
            autoSize={{ minRows: 2, maxRows: 6 }}
            maxLength={3000}
            showCount
            placeholder="Текст комментария (до 3000 символов)"
            aria-label="Текст комментария"
            data-testid="public-comment-input"
          />
          <FileUploader
            scope="ticket"
            value={commentFiles}
            onChange={setCommentFiles}
            maxCount={5}
            upload={(file, options) =>
              uploadPublicCommentFile(token, file, {
                onProgress: options.onProgress,
                signal: options.signal,
              })
            }
            hint="До 5 вложений: фото до 10 МБ, видео до 200 МБ"
          />
          <Flex justify="flex-end">
            <Button
              type="primary"
              icon={<SendOutlined />}
              loading={addComment.isPending}
              disabled={!canSubmit}
              onClick={() =>
                addComment.mutate({
                  author_name: authorName.trim() || undefined,
                  body: commentBody.trim(),
                  file_ids: commentFiles.map((file) => file.id),
                })
              }
              data-testid="public-comment-submit"
            >
              Отправить комментарий
            </Button>
          </Flex>
        </Flex>
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
