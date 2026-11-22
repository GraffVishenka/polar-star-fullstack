/** Фильтр периода статистики: месяц / квартал / год / произвольный диапазон. */
import { DatePicker, Flex, Segmented } from 'antd';
import dayjs from 'dayjs';
import {
  DATE_FORMAT,
  PERIOD_PRESET_OPTIONS,
  customStatsPeriod,
  periodRange,
  type PeriodPreset,
  type StatsPeriod,
} from '../period';

export interface StatsPeriodFilterProps {
  period: StatsPeriod;
  onChange: (period: StatsPeriod) => void;
  loading?: boolean;
  /** Уникальный префикс testid для страниц, где фильтров несколько. */
  testIdPrefix?: string;
}

export function StatsPeriodFilter({
  period,
  onChange,
  loading = false,
  testIdPrefix = 'stats-period',
}: StatsPeriodFilterProps) {
  const handlePreset = (value: PeriodPreset) => {
    if (value === 'custom') {
      // «Период» — оставляем выбранный диапазон, переключаем только режим.
      onChange({ preset: 'custom', range: period.range });
      return;
    }
    onChange({ preset: value, range: periodRange(value) });
  };

  return (
    <Flex gap={8} wrap align="center" data-testid={testIdPrefix}>
      <Segmented<PeriodPreset>
        value={period.preset}
        options={PERIOD_PRESET_OPTIONS}
        onChange={handlePreset}
        disabled={loading}
        aria-label="Период статистики"
        data-testid={`${testIdPrefix}-preset`}
      />
      <DatePicker.RangePicker
        value={[dayjs(period.range.from, DATE_FORMAT), dayjs(period.range.to, DATE_FORMAT)]}
        format="DD.MM.YYYY"
        allowClear={false}
        disabled={loading}
        onChange={(values) => {
          const from = values?.[0];
          const to = values?.[1];
          if (!from || !to) return;
          onChange(customStatsPeriod(from, to));
        }}
        aria-label="Произвольный период"
        data-testid={`${testIdPrefix}-range`}
      />
    </Flex>
  );
}
