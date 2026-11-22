/**
 * Кнопка «Ведомость помещения» (EQ-8, DOC-2). Готова к подключению на странице
 * склада/помещения (fe-3) — страницу `/equipment` этот блок не трогает.
 */
import { FileExcelOutlined } from '@ant-design/icons';
import type { ApiError } from '@/api/errors';
import { warehouseInventoryPath } from '@/api/equipment';
import { ExportButton } from '@/components/ExportButton';
import { inventoryErrorMessage } from './useInventoryExport';

export interface InventoryExportButtonProps {
  warehouseId: number | null | undefined;
  label?: string;
  size?: 'small' | 'middle' | 'large';
  type?: 'primary' | 'default' | 'dashed' | 'text' | 'link';
  block?: boolean;
  disabled?: boolean;
  onSuccess?: (filename: string) => void;
  onError?: (error: ApiError) => void;
}

export function InventoryExportButton({
  warehouseId,
  label = 'Ведомость помещения',
  size = 'middle',
  type = 'default',
  block = false,
  disabled = false,
  onSuccess,
  onError,
}: InventoryExportButtonProps) {
  const valid = typeof warehouseId === 'number' && Number.isInteger(warehouseId) && warehouseId > 0;

  return (
    <ExportButton
      path={valid ? warehouseInventoryPath(warehouseId) : ''}
      filename={valid ? `inventory_${warehouseId}.xlsx` : undefined}
      label={label}
      icon={<FileExcelOutlined />}
      size={size}
      type={type}
      block={block}
      disabled={disabled || !valid}
      errorMessage={inventoryErrorMessage}
      errorHint={
        valid ? undefined : 'Выберите помещение (склад/кабинет), чтобы сформировать ведомость'
      }
      successMessage="Ведомость готова"
      onSuccess={onSuccess}
      onError={onError}
    />
  );
}
