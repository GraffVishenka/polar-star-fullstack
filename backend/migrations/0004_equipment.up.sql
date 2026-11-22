-- 0004_equipment: склады, оборудование и журнал перемещений (Б4, EQ-1..EQ-7).
--
-- Схема: building (1) -> warehouse (N) -> equipment (N); equipment_movements —
-- неизменяемый журнал перемещений (EQ-5). Все DATETIME хранятся в UTC
-- (DSN loc=UTC, MySQL default-time-zone=+00:00), бизнес-даты считаются в
-- timezone приложения (CAL-2, NFR-7).
--
-- Решения по схеме:
--  * `deleted_at` в warehouses/equipment — мягкое удаление/корзина. Физическое
--    удаление склада возможно только для пустого склада (EQ-6), поэтому колонка
--    пока не используется удалением, но нужна для TRASH и для UNIQUE-ключа,
--    заданного ТЗ.
--  * Индексы `*_live` — generated columns: в MySQL NULL-значения в UNIQUE-ключе
--    считаются различными, поэтому UNIQUE(building_id, name, deleted_at) сам по
--    себе не запрещает два активных склада-тёзки. Дополнительный UNIQUE по
--    generated column закрывает эту дыру: у неудалённой строки ключ заполнен, у
--    удалённой — NULL и не конфликтует.
--  * Журнал перемещений неизменяем: у таблицы нет updated_at, API не отдаёт
--    PATCH/DELETE, а FK на склады — RESTRICT, поэтому история не «портится»
--    удалением справочников.

-- Склады/кабинеты здания (EQ-1). Имя уникально в рамках здания.
CREATE TABLE warehouses (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    building_id BIGINT UNSIGNED NOT NULL,
    name        VARCHAR(200)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME        NULL,
    live_name   VARCHAR(200) GENERATED ALWAYS AS (IF(deleted_at IS NULL, name, NULL)) STORED,
    PRIMARY KEY (id),
    UNIQUE KEY uq_warehouses_building_name (building_id, name, deleted_at),
    UNIQUE KEY uq_warehouses_building_live_name (building_id, live_name),
    KEY idx_warehouses_building (building_id, deleted_at),
    CONSTRAINT fk_warehouses_building
        FOREIGN KEY (building_id) REFERENCES buildings (id)
            ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Единицы оборудования (EQ-2). Склад обязателен: состояния «не размещено» нет.
-- registration_number уникален среди неудалённых (EQ-2), регистронезависимо —
-- collation таблицы utf8mb4_0900_ai_ci.
CREATE TABLE equipment (
    id                       BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name                     VARCHAR(200)    NOT NULL,
    equipment_type_id        BIGINT UNSIGNED NOT NULL,
    serial_number            VARCHAR(100)    NOT NULL,
    registration_number      VARCHAR(100)    NOT NULL,
    warehouse_id             BIGINT UNSIGNED NOT NULL,
    created_at               DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at               DATETIME        NULL,
    registration_number_live VARCHAR(100) GENERATED ALWAYS AS (
        IF(deleted_at IS NULL, registration_number, NULL)
        ) STORED,
    PRIMARY KEY (id),
    UNIQUE KEY uq_equipment_registration_live (registration_number_live),
    KEY idx_equipment_warehouse (warehouse_id, deleted_at),
    KEY idx_equipment_type (equipment_type_id, deleted_at),
    KEY idx_equipment_deleted (deleted_at),
    -- Поиск EQ-7 идёт по названию/серийному/регистрационному номеру; префиксные
    -- индексы помогают сортировкам и точным сравнениям, LIKE '%...%' — скан.
    KEY idx_equipment_name (name),
    KEY idx_equipment_serial (serial_number),
    CONSTRAINT fk_equipment_type
        FOREIGN KEY (equipment_type_id) REFERENCES equipment_types (id)
            ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_equipment_warehouse
        FOREIGN KEY (warehouse_id) REFERENCES warehouses (id)
            ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Журнал перемещений (EQ-3, EQ-5): только INSERT, без PATCH/DELETE. Имена
-- складов/зданий/пользователя отдаются join'ом, поэтому записи не дублируют
-- названия и не устаревают при переименовании.
CREATE TABLE equipment_movements (
    id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    equipment_id      BIGINT UNSIGNED NOT NULL,
    from_warehouse_id BIGINT UNSIGNED NOT NULL,
    to_warehouse_id   BIGINT UNSIGNED NOT NULL,
    moved_by_user_id  BIGINT UNSIGNED NOT NULL,
    moved_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    comment           VARCHAR(1000)   NULL,
    PRIMARY KEY (id),
    KEY idx_movements_equipment (equipment_id, moved_at DESC, id DESC),
    KEY idx_movements_from (from_warehouse_id),
    KEY idx_movements_to (to_warehouse_id),
    KEY idx_movements_user (moved_by_user_id),
    CONSTRAINT fk_movements_equipment
        FOREIGN KEY (equipment_id) REFERENCES equipment (id)
            ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_movements_from_warehouse
        FOREIGN KEY (from_warehouse_id) REFERENCES warehouses (id)
            ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_movements_to_warehouse
        FOREIGN KEY (to_warehouse_id) REFERENCES warehouses (id)
            ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_movements_user
        FOREIGN KEY (moved_by_user_id) REFERENCES users (id)
            ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
