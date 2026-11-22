/**
 * BuildingSelect — выбор здания (TICKET-6, PUB-4, Ф2).
 *
 * Роли:
 *  - аноним/ADMIN: активные здания + пункт «Все здания» (`value = null`);
 *  - USER: фильтр заблокирован на своём здании (TICKET-7) — селект `disabled`
 *    с подсказкой, чтобы не выглядел «просто нерабочим».
 */
import { Select, Tooltip } from 'antd';
import { BankOutlined, GlobalOutlined, LockOutlined } from '@ant-design/icons';

export interface BuildingOption {
  id: number;
  name: string;
}

export interface BuildingSelectProps {
  buildings: BuildingOption[];
  /** `null` — режим «Все здания». */
  value?: number | null;
  onChange?: (buildingId: number | null) => void;
  /** USER: доступно только своё здание. */
  locked?: boolean;
  /** Показывать пункт «Все здания» (ADMIN/публичный обзор). */
  allowAll?: boolean;
  loading?: boolean;
  disabled?: boolean;
  size?: 'small' | 'middle' | 'large';
  placeholder?: string;
  /** Текст подсказки при `locked`. */
  lockedHint?: string;
  className?: string;
  id?: string;
}

export function BuildingSelect({
  buildings,
  value,
  onChange,
  locked = false,
  allowAll = true,
  loading = false,
  disabled = false,
  size = 'middle',
  placeholder = 'Выберите здание',
  lockedHint = 'Доступно только ваше здание',
  className,
  id,
}: BuildingSelectProps) {
  const options = [
    ...(allowAll
      ? [
          {
            value: '__all__',
            label: (
              <span>
                <GlobalOutlined style={{ marginInlineEnd: 6 }} />
                Все здания
              </span>
            ),
          },
        ]
      : []),
    ...buildings.map((building) => ({
      value: String(building.id),
      label: (
        <span>
          <BankOutlined style={{ marginInlineEnd: 6 }} />
          {building.name}
        </span>
      ),
    })),
  ];

  const handleChange = (next: string | undefined) => {
    if (!onChange) return;
    if (next === '__all__' || next === undefined) onChange(null);
    else onChange(Number(next));
  };

  const select = (
    <Select
      id={id}
      className={className}
      aria-label="Фильтр по зданию"
      data-testid="building-select"
      style={{ minWidth: 200, width: '100%' }}
      size={size}
      value={value === null || value === undefined ? '__all__' : String(value)}
      options={options}
      loading={loading}
      disabled={disabled || locked || loading}
      placeholder={placeholder}
      onChange={handleChange}
      showSearch
      optionFilterProp="label"
    />
  );

  if (locked) {
    return (
      <Tooltip title={lockedHint}>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, width: '100%' }}>
          <LockOutlined style={{ color: '#8c8c8c' }} />
          {select}
        </span>
      </Tooltip>
    );
  }

  return select;
}
