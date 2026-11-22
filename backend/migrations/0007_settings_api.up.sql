-- 0007_settings_api: аддитивные доработки настроек и календаря (Б7, CAL-1..CAL-3).
--
-- settings из 0001_core уже содержит все поля контракта (app_name,
-- deadline_days, workdays, timezone, deadline_reminder_time, push_enabled,
-- status_labels, status_colors), holidays (holiday_date UNIQUE) тоже создана в
-- 0001 — пересоздавать их не нужно. Миграция добавляет только индекс под
-- фильтр `GET /api/admin/holidays?year=`: YEAR(holiday_date) в WHERE индекс не
-- использует, поэтому год выносится в generated-колонку.
ALTER TABLE holidays
    ADD COLUMN holiday_year SMALLINT GENERATED ALWAYS AS (YEAR(holiday_date)) STORED,
    ADD KEY idx_holidays_year (holiday_year, holiday_date);
