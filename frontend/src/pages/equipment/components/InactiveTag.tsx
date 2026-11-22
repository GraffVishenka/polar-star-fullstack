/**
 * Бейдж неактивной сущности (EQ-1/EQ-6).
 *
 * Деактивированное здание исчезает из публичного выбора и формы заявки,
 * но остаётся в истории, фильтрах и карточке оборудования — там его и помечаем.
 */
import { Tag } from 'antd';

export interface InactiveTagProps {
  /** Меньший размер для таблиц и выпадающих списков. */
  compact?: boolean;
}

export function InactiveTag({ compact = false }: InactiveTagProps) {
  return (
    <Tag
      color="default"
      data-testid="inactive-badge"
      style={{ marginInlineStart: 6, fontSize: compact ? 11 : 12, lineHeight: '18px' }}
    >
      Неактивно
    </Tag>
  );
}

export interface EntityNameProps {
  name: string;
  inactive?: boolean;
  compact?: boolean;
}

/** Название сущности с бейджем «Неактивно» — единый вид в фильтрах, таблице и журнале. */
export function EntityName({ name, inactive = false, compact = false }: EntityNameProps) {
  return (
    <span>
      {name}
      {inactive ? <InactiveTag compact={compact} /> : null}
    </span>
  );
}
