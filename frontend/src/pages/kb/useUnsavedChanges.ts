/**
 * Защита от потери несохранённых изменений статьи (Ф3).
 *
 * Два рубежа:
 *  1. `beforeunload` — закрытие вкладки/перезагрузка (сработает только когда
 *     есть несохранённые правки; браузер покажет свой диалог);
 *  2. `useBlocker` — переход внутри SPA (дерево, меню, хлебные крошки): пока
 *     правки не сохранены, показываем подтверждение и только после согласия
 *     отпускаем навигацию.
 */
import { useCallback, useEffect, useState } from 'react';
import { useBlocker } from 'react-router-dom';

export interface UnsavedChangesGuard {
  /** Нужно показать диалог подтверждения перехода. */
  confirmOpen: boolean;
  /** Перейти без сохранения. */
  confirmLeave: () => void;
  /** Остаться в редакторе. */
  stay: () => void;
}

export function useUnsavedChanges(dirty: boolean): UnsavedChangesGuard {
  const blocker = useBlocker(dirty);
  const [confirmOpen, setConfirmOpen] = useState(false);

  useEffect(() => {
    if (blocker.state === 'blocked') setConfirmOpen(true);
  }, [blocker.state]);

  useEffect(() => {
    if (!dirty) return;
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      // Chrome требует вернуть значение; строка не показывается пользователю.
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [dirty]);

  const confirmLeave = useCallback(() => {
    setConfirmOpen(false);
    blocker.proceed?.();
  }, [blocker]);

  const stay = useCallback(() => {
    setConfirmOpen(false);
    blocker.reset?.();
  }, [blocker]);

  return { confirmOpen, confirmLeave, stay };
}
