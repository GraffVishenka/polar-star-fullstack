-- 0002_users_admin: поддержка админ-списка пользователей и подбора исполнителей.
--
-- FK users.building_id → buildings(id) ON DELETE RESTRICT уже создан в
-- 0001_core, поэтому повторно его не добавляем (AUTH-4: у USER ровно одно
-- здание, у ADMIN закрепления нет). Здесь только индексы под запросы Б2:
-- фильтр списка по зданию/роли/активности и сортировка по ФИО,
-- а также выборка активных пользователей здания для assignable (TICKET-8).

CREATE INDEX idx_users_building_role_active
    ON users (building_id, role, is_active);

CREATE INDEX idx_users_full_name
    ON users (full_name);
