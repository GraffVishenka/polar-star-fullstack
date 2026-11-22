/** Чистые функции периода/форматирования статистики (STATS-1/STATS-2). */
import dayjs from 'dayjs';
import { describe, expect, it } from 'vitest';
import {
  customStatsPeriod,
  defaultStatsPeriod,
  formatCount,
  formatHours,
  formatMonthLabel,
  formatPercent,
  formatPeriodRange,
  isEmptyStats,
  periodRange,
  toStatsParams,
} from '../period';

describe('periodRange', () => {
  const now = dayjs('2026-11-22');

  it('месяц — с первого по последний день', () => {
    expect(periodRange('month', now)).toEqual({ from: '2026-11-01', to: '2026-11-30' });
  });

  it('квартал — календарные границы (Q4: октябрь–декабрь)', () => {
    expect(periodRange('quarter', now)).toEqual({ from: '2026-10-01', to: '2026-12-31' });
  });

  it('год — с 1 января по 31 декабря', () => {
    expect(periodRange('year', now)).toEqual({ from: '2026-01-01', to: '2026-12-31' });
  });

  it('дефолт — текущий месяц, параметры — from/to', () => {
    const period = defaultStatsPeriod(now);
    expect(period.preset).toBe('month');
    expect(toStatsParams(period)).toEqual({ from: '2026-11-01', to: '2026-11-30' });
  });
});

describe('customStatsPeriod', () => {
  it('переставляет даты, если пользователь выбрал диапазон наоборот', () => {
    const period = customStatsPeriod(dayjs('2026-12-31'), dayjs('2026-01-01'));
    expect(period.preset).toBe('custom');
    expect(period.range).toEqual({ from: '2026-01-01', to: '2026-12-31' });
  });
});

describe('форматирование метрик', () => {
  it('null-метрики показываются как «—» (STATS-2)', () => {
    expect(formatHours(null)).toBe('—');
    expect(formatPercent(undefined)).toBe('—');
    expect(formatCount(null)).toBe('—');
  });

  it('часы/проценты/числа — по-русски', () => {
    expect(formatHours(8.4)).toBe('8,4 ч');
    expect(formatPercent(90)).toBe('90 %');
    expect(formatCount(1234)).toMatch(/^1\s234$/);
  });

  it('месяц `YYYY-MM` подписывается по-русски', () => {
    expect(formatMonthLabel('2026-03')).toBe('март 2026');
    expect(formatMonthLabel('2026-11')).toBe('ноябрь 2026');
    expect(formatMonthLabel('bad')).toBe('bad');
  });

  it('диапазон периода — DD.MM.YYYY', () => {
    expect(formatPeriodRange({ from: '2026-11-01', to: '2026-11-30' })).toBe(
      '01.11.2026 — 30.11.2026',
    );
  });

  it('пустая статистика определяется по нулевым завершённым', () => {
    expect(isEmptyStats({ total_done: 0, done_in_period: 0 })).toBe(true);
    expect(isEmptyStats({ total_done: 2, done_in_period: 0 })).toBe(false);
    expect(isEmptyStats(null)).toBe(false);
  });
});
