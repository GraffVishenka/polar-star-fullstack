/**
 * `usePublicSettings` (PUB-1, Б7): один запрос на все публичные экраны,
 * безопасный фолбэк на `app/config` при ошибке или неполном ответе.
 */
import { describe, expect, it, vi } from 'vitest';
import { renderHook, screen, waitFor } from '@testing-library/react';
import { HttpResponse, http } from 'msw';
import { APP_NAME } from '@/app/config';
import { server } from '@/mocks/server';
import { createWrapper, renderWithProviders } from '@/test/utils';
import {
  FALLBACK_PUBLIC_SETTINGS,
  mergePublicSettings,
  usePublicSettings,
} from './usePublicSettings';

const CUSTOM_SETTINGS = {
  app_name: 'Сервис XYZ',
  statuses: [
    { code: 'new', label: 'Свежая', color: '#111111' },
    { code: 'in_progress', label: 'В работе', color: '#222222' },
    { code: 'deferred', label: 'Отложена', color: '#333333' },
    { code: 'done', label: 'Завершена', color: '#444444' },
  ],
};

function renderSettingsHook() {
  return renderHook(() => usePublicSettings(), { wrapper: createWrapper() });
}

describe('usePublicSettings', () => {
  it('отдаёт app_name и подписи/цвета статусов из публичного API', async () => {
    server.use(http.get('*/api/public/settings', () => HttpResponse.json(CUSTOM_SETTINGS)));

    const { result } = renderSettingsHook();
    await waitFor(() => expect(result.current.isFallback).toBe(false));

    expect(result.current.appName).toBe('Сервис XYZ');
    expect(result.current.statusMeta.new).toEqual({ label: 'Свежая', color: '#111111' });
    expect(result.current.statusMeta.done.label).toBe('Завершена');
    expect(result.current.settings.statuses).toHaveLength(4);
  });

  it('при ошибке API возвращает значения по умолчанию и не падает', async () => {
    server.use(
      http.get('*/api/public/settings', () =>
        HttpResponse.json({ code: 'INTERNAL_ERROR' }, { status: 500 }),
      ),
    );

    const { result } = renderSettingsHook();
    await waitFor(() => expect(result.current.isFallback).toBe(true));

    expect(result.current.appName).toBe(FALLBACK_PUBLIC_SETTINGS.app_name);
    expect(result.current.appName).toBe(APP_NAME);
    expect(result.current.statusMeta.new.label).toBe('Новая');
    expect(result.current.statusMeta.deferred.color).toBe('#8c8c8c');
  });

  it('дополняет неполный ответ значениями по умолчанию (4 статуса, валидный цвет)', () => {
    const merged = mergePublicSettings({
      app_name: '  Сервис X  ',
      statuses: [
        { code: 'new', label: '', color: 'not-a-hex' },
        { code: 'in_progress', label: 'В работе (своё)', color: '#123456' },
      ],
    });

    expect(merged.app_name).toBe('Сервис X');
    expect(merged.statuses).toHaveLength(4);
    expect(merged.statuses.map((status) => status.code)).toEqual([
      'new',
      'in_progress',
      'deferred',
      'done',
    ]);
    expect(merged.statuses[0]).toEqual({ code: 'new', label: 'Новая', color: '#1677ff' });
    expect(merged.statuses[1]).toEqual({
      code: 'in_progress',
      label: 'В работе (своё)',
      color: '#123456',
    });
  });

  it('не дублирует запрос: два потребителя с одним ключом дают один HTTP-вызов', async () => {
    const requestSpy = vi.fn(() => HttpResponse.json(CUSTOM_SETTINGS));
    server.use(http.get('*/api/public/settings', requestSpy));

    function Consumer({ id }: { id: string }) {
      const { appName } = usePublicSettings();
      return <span data-testid={`app-name-${id}`}>{appName}</span>;
    }

    renderWithProviders(
      <>
        <Consumer id="header" />
        <Consumer id="footer" />
      </>,
    );

    await waitFor(() => {
      expect(screen.getByTestId('app-name-header')).toHaveTextContent('Сервис XYZ');
      expect(screen.getByTestId('app-name-footer')).toHaveTextContent('Сервис XYZ');
    });
    expect(requestSpy).toHaveBeenCalledTimes(1);
  });
});
