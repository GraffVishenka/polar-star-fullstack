-- Откат 0007_settings_api: только добавленный индекс/колонка (двумя
-- statements, чтобы не зависеть от порядка применения в одном ALTER).
ALTER TABLE holidays DROP KEY idx_holidays_year;
ALTER TABLE holidays DROP COLUMN holiday_year;
