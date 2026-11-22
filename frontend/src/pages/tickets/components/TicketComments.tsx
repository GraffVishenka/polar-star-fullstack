/**
 * Комментарии заявки (TICKET-14, TICKET-15).
 *
 * - плоский список по 50 на страницу (infinite query, «Показать ещё»);
 * - добавление текста и до 5 вложений (FileUploader: фото/видео с прогрессом);
 * - оптимистичная вставка комментария с откатом при 403/422.
 */
import { useState } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Card, Flex, Input, Typography } from 'antd';
import { SendOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { createTicketComment, fetchTicketComments } from '@/api/tickets';
import type { FileMeta, TicketComment } from '@/api/types';
import { isApiError } from '@/api/errors';
import { toast } from '@/app/toast';
import { useAuth } from '@/auth/AuthContext';
import { EmptyState } from '@/components/EmptyState';
import { FileUploader } from '@/components/FileUploader';
import { MediaGallery } from '@/components/MediaGallery';

export const COMMENTS_PAGE_SIZE = 50;

export function ticketCommentsKey(ticketId: number) {
  return ['tickets', 'comments', ticketId] as const;
}

interface CommentsPage {
  items: TicketComment[];
  total: number;
  page: number;
  size: number;
}

export interface TicketCommentsProps {
  ticketId: number;
  canComment: boolean;
}

export function TicketComments({ ticketId, canComment }: TicketCommentsProps) {
  const { user } = useAuth();
  const qc = useQueryClient();
  const commentsKey = ticketCommentsKey(ticketId);

  const [body, setBody] = useState('');
  const [files, setFiles] = useState<FileMeta[]>([]);

  const query = useInfiniteQuery({
    queryKey: commentsKey,
    queryFn: ({ pageParam }) =>
      fetchTicketComments(ticketId, { page: pageParam, size: COMMENTS_PAGE_SIZE }),
    initialPageParam: 1,
    getNextPageParam: (lastPage) =>
      lastPage.page * lastPage.size < lastPage.total ? lastPage.page + 1 : undefined,
  });

  const mutation = useMutation({
    mutationFn: (payload: { submitBody: string; fileIds: number[] }) =>
      createTicketComment(ticketId, { body: payload.submitBody, file_ids: payload.fileIds }),
    onMutate: async (payload) => {
      await qc.cancelQueries({ queryKey: commentsKey });
      const snapshot = qc.getQueryData<{ pages: CommentsPage[]; pageParams: unknown[] }>(
        commentsKey,
      );
      const optimistic: TicketComment = {
        id: -Date.now(),
        ticket_id: ticketId,
        author_name: user?.full_name ?? 'Вы',
        author_type: 'user',
        body: payload.submitBody,
        created_at: new Date().toISOString(),
        files: files.slice(),
      };

      qc.setQueryData<{ pages: CommentsPage[]; pageParams: unknown[] }>(commentsKey, (data) => {
        if (!data) return data;
        return {
          ...data,
          pages: data.pages.map((page, index) =>
            index === data.pages.length - 1
              ? { ...page, items: [...page.items, optimistic], total: page.total + 1 }
              : page,
          ),
        };
      });

      setBody('');
      setFiles([]);
      return { snapshot, submitBody: payload.submitBody, fileIds: payload.fileIds };
    },
    onError: (error, _payload, context) => {
      // Откат: возвращаем кэш и текст, чтобы пользователь не потерял сообщение.
      if (context?.snapshot) qc.setQueryData(commentsKey, context.snapshot);
      setBody(context?.submitBody ?? '');
      toast.error(isApiError(error) ? error.userMessage : 'Не удалось добавить комментарий');
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: commentsKey });
      void qc.invalidateQueries({ queryKey: ['tickets', 'board'] });
      void qc.invalidateQueries({ queryKey: ['tickets', 'item'] });
    },
  });

  const submit = () => {
    const trimmed = body.trim();
    if (!trimmed && files.length === 0) return;
    mutation.mutate({ submitBody: trimmed, fileIds: files.map((file) => file.id) });
  };

  const comments = query.data?.pages.flatMap((page) => page.items) ?? [];
  const total = query.data?.pages[0]?.total ?? 0;

  return (
    <Card
      size="small"
      title={
        <Flex align="center" gap={8}>
          <span>Комментарии</span>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {total}
          </Typography.Text>
        </Flex>
      }
      data-testid="ticket-comments"
    >
      <Flex vertical gap={12}>
        {query.isLoading ? (
          <Typography.Text type="secondary">Загружаем комментарии…</Typography.Text>
        ) : null}

        {!query.isLoading && comments.length === 0 ? (
          <EmptyState compact title="Комментариев пока нет" description="Напишите первый." />
        ) : (
          <Flex vertical gap={8} data-testid="comments-list">
            {comments.map((comment) => (
              <Card
                key={comment.id}
                size="small"
                type="inner"
                data-testid={`comment-item-${comment.id}`}
              >
                <Flex justify="space-between" gap={8} wrap>
                  <Typography.Text strong>{comment.author_name}</Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {dayjs(comment.created_at).format('DD.MM.YYYY HH:mm')}
                  </Typography.Text>
                </Flex>
                {comment.body ? (
                  <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', margin: '6px 0 0' }}>
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

        {query.hasNextPage ? (
          <Flex justify="center">
            <Button
              size="small"
              loading={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
              data-testid="comments-more"
            >
              Показать ещё ({comments.length} из {total})
            </Button>
          </Flex>
        ) : null}

        {canComment ? (
          <Flex vertical gap={8} data-testid="comment-form">
            <Input.TextArea
              value={body}
              onChange={(event) => setBody(event.target.value)}
              placeholder="Текст комментария (до 3000 символов). Можно приложить фото или видео"
              autoSize={{ minRows: 2, maxRows: 6 }}
              maxLength={3000}
              showCount
              data-testid="comment-input"
              aria-label="Текст комментария"
            />
            <FileUploader
              scope="ticket"
              value={files}
              onChange={setFiles}
              maxCount={5}
              hint="До 5 вложений: фото до 10 МБ, видео до 200 МБ"
            />
            <Flex justify="flex-end" gap={8}>
              <Button
                type="primary"
                icon={<SendOutlined />}
                loading={mutation.isPending}
                disabled={!body.trim() && files.length === 0}
                onClick={submit}
                data-testid="comment-submit"
              >
                Добавить комментарий
              </Button>
            </Flex>
          </Flex>
        ) : (
          <Alert
            type="info"
            showIcon
            message="Комментарии доступны сотрудникам здания заявки и администратору"
            data-testid="comment-forbidden"
          />
        )}
      </Flex>
    </Card>
  );
}
