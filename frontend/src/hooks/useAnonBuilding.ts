/**
 * Публичная навигация по зданиям (§3.11 PUB-1..PUB-3).
 *
 * Выбранное анонимом здание хранится в cookie `anon_building_id`:
 *  - доступна JS (не httpOnly), SameSite=Lax, Path=/, срок 30 дней (PUB-3);
 *  - cookie не даёт доступа к закрытым данным, сервер всегда проверяет здание
 *    заново (NFR-8), поэтому клиент лишь подсказывает навигацию;
 *  - после `/` с валидной cookie сразу открывается `/b/{id}` (PUB-1);
 *  - если здание удалено/деактивировано — cookie чистится, остаёмся на выборе.
 */
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { fetchPublicBuildings } from '@/api/public';
import type { PublicBuilding } from '@/api/types';

export const ANON_BUILDING_COOKIE = 'anon_building_id';

/** 30 дней в секундах (PUB-3). */
export const ANON_BUILDING_COOKIE_MAX_AGE = 60 * 60 * 24 * 30;

/** Парсит `document.cookie`; нечисловое/отрицательное значение считается отсутствующим. */
export function readAnonBuildingCookie(cookieString?: string): number | null {
  const source = cookieString ?? (typeof document !== 'undefined' ? document.cookie : '');
  if (!source) return null;
  const match = new RegExp(`(?:^|;\\s*)${ANON_BUILDING_COOKIE}=(\\d+)(?:;|$)`).exec(source);
  if (!match?.[1]) return null;
  const id = Number.parseInt(match[1], 10);
  return Number.isSafeInteger(id) && id > 0 ? id : null;
}

/** Запоминает выбранное здание на 30 дней (PUB-3). */
export function writeAnonBuildingCookie(buildingId: number): void {
  if (typeof document === 'undefined') return;
  document.cookie = [
    `${ANON_BUILDING_COOKIE}=${buildingId}`,
    'Path=/',
    `Max-Age=${ANON_BUILDING_COOKIE_MAX_AGE}`,
    'SameSite=Lax',
  ].join('; ');
}

export function clearAnonBuildingCookie(): void {
  if (typeof document === 'undefined') return;
  document.cookie = `${ANON_BUILDING_COOKIE}=; Path=/; Max-Age=0; SameSite=Lax`;
}

export const anonBuildingsQueryKey = ['public', 'buildings'] as const;

export interface UseAnonBuildingResult {
  /** Активные здания со счётчиками (только они приходят из публичного API). */
  buildings: PublicBuilding[];
  isLoading: boolean;
  isError: boolean;
  error: Error | null;
  /** Значение cookie на момент открытия страницы. */
  cookieBuildingId: number | null;
  /** Текущее выбранное здание (может быть null: не выбрано или стало недоступно). */
  selectedBuilding: PublicBuilding | null;
  /** Готовый путь редиректа `/b/{id}` или null, если редиректить некуда. */
  redirectPath: string | null;
  selectBuilding: (buildingId: number) => void;
  clearSelection: () => void;
  refetch: () => void;
}

export interface UseAnonBuildingOptions {
  /** Отключить запрос списка (например, на странице конкретного здания). */
  enabled?: boolean;
}

export function useAnonBuilding(options: UseAnonBuildingOptions = {}): UseAnonBuildingResult {
  const { enabled = true } = options;
  const initialCookieId = useMemo(() => readAnonBuildingCookie(), []);
  const [selectedId, setSelectedId] = useState<number | null>(initialCookieId);

  const query = useQuery({
    queryKey: anonBuildingsQueryKey,
    queryFn: fetchPublicBuildings,
    staleTime: 60_000,
    enabled,
  });

  const buildings = useMemo(() => query.data ?? [], [query.data]);

  useEffect(() => {
    if (!query.isSuccess || selectedId === null) return;
    // Публичный список содержит только активные здания (TICKET-6):
    // отсутствие id означает удалённое/деактивированное здание (PUB-1).
    const exists = buildings.some((building) => building.id === selectedId && building.id > 0);
    if (!exists) {
      clearAnonBuildingCookie();
      setSelectedId(null);
    }
  }, [buildings, query.isSuccess, selectedId]);

  const selectedBuilding = useMemo(
    () => buildings.find((building) => building.id === selectedId) ?? null,
    [buildings, selectedId],
  );

  const selectBuilding = useCallback((buildingId: number) => {
    writeAnonBuildingCookie(buildingId);
    setSelectedId(buildingId);
  }, []);

  const clearSelection = useCallback(() => {
    clearAnonBuildingCookie();
    setSelectedId(null);
  }, []);

  return {
    buildings,
    isLoading: query.isLoading,
    isError: query.isError,
    error: (query.error as Error | null) ?? null,
    cookieBuildingId: initialCookieId,
    selectedBuilding,
    redirectPath: selectedBuilding ? `/b/${selectedBuilding.id}` : null,
    selectBuilding,
    clearSelection,
    refetch: () => {
      void query.refetch();
    },
  };
}
