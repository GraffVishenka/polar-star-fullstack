/**
 * Мутации карточки заявки: PATCH с оптимистичным обновлением и откатом,
 * удаление в корзину (TICKET-9, TICKET-10, TICKET-12).
 */
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { deleteTicket, updateTicket } from '@/api/tickets';
import type { TicketUpdateRequest, TicketUpdateResponse } from '@/api/types';
import { isApiError } from '@/api/errors';
import { queryKeys } from '@/app/queryClient';
import { toast } from '@/app/toast';
import {
  invalidateTicketQueries,
  optimisticTicketPatch,
  restoreTicketQueries,
  snapshotTicketQueries,
} from './optimistic';

export interface UpdateTicketVariables {
  id: number;
  patch: TicketUpdateRequest;
}

export function useUpdateTicket() {
  const qc = useQueryClient();

  return useMutation<
    TicketUpdateResponse,
    unknown,
    UpdateTicketVariables,
    {
      snapshot: ReturnType<typeof snapshotTicketQueries>;
    }
  >({
    mutationFn: ({ id, patch }) => updateTicket(id, patch),
    onMutate: async ({ id, patch }) => {
      await qc.cancelQueries({ queryKey: ['tickets'] });
      const snapshot = snapshotTicketQueries(qc);
      optimisticTicketPatch(qc, id, patch);
      return { snapshot };
    },
    onError: (error, _variables, context) => {
      // Ф2: откат оптимистичного обновления при 403/404/422.
      if (context?.snapshot) restoreTicketQueries(qc, context.snapshot);
      toast.error(isApiError(error) ? error.userMessage : 'Не удалось сохранить изменения');
    },
    onSuccess: (data) => {
      qc.setQueryData(queryKeys.ticket(data.id), data);
      if (data.executor_reset) {
        toast.warning(
          'Исполнитель снят: он относится к другому зданию и не может вести эту заявку',
        );
      }
    },
    onSettled: () => {
      void invalidateTicketQueries(qc);
    },
  });
}

export function useDeleteTicket() {
  const qc = useQueryClient();

  return useMutation<void, unknown, number>({
    mutationFn: (id) => deleteTicket(id),
    onSuccess: (_data, id) => {
      toast.success(`Заявка №${id} перемещена в корзину`);
      qc.removeQueries({ queryKey: queryKeys.ticket(id) });
      void invalidateTicketQueries(qc);
    },
    onError: (error) => {
      toast.error(isApiError(error) ? error.userMessage : 'Не удалось удалить заявку');
    },
  });
}
