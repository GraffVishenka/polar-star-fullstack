/**
 * Тесты оптимистичного переноса узлов БЗ (KB-6).
 *
 * Проверяем оба исхода: откат при `409 CYCLE_DETECTED` (дерево возвращается
 * к прежнему виду, пользователь видит причину) и сохранение нового порядка,
 * который после «перезагрузки» (новый QueryClient) приходит с сервера.
 */
import { beforeEach, describe, expect, it } from 'vitest';
import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { HttpResponse, delay, http } from 'msw';
import { server } from '@/mocks/server';
import { authenticateAs, renderWithProviders } from '@/test/utils';
import { useKbMove, useKbTreeQuery } from './hooks';
import { flattenKbTree } from './tree';

beforeEach(() => {
  // Авторизованная сессия ADMIN (id 1): дерево отдаётся только USER/ADMIN.
  authenticateAs(1);
});

function TreeHarness() {
  const treeQuery = useKbTreeQuery();
  const { move, error } = useKbMove();
  const flat = treeQuery.data ? flattenKbTree(treeQuery.data) : [];
  const text = flat.map((entry) => `${entry.node.id}@${entry.parentId ?? 'root'}`).join(' ');

  return (
    <div>
      <button
        type="button"
        data-testid="move-order"
        onClick={() => void move({ nodeId: 12, parentId: 10, beforeNodeId: 11 })}
      >
        поставить перед соседом
      </button>
      <span data-testid="move-error">{error ?? ''}</span>
      <output data-testid="tree">{text}</output>
    </div>
  );
}

describe('useKbMove (drag-n-drop)', () => {
  it('откатывает оптимистичное перемещение при 409 CYCLE_DETECTED', async () => {
    const user = userEvent.setup();
    renderWithProviders(<TreeHarness />);

    await waitFor(() => expect(screen.getByTestId('tree')).toHaveTextContent('11@10 12@10'));

    // Сервер отвечает «медленно» и с 409: оптимистичное состояние должно
    // успеть показаться, а затем откатиться.
    server.use(
      http.post('*/api/kb/nodes/:id/move', async () => {
        await delay(150);
        return HttpResponse.json(
          {
            code: 'CYCLE_DETECTED',
            message: 'Нельзя переместить раздел внутрь себя',
            details: null,
            request_id: 'mock-cycle',
          },
          { status: 409 },
        );
      }),
    );

    await user.click(screen.getByTestId('move-order'));

    // Оптимистично: раздел 12 уже стоит перед 11.
    await waitFor(() => {
      const text = screen.getByTestId('tree').textContent ?? '';
      expect(text.indexOf('12@10')).toBeLessThan(text.indexOf('11@10'));
    });

    // Ошибка 409 → откат к порядку сервера и понятный текст.
    await waitFor(() =>
      expect(screen.getByTestId('move-error')).toHaveTextContent(
        'Нельзя вложить раздел в самого себя',
      ),
    );
    const rolledBack = screen.getByTestId('tree').textContent ?? '';
    expect(rolledBack.indexOf('11@10')).toBeLessThan(rolledBack.indexOf('12@10'));
  });

  it('сохраняет новый порядок после перезагрузки (сортировка на сервере)', async () => {
    const user = userEvent.setup();
    const first = renderWithProviders(<TreeHarness />);

    await waitFor(() => expect(screen.getByTestId('tree')).toHaveTextContent('11@10'));

    await user.click(screen.getByTestId('move-order'));

    await waitFor(() => {
      const text = screen.getByTestId('tree').textContent ?? '';
      expect(text.indexOf('12@10')).toBeGreaterThanOrEqual(0);
      expect(text.indexOf('12@10')).toBeLessThan(text.indexOf('11@10'));
    });

    first.unmount();

    // «Перезагрузка»: новый QueryClient и новый запрос GET /kb/tree.
    renderWithProviders(<TreeHarness />);
    await waitFor(() => {
      const text = screen.getByTestId('tree').textContent ?? '';
      expect(text.indexOf('12@10')).toBeGreaterThanOrEqual(0);
      expect(text.indexOf('12@10')).toBeLessThan(text.indexOf('11@10'));
    });
    expect(screen.getByTestId('move-error')).toHaveTextContent('');
  });
});
