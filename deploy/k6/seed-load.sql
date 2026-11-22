-- Нагрузочные данные для k6 (Б13): 10 000 заявок.
--
-- Запуск после `make demo` (нужны здание, вид проблемы и сотрудник):
--   docker compose exec -T mysql \
--     mysql -upolar -ppolar polar_star < deploy/k6/seed-load.sql
--
-- Скрипт идемпотентен по маркеру '[load]' в описании: повторный вызов
-- досоздаёт только недостающие заявки. Заявки создаются в первом активном
-- здании, чтобы фильтры и дашборд работали без дополнительной настройки.

SET @building_id := (SELECT id FROM buildings WHERE is_active = 1 ORDER BY id LIMIT 1);
SET @problem_type_id := (SELECT id FROM problem_types ORDER BY sort_order, id LIMIT 1);
SET @user_id := (SELECT id FROM users WHERE role = 'USER' ORDER BY id LIMIT 1);

DROP PROCEDURE IF EXISTS seed_load_tickets;

DELIMITER //
CREATE PROCEDURE seed_load_tickets(IN total INT)
BEGIN
    DECLARE existing INT DEFAULT 0;
    DECLARE i INT DEFAULT 1;

    SELECT COUNT(*) INTO existing FROM tickets WHERE description LIKE '[load]%';
    IF existing >= total THEN
        SELECT CONCAT('нагрузочные заявки уже есть: ', existing) AS status;
    ELSE
        WHILE i <= total DO
            INSERT INTO tickets
                (building_id, applicant_name, cabinet, problem_type_id, description,
                 deadline, executor_id, status, created_at, updated_at)
            VALUES
                (@building_id, CONCAT('Нагрузка ', i), '101', @problem_type_id,
                 CONCAT('[load] заявка ', i),
                 DATE_ADD(CURDATE(), INTERVAL 2 DAY),
                 CASE WHEN i % 3 = 0 THEN NULL ELSE @user_id END,
                 CASE i % 4 WHEN 0 THEN 'done' WHEN 1 THEN 'new'
                            WHEN 2 THEN 'in_progress' ELSE 'deferred' END,
                 UTC_TIMESTAMP() - INTERVAL (i % 120) DAY,
                 UTC_TIMESTAMP());
            SET i := i + 1;
        END WHILE;
        SELECT CONCAT('создано нагрузочных заявок: ', total) AS status;
    END IF;
END //
DELIMITER ;

CALL seed_load_tickets(10000);

DROP PROCEDURE IF EXISTS seed_load_tickets;
