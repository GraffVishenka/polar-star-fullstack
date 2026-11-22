/**
 * `/kb` — рабочий раздел базы знаний (Ф3).
 *
 * Двухпанельный вид: слева дерево неограниченной вложенности с поиском,
 * созданием раздела/подраздела и drag-n-drop сортировкой (KB-6), справа —
 * статья (безопасный рендер, KB-4) или TipTap-редактор с вставкой медиа (KB-5).
 *
 * Права (§3.1): чтение/создание/правка/сортировка — любой авторизованный;
 * удаление и тумблер видимости — только ADMIN (KB-3, KB-7).
 * Публичная версия БЗ живёт на `/public/kb` (Ф5) и здесь не дублируется.
 */
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Button, Card, Flex, Input, Space, Spin, Typography } from 'antd';
import { PlusOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import type { KbNode } from '@/api/types';
import { useAuth } from '@/auth/AuthContext';
import { toast } from '@/app/toast';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { EmptyState } from '@/components/EmptyState';
import { PageHeader } from '@/components/PageHeader';
import { KbArticle } from './components/KbArticle';
import { KbEditor, type KbEditorValues } from './components/KbEditor';
import { KbNodeFormModal, type KbNodeFormValues } from './components/KbNodeFormModal';
import { KbTree, type KbMoveRequestTarget } from './components/KbTree';
import {
  kbErrorMessage,
  useKbCreate,
  useKbDelete,
  useKbMove,
  useKbNodeQuery,
  useKbTreeQuery,
  useKbUpdate,
  useKbVisibility,
} from './hooks';
import { countKbNodes, filterKbTree, findKbPath, findKbNode } from './tree';
import { useUnsavedChanges } from './useUnsavedChanges';
import './kb.css';

interface CreateModalState {
  mode: 'root' | 'child';
  parentId: number | null;
}

export function KbPage() {
  const params = useParams<{ nodeId?: string }>();
  const navigate = useNavigate();
  const { isAdmin } = useAuth();

  const rawId = Number(params.nodeId);
  const selectedId = Number.isSafeInteger(rawId) && rawId > 0 ? rawId : null;

  const treeQuery = useKbTreeQuery();
  const tree = useMemo(() => treeQuery.data ?? [], [treeQuery.data]);
  const nodeQuery = useKbNodeQuery(selectedId);
  const node = nodeQuery.data ?? null;

  const [search, setSearch] = useState('');
  const [editing, setEditing] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [createModal, setCreateModal] = useState<CreateModalState | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<KbNode | null>(null);
  const [cancelPrompt, setCancelPrompt] = useState(false);

  const createMutation = useKbCreate();
  const updateMutation = useKbUpdate();
  const deleteMutation = useKbDelete();
  const visibilityMutation = useKbVisibility();
  const move = useKbMove();
  const guard = useUnsavedChanges(dirty);

  const filtered = useMemo(() => filterKbTree(tree, search), [tree, search]);
  const path = useMemo(() => findKbPath(tree, selectedId), [tree, selectedId]);
  const total = useMemo(() => countKbNodes(tree), [tree]);

  // Переход между статьями закрывает режим правки; несохранённое защищено
  // блокировщиком навигации (`useUnsavedChanges`), поэтому здесь сброс безопасен.
  useEffect(() => {
    setEditing(false);
    setDirty(false);
  }, [selectedId]);

  const selectNode = useCallback(
    (id: number) => {
      navigate(`/kb/${id}`);
    },
    [navigate],
  );

  const openCreate = (state: CreateModalState) => {
    setCreateModal(state);
  };

  const submitCreate = async (values: KbNodeFormValues) => {
    try {
      const created = await createMutation.create({
        title: values.title.trim(),
        parent_id: values.parent_id ?? null,
      });
      setCreateModal(null);
      toast.success(`Раздел «${created.title}» создан`);
      selectNode(created.id);
    } catch {
      // Текст ошибки показывается в модалке (`createMutation.error`),
      // введённые значения сохраняются для повторной попытки.
    }
  };

  const saveArticle = async (values: KbEditorValues) => {
    if (!node) return;
    try {
      await updateMutation.update(node.id, {
        title: values.title,
        content_html: values.content_html,
      });
      setEditing(false);
      setDirty(false);
      toast.success('Статья сохранена');
    } catch {
      // Ошибка показывается в редакторе (`updateMutation.error`), правки остаются.
    }
  };

  const confirmDelete = async () => {
    if (!deleteTarget) return;
    const parentId = deleteTarget.parent_id;
    try {
      await deleteMutation.remove(deleteTarget.id);
      setDeleteTarget(null);
      if (parentId !== null) selectNode(parentId);
      else navigate('/kb');
    } catch {
      // Тост с текстом ошибки уже показан хуком.
    }
  };

  const handleMove = (target: KbMoveRequestTarget) => {
    void move.move({
      nodeId: target.nodeId,
      parentId: target.parentId,
      beforeNodeId: target.beforeNodeId,
    });
  };

  const descendantsToDelete = deleteTarget
    ? (() => {
        const found = findKbNode(tree, deleteTarget.id);
        return found ? countKbNodes(found.children) : 0;
      })()
    : 0;

  const treeLoading = treeQuery.isLoading;
  const treeError = treeQuery.error;

  return (
    <Flex vertical gap={16} data-testid="kb-page">
      <PageHeader
        title="База знаний"
        description={
          <>
            {total} разделов · вложенность не ограничена
            {isAdmin ? ' · удаление и видимость доступны администратору' : null}
          </>
        }
        extra={
          <Space size={8} wrap>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => void treeQuery.refetch()}
              aria-label="Обновить дерево разделов"
            >
              Обновить
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => openCreate({ mode: 'root', parentId: null })}
              data-testid="kb-add-root"
            >
              Добавить раздел
            </Button>
          </Space>
        }
      />

      <div className="kb-layout">
        <Card
          size="small"
          className="kb-sidebar"
          title="Разделы"
          data-testid="kb-sidebar"
          extra={
            selectedId !== null ? (
              <Button
                type="link"
                size="small"
                icon={<PlusOutlined />}
                onClick={() => openCreate({ mode: 'child', parentId: selectedId })}
                data-testid="kb-add-subsection"
              >
                Добавить подраздел
              </Button>
            ) : null
          }
        >
          <Flex vertical gap={10}>
            <Input
              allowClear
              prefix={<SearchOutlined />}
              placeholder="Поиск по заголовкам"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              aria-label="Поиск по заголовкам разделов"
              data-testid="kb-search"
            />

            {move.error ? (
              <Alert
                type="error"
                showIcon
                closable
                message={move.error}
                data-testid="kb-move-error"
                onClose={move.clearError}
              />
            ) : null}

            {treeError ? (
              <Alert
                type="error"
                showIcon
                message={kbErrorMessage(treeError, 'Не удалось загрузить дерево разделов')}
                data-testid="kb-tree-error"
              />
            ) : treeLoading ? (
              <Flex justify="center" style={{ padding: 24 }}>
                <Spin data-testid="kb-tree-loading" />
              </Flex>
            ) : (
              <KbTree
                nodes={filtered.tree}
                selectedId={selectedId}
                matchIds={filtered.matchIds}
                canDrag={!search.trim()}
                onSelect={selectNode}
                onMove={handleMove}
                onAddChild={(parentId) => openCreate({ mode: 'child', parentId })}
                emptyText={
                  search.trim() ? 'Ничего не найдено — измените запрос' : 'Разделов пока нет'
                }
              />
            )}

            {search.trim() && !filtered.empty ? (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                Найдено разделов: {filtered.matchIds.size}. Перетаскивание отключено в поиске.
              </Typography.Text>
            ) : null}
          </Flex>
        </Card>

        <Card size="small" data-testid="kb-content">
          {selectedId === null ? (
            <EmptyState
              title="Выберите раздел"
              description="Слева — дерево базы знаний. Раздел можно создать или открыть статью."
              action={{
                label: 'Добавить раздел',
                onClick: () => openCreate({ mode: 'root', parentId: null }),
                icon: <PlusOutlined />,
              }}
            />
          ) : nodeQuery.isLoading ? (
            <Flex justify="center" style={{ padding: 48 }}>
              <Spin data-testid="kb-node-loading" />
            </Flex>
          ) : nodeQuery.error || !node ? (
            <EmptyState
              title="Раздел не найден"
              description={kbErrorMessage(
                nodeQuery.error,
                'Возможно, раздел удалён или перемещён в корзину.',
              )}
              action={{ label: 'К списку разделов', onClick: () => navigate('/kb') }}
            />
          ) : editing ? (
            <KbEditor
              key={node.id}
              initialTitle={node.title}
              initialHtml={node.content_html}
              pending={updateMutation.pending}
              error={updateMutation.error}
              onDirtyChange={setDirty}
              onSave={saveArticle}
              onCancel={() => {
                if (dirty) setCancelPrompt(true);
                else setEditing(false);
              }}
            />
          ) : (
            <KbArticle
              node={node}
              path={path}
              isAdmin={isAdmin}
              onEdit={() => setEditing(true)}
              onDelete={() => setDeleteTarget(node)}
              onVisibilityChange={(hidden) => {
                void visibilityMutation
                  .setVisibility(node.id, hidden)
                  .then(() => {
                    toast.success(hidden ? 'Раздел скрыт от анонимов' : 'Раздел снова публичный');
                  })
                  .catch(() => undefined);
              }}
              visibilityPending={visibilityMutation.pending}
            />
          )}
        </Card>
      </div>

      <KbNodeFormModal
        open={createModal !== null}
        mode={createModal?.mode ?? 'root'}
        tree={tree}
        defaultParentId={createModal?.parentId ?? null}
        pending={createMutation.pending}
        error={createMutation.error}
        onCancel={() => setCreateModal(null)}
        onSubmit={submitCreate}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Удалить раздел?"
        description={
          deleteTarget
            ? `«${deleteTarget.title}» будет перемещён в корзину.${
                descendantsToDelete > 0
                  ? ` Вместе с ним скрыты подразделы: ${descendantsToDelete}.`
                  : ''
              }`
            : undefined
        }
        warning="Восстановление узла не восстанавливает потомков (TRASH-5)."
        okText="Удалить"
        danger
        loading={deleteMutation.pending}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteTarget(null)}
      />

      <ConfirmDialog
        open={guard.confirmOpen}
        title="Есть несохранённые изменения"
        description="Если уйти со страницы, правки статьи будут потеряны."
        okText="Выйти без сохранения"
        cancelText="Остаться"
        danger
        onConfirm={guard.confirmLeave}
        onCancel={guard.stay}
      />

      <ConfirmDialog
        open={cancelPrompt}
        title="Отменить редактирование?"
        description="Несохранённые изменения статьи будут потеряны."
        okText="Отменить изменения"
        cancelText="Продолжить редактирование"
        danger
        onConfirm={() => {
          setCancelPrompt(false);
          setEditing(false);
          setDirty(false);
        }}
        onCancel={() => setCancelPrompt(false)}
      />
    </Flex>
  );
}
