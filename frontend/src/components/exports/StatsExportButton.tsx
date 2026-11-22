/**
 * Кнопка «Выгрузить в Excel» для страниц статистики (STATS-3, DOC-2).
 * Используется fe-2 в `/profile` и `/admin/stats` (Ф10) — страницы не создаём.
 */
import { FileExcelOutlined } from '@ant-design/icons';
import type { AdminStatsParams } from '@/api/types';
import type { ApiError } from '@/api/errors';
import { statsExportPath } from '@/api/stats';
import { ExportButton } from '@/components/ExportButton';
import { statsErrorMessage } from './useStatsExport';

export interface StatsExportButtonProps {
  params?: AdminStatsParams;
  label?: string;
  size?: 'small' | 'middle' | 'large';
  type?: 'primary' | 'default' | 'dashed' | 'text' | 'link';
  block?: boolean;
  disabled?: boolean;
  onSuccess?: (filename: string) => void;
  onError?: (error: ApiError) => void;
}

export function StatsExportButton({
  params = {},
  label = 'Выгрузить в Excel',
  size = 'middle',
  type = 'default',
  block = false,
  disabled = false,
  onSuccess,
  onError,
}: StatsExportButtonProps) {
  return (
    <ExportButton
      path={statsExportPath(params)}
      label={label}
      icon={<FileExcelOutlined />}
      size={size}
      type={type}
      block={block}
      disabled={disabled}
      errorMessage={statsErrorMessage}
      successMessage="Файл статистики готов"
      onSuccess={onSuccess}
      onError={onError}
    />
  );
}
