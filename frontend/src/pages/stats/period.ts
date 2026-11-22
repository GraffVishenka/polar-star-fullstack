/**
 * Период статистики (STATS-1/STATS-4): пресеты «месяц/квартал/год/произвольно»
 * и форматирование метрик. Чистые функции — без React, покрыты unit-тестами.
 *
 * Дефолт — текущий месяц (границы считаются на клиенте только для фильтра;
 * сами метрики и «в срок» считает сервер, NFR-7).
 */
import dayjs, { type Dayjs } from 'dayjs';
import type { StatsParams } from '@/api/types';

export type PeriodPreset = 'month' | 'quarter' | 'year' | 'custom';

export interface PeriodRange {
  /** `YYYY-MM-DD` включительно. */
  from: string;
  /** `YYYY-MM-DD` включительно. */
  to: string;
}

export interface StatsPeriod {
  preset: PeriodPreset;
  range: PeriodRange;
}

export const PERIOD_PRESET_OPTIONS: Array<{ value: PeriodPreset; label: string }> = [
  { value: 'month', label: 'Месяц' },
  { value: 'quarter', label: 'Квартал' },
  { value: 'year', label: 'Год' },
  { value: 'custom', label: 'Период' },
];

export const DATE_FORMAT = 'YYYY-MM-DD';
export const DISPLAY_DATE_FORMAT = 'DD.MM.YYYY';

const MONTHS_RU = [
  'январь',
  'февраль',
  'март',
  'апрель',
  'май',
  'июнь',
  'июль',
  'август',
  'сентябрь',
  'октябрь',
  'ноябрь',
  'декабрь',
];

/** Границы пресета в `YYYY-MM-DD`; `now` инжектится для тестов. */
export function periodRange(
  preset: Exclude<PeriodPreset, 'custom'>,
  now: Dayjs = dayjs(),
): PeriodRange {
  if (preset === 'month') {
    return {
      from: now.startOf('month').format(DATE_FORMAT),
      to: now.endOf('month').format(DATE_FORMAT),
    };
  }
  if (preset === 'quarter') {
    const quarterStartMonth = Math.floor(now.month() / 3) * 3;
    const start = now.month(quarterStartMonth).startOf('month');
    return {
      from: start.format(DATE_FORMAT),
      to: start.add(2, 'month').endOf('month').format(DATE_FORMAT),
    };
  }
  return {
    from: now.startOf('year').format(DATE_FORMAT),
    to: now.endOf('year').format(DATE_FORMAT),
  };
}

/** Дефолтный период страниц статистики — текущий месяц. */
export function defaultStatsPeriod(now: Dayjs = dayjs()): StatsPeriod {
  return { preset: 'month', range: periodRange('month', now) };
}

/** Период из произвольных дат (DatePicker): некорректный диапазон выравниваем. */
export function customStatsPeriod(from: Dayjs, to: Dayjs): StatsPeriod {
  const [start, end] = from.isAfter(to) ? [to, from] : [from, to];
  return {
    preset: 'custom',
    range: { from: start.format(DATE_FORMAT), to: end.format(DATE_FORMAT) },
  };
}

/** Параметры запроса к API (`from`/`to`). */
export function toStatsParams(period: StatsPeriod): StatsParams {
  return { from: period.range.from, to: period.range.to };
}

/** «01.11.2026 — 30.11.2026» для подписи периода. */
export function formatPeriodRange(range: PeriodRange): string {
  const from = dayjs(range.from);
  const to = dayjs(range.to);
  const format = (date: Dayjs) => (date.isValid() ? date.format(DISPLAY_DATE_FORMAT) : '—');
  return `${format(from)} — ${format(to)}`;
}

/** Число с русским разделителем: 1234 → «1 234». */
export function formatCount(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—';
  return new Intl.NumberFormat('ru-RU').format(value);
}

/** Часы с одним знаком: 8.4 → «8,4 ч»; null → «—» (STATS-2). */
export function formatHours(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—';
  return `${new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 }).format(value)} ч`;
}

/** Проценты: 90 → «90 %»; null → «—» (нет завершённых, STATS-2). */
export function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—';
  return `${new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 1 }).format(value)} %`;
}

/** `YYYY-MM` → «март 2026» (месяцы по-русски, без зависимости от локали dayjs). */
export function formatMonthLabel(month: string): string {
  const parsed = /^(\d{4})-(\d{2})$/.exec(month);
  if (!parsed) return month;
  const year = Number(parsed[1]);
  const monthIndex = Number(parsed[2]) - 1;
  if (monthIndex < 0 || monthIndex > 11) return month;
  return `${MONTHS_RU[monthIndex]} ${year}`;
}

/** Пустая статистика: нет завершённых ни за всё время, ни за период. */
export function isEmptyStats(
  stats:
    | {
        total_done: number;
        done_in_period: number;
      }
    | null
    | undefined,
): boolean {
  if (!stats) return false;
  return stats.total_done === 0 && stats.done_in_period === 0;
}
