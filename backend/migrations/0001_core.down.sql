-- Откат 0001_core (только для локальной разработки; миграции в production
-- выполняются только вперёд, NFR-5).
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS holidays;
DROP TABLE IF EXISTS equipment_types;
DROP TABLE IF EXISTS problem_types;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS buildings;
