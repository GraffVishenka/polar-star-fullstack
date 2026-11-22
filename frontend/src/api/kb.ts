/** База знаний (§3.5). Дерево приходит одним запросом (KB-8). */
import { apiRequest } from './client';
import type {
  KbDeleteResponse,
  KbMoveRequest,
  KbNode,
  KbNodeCreateRequest,
  KbNodeUpdateRequest,
  KbTreeNode,
} from './types';

export function fetchKbTree(): Promise<KbTreeNode[]> {
  return apiRequest<KbTreeNode[]>('/kb/tree');
}

export function fetchKbNode(id: number): Promise<KbNode> {
  return apiRequest<KbNode>(`/kb/nodes/${id}`);
}

export function createKbNode(body: KbNodeCreateRequest): Promise<KbNode> {
  return apiRequest<KbNode>('/kb/nodes', { method: 'POST', body });
}

export function updateKbNode(id: number, body: KbNodeUpdateRequest): Promise<KbNode> {
  return apiRequest<KbNode>(`/kb/nodes/${id}`, { method: 'PATCH', body });
}

/** KB-6: ручная сортировка / перенос узла; цикл → 409 CYCLE_DETECTED. */
export function moveKbNode(id: number, body: KbMoveRequest): Promise<KbNode> {
  return apiRequest<KbNode>(`/kb/nodes/${id}/move`, { method: 'POST', body });
}

/** KB-3: удаление в корзину каскадно, только ADMIN; ответ — число потомков. */
export function deleteKbNode(id: number): Promise<KbDeleteResponse> {
  return apiRequest<KbDeleteResponse>(`/kb/nodes/${id}`, { method: 'DELETE' });
}

/** KB-7: тумблер видимости скрытого корневого раздела — только ADMIN. */
export function setKbNodeVisibility(id: number, isHidden: boolean): Promise<KbNode> {
  return apiRequest<KbNode>(`/kb/nodes/${id}/visibility`, {
    method: 'PATCH',
    body: { is_hidden_for_anon: isHidden },
  });
}
