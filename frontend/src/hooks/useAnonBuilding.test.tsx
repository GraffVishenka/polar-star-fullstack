/** Тесты хука выбора здания анонимом: cookie 30 дней, редирект, неактивное здание. */
import { QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it } from 'vitest';
import {
  ANON_BUILDING_COOKIE,
  ANON_BUILDING_COOKIE_MAX_AGE,
  clearAnonBuildingCookie,
  readAnonBuildingCookie,
  useAnonBuilding,
  writeAnonBuildingCookie,
} from './useAnonBuilding';
import { createTestQueryClient } from '@/test/utils';

function setup() {
  const client = createTestQueryClient();
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useAnonBuilding(), { wrapper });
}

function setCookie(value: string) {
  document.cookie = `${ANON_BUILDING_COOKIE}=${value}; Path=/`;
}

describe('cookie анонимного здания (PUB-3)', () => {
  it('пишется на 30 дней с SameSite=Lax и path=/', () => {
    writeAnonBuildingCookie(2);
    // jsdom не отдаёт атрибуты cookie через document.cookie, поэтому проверяем
    // значение и отдельно — что срок жизни соответствует 30 дням.
    expect(document.cookie).toContain(`${ANON_BUILDING_COOKIE}=2`);
    expect(ANON_BUILDING_COOKIE_MAX_AGE).toBe(60 * 60 * 24 * 30);
    expect(readAnonBuildingCookie()).toBe(2);
  });

  it('очищается', () => {
    writeAnonBuildingCookie(3);
    clearAnonBuildingCookie();
    expect(readAnonBuildingCookie()).toBeNull();
  });

  it('нечисловое значение считается отсутствующим', () => {
    expect(readAnonBuildingCookie(`${ANON_BUILDING_COOKIE}=abc`)).toBeNull();
    expect(readAnonBuildingCookie(`${ANON_BUILDING_COOKIE}=-5`)).toBeNull();
    expect(readAnonBuildingCookie('other=1')).toBeNull();
  });
});

describe('useAnonBuilding', () => {
  it('без cookie не редиректит и отдаёт активные здания', async () => {
    const { result } = setup();

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.buildings).toHaveLength(3);
    expect(result.current.buildings.some((building) => building.id === 4)).toBe(false);
    expect(result.current.redirectPath).toBeNull();
    expect(result.current.cookieBuildingId).toBeNull();
  });

  it('с валидной cookie предлагает редирект на /b/{id}', async () => {
    setCookie('2');
    const { result } = setup();

    await waitFor(() => expect(result.current.redirectPath).toBe('/b/2'));
    expect(result.current.selectedBuilding?.name).toBe('Производственный корпус');
  });

  it('неактивное (отсутствующее в публичном списке) здание сбрасывает cookie', async () => {
    setCookie('4');
    const { result } = setup();

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    await waitFor(() => expect(result.current.redirectPath).toBeNull());

    expect(result.current.cookieBuildingId).toBe(4);
    expect(result.current.selectedBuilding).toBeNull();
    expect(document.cookie).not.toContain(`${ANON_BUILDING_COOKIE}=4`);
  });

  it('битая cookie не ломает страницу выбора', async () => {
    setCookie('not-a-number');
    const { result } = setup();

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(result.current.cookieBuildingId).toBeNull();
    expect(result.current.redirectPath).toBeNull();
  });

  it('selectBuilding запоминает здание и обновляет путь', async () => {
    const { result } = setup();
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      result.current.selectBuilding(3);
    });

    await waitFor(() => expect(result.current.redirectPath).toBe('/b/3'));
    expect(readAnonBuildingCookie()).toBe(3);
  });
});
