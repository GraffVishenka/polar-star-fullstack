-- Откат 0002_users_admin: только индексы, данные и FK не затрагиваются.
DROP INDEX idx_users_full_name ON users;
DROP INDEX idx_users_building_role_active ON users;
