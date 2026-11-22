/**
 * Данные и мутации БЗ (KB-1…KB-8).
 *
 * Все запросы идут через TanStack Query; ключи — общие `queryKeys.kbTree` и
 * `queryKeys.kbNode(id)` (Ф1), поэтому дерево и статья обновляются согласованно.
 * Мутации показывают понятный русский текст ошибки (`ApiError.userMessage`),
 * а перенос узла — ещё и оптимистичное обновление с откатом (KB-6).
 */
import { useCallback, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { queryKeys } from '@/app/queryClient';
import { toast } from '@/app/toast';
import { ApiError } from '@/api/errors';
import {
  createKbNode,
  deleteKbNode,
  fetchKbNode,
  fetchKbTree,
  moveKbNode,
  setKbNodeVisibility,
  updateKbNode,
} from '@/api/kb';
import type {
  KbMoveRequest,
  KbNode,
  KbNodeCreateRequest,
  KbNodeUpdateRequest,
  KbTreeNode,
} from '@/api/types';
import { moveKbTreeNode } from './tree';

/** Текст ошибки для баннера/тоста: доменный код важнее формулировки сервера. */
export function kbErrorMessage(error: unknown, fallback = 'Не удалось выполнить действие'): string {
  if (error instanceof ApiError) return error.userMessage;
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

export function useKbTreeQuery() {
  return useQuery({ queryKey: queryKeys.kbTree, queryFn: fetchKbTree });
}

export function useKbNodeQuery(id: number | null) {
  return useQuery({
    queryKey: queryKeys.kbNode(id ?? 0),
    queryFn: () => fetchKbNode(id as number),
    enabled: id !== null && Number.isSafeInteger(id) && id > 0,
  });
}

export interface KbMoveVariables {
  nodeId: number;
  parentId: number | null;
  beforeNodeId: number | null;
}

/**
 * Перенос узла (KB-6): оптимистично перестраиваем дерево, при ошибке
 * (409 `CYCLE_DETECTED`, 400 «родитель не найден») возвращаем снимок —
 * порядок и вложенность восстанавливаются, пользователь видит причину.
 */
export function useKbMove() {
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: ({ nodeId, parentId, beforeNodeId }: KbMoveVariables) => {
      const body: KbMoveRequest = { parent_id: parentId, before_node_id: beforeNodeId };
      return moveKbNode(nodeId, body);
    },
    onMutate: async (variables) => {
      setError(null);
      await queryClient.cancelQueries({ queryKey: queryKeys.kbTree });
      const previous = queryClient.getQueryData<KbTreeNode[]>(queryKeys.kbTree);
      if (previous) {
        queryClient.setQueryData<KbTreeNode[]>(
          queryKeys.kbTree,
          moveKbTreeNode(previous, variables.nodeId, variables.parentId, variables.beforeNodeId),
        );
      }
      return { previous };
    },
    onError: (cause, _variables, context) => {
      // Откат: сервер — источник истины, цикл/невалидный родитель отклонены.
      if (context?.previous) queryClient.setQueryData(queryKeys.kbTree, context.previous);
      const message = kbErrorMessage(cause, 'Не удалось переместить раздел');
      setError(message);
      toast.error(message);
    },
    onSuccess: (node) => {
      queryClient.setQueryData(queryKeys.kbNode(node.id), node);
      setError(null);
    },
    onSettled: () => {
      // Перечитываем дерево: сервер переиндексировал соседей шагом 10.
      void queryClient.invalidateQueries({ queryKey: queryKeys.kbTree });
    },
  });

  const move = useCallback(
    (variables: KbMoveVariables) => mutation.mutateAsync(variables).catch(() => undefined),
    [mutation],
  );

  return useMemo(
    () => ({ move, pending: mutation.isPending, error, clearError: () => setError(null) }),
    [move, mutation.isPending, error],
  );
}

/** Создание узла или подраздела (KB-1, KB-2). */
export function useKbCreate() {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: (body: KbNodeCreateRequest) => createKbNode(body),
    onSuccess: (node) => {
      queryClient.setQueryData(queryKeys.kbNode(node.id), node);
      void queryClient.invalidateQueries({ queryKey: queryKeys.kbTree });
    },
  });

  const create = useCallback(
    async (body: KbNodeCreateRequest): Promise<KbNode> => mutation.mutateAsync(body),
    [mutation],
  );

  return useMemo(
    () => ({ create, pending: mutation.isPending, error: mutation.error }),
    [create, mutation.isPending, mutation.error],
  );
}

/** Сохранение заголовка и/или HTML статьи (KB-1, KB-4). */
export function useKbUpdate() {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: ({ id, body }: { id: number; body: KbNodeUpdateRequest }) => updateKbNode(id, body),
    onSuccess: (node) => {
      queryClient.setQueryData(queryKeys.kbNode(node.id), node);
      void queryClient.invalidateQueries({ queryKey: queryKeys.kbTree });
    },
  });

  const update = useCallback(
    (id: number, body: KbNodeUpdateRequest) => mutation.mutateAsync({ id, body }),
    [mutation],
  );

  return useMemo(
    () => ({ update, pending: mutation.isPending, error: mutation.error }),
    [update, mutation.isPending, mutation.error],
  );
}

/** Удаление в корзину — только ADMIN; в ответе число потомков (KB-3, TRASH-5). */
export function useKbDelete() {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: (id: number) => deleteKbNode(id),
    onSuccess: (result) => {
      queryClient.removeQueries({ queryKey: queryKeys.kbNode(result.deleted_id) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.kbTree });
      toast.success(
        result.descendants_deleted > 0
          ? `Раздел удалён вместе с подразделами: ${result.descendants_deleted}`
          : 'Раздел перемещён в корзину',
      );
    },
  });

  const remove = useCallback((id: number) => mutation.mutateAsync(id), [mutation]);

  return useMemo(
    () => ({ remove, pending: mutation.isPending, error: mutation.error }),
    [remove, mutation.isPending, mutation.error],
  );
}

/** Тумблер видимости корневого раздела — только ADMIN (KB-7). */
export function useKbVisibility() {
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: ({ id, hidden }: { id: number; hidden: boolean }) =>
      setKbNodeVisibility(id, hidden),
    onSuccess: (node) => {
      queryClient.setQueryData(queryKeys.kbNode(node.id), node);
      void queryClient.invalidateQueries({ queryKey: queryKeys.kbTree });
    },
  });

  const setVisibility = useCallback(
    (id: number, hidden: boolean) => mutation.mutateAsync({ id, hidden }),
    [mutation],
  );

  return useMemo(
    () => ({ setVisibility, pending: mutation.isPending, error: mutation.error }),
    [setVisibility, mutation.isPending, mutation.error],
  );
}
