/**
 * Локальные аддитивные типы статистики (STATS-1).
 *
 * `src/api/types.ts` генерируется из контракта, поэтому расширение для сводки
 * по сотрудникам живёт здесь и согласуется с lead/be. Бэкенд может прислать
 * `employees` в `GET /api/admin/stats` без `user_id`; если поля нет — блок скрыт.
 */
import type { StatsResponse } from '@/api/types';

export interface EmployeeStats {
  user_id: number;
  user_name: string;
  total_done: number;
  done_in_period: number;
  avg_resolution_hours: number | null;
  median_resolution_hours: number | null;
  on_time_percent: number | null;
  overdue_count: number;
}

export interface StatsWithEmployees extends StatsResponse {
  /** Аддитивное расширение контракта: сводка по сотрудникам для ADMIN. */
  employees?: EmployeeStats[];
}
