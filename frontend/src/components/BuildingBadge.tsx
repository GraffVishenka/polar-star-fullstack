/**
 * BuildingBadge — компактная плашка здания.
 *
 * Показывается в шапке приложения и карточках заявок. Отдельно обработан случай
 * AUTH-9: у USER не назначено здание — плашка предупреждает и просит
 * обратиться к администратору.
 */
import { Tag, Tooltip } from 'antd';
import { BankOutlined, GlobalOutlined, WarningOutlined } from '@ant-design/icons';

export interface BuildingBadgeProps {
  buildingId?: number | null;
  buildingName?: string | null;
  /** Сводный режим «Все здания» (ADMIN, публичный обзор). */
  allBuildings?: boolean;
  /** Компактный размер для карточек. */
  size?: 'default' | 'small';
  /** Показывать id в подсказке (удобно при отладке). */
  className?: string;
}

export function BuildingBadge({
  buildingId,
  buildingName,
  allBuildings = false,
  size = 'default',
  className,
}: BuildingBadgeProps) {
  if (allBuildings) {
    return (
      <Tag
        icon={<GlobalOutlined />}
        color="blue"
        className={className}
        data-testid="building-badge-all"
        style={{ marginInlineEnd: 0, fontSize: size === 'small' ? 11 : 12 }}
      >
        Все здания
      </Tag>
    );
  }

  if (!buildingName) {
    return (
      <Tooltip title="Обратитесь к администратору: сотруднику должно быть назначено здание">
        <Tag
          icon={<WarningOutlined />}
          color="warning"
          className={className}
          data-testid="building-badge-missing"
          style={{ marginInlineEnd: 0, fontSize: size === 'small' ? 11 : 12 }}
        >
          Здание не назначено
        </Tag>
      </Tooltip>
    );
  }

  return (
    <Tag
      icon={<BankOutlined />}
      className={className}
      color="geekblue"
      data-testid="building-badge"
      title={buildingId ? `ID здания: ${buildingId}` : undefined}
      style={{ marginInlineEnd: 0, fontSize: size === 'small' ? 11 : 12 }}
    >
      {buildingName}
    </Tag>
  );
}
