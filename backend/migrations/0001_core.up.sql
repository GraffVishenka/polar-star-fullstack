-- 0001_core: фундамент БД (Б1).
-- Таблицы: buildings, users, refresh_tokens, settings, problem_types,
-- equipment_types, holidays. Доменные таблицы (заявки, БЗ, оборудование)
-- добавляются миграциями своих блоков (Б2+, уникальный префикс домена).
--
-- Все DATETIME хранятся в UTC (DSN loc=UTC, MySQL default-time-zone=+00:00),
-- бизнес-даты считаются в timezone приложения (CAL-2, NFR-7).

-- Здания: не удаляются, а деактивируются (PLAN §4, п. 8; EQ-1, EQ-6).
CREATE TABLE buildings (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(200)    NOT NULL,
    address     VARCHAR(500)    NULL,
    is_active   TINYINT(1)      NOT NULL DEFAULT 1,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_buildings_active (is_active, name)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Пользователи (AUTH-4): аккаунты создаёт только ADMIN; у USER обязательно
-- одно здание, у ADMIN закрепления нет. is_active проверяется на каждом
-- запросе (AUTH-6), must_change_password даёт 428 (AUTH-4).
CREATE TABLE users (
    id                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    login                VARCHAR(64)     NOT NULL,
    full_name            VARCHAR(200)    NOT NULL,
    role                 ENUM ('USER', 'ADMIN') NOT NULL DEFAULT 'USER',
    password_hash        VARCHAR(255)    NOT NULL,
    building_id          BIGINT UNSIGNED NULL,
    is_active            TINYINT(1)      NOT NULL DEFAULT 1,
    must_change_password TINYINT(1)      NOT NULL DEFAULT 0,
    created_at           DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_login (login),
    KEY idx_users_building (building_id),
    KEY idx_users_role_active (role, is_active),
    CONSTRAINT fk_users_building
        FOREIGN KEY (building_id) REFERENCES buildings (id)
            ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Refresh-токены (AUTH-2): хранится SHA-256 хеш, ротация при каждом refresh,
-- replaced_by связывает цепочку для отзыва при повторном использовании.
CREATE TABLE refresh_tokens (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id     BIGINT UNSIGNED NOT NULL,
    token_hash  CHAR(64)        NOT NULL,
    expires_at  DATETIME        NOT NULL,
    revoked_at  DATETIME        NULL,
    replaced_by BIGINT UNSIGNED NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_refresh_tokens_hash (token_hash),
    KEY idx_refresh_tokens_user (user_id, revoked_at),
    KEY idx_refresh_tokens_expires (expires_at),
    CONSTRAINT fk_refresh_tokens_user
        FOREIGN KEY (user_id) REFERENCES users (id)
            ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_refresh_tokens_replaced
        FOREIGN KEY (replaced_by) REFERENCES refresh_tokens (id)
            ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Настройки — singleton (id=1). CAL-1..CAL-3, TICKET-1.
-- workdays: JSON-массив номеров дней ISO-8601 (1=Пн..7=Вс).
-- status_labels/status_colors: JSON-объекты по кодам статусов new|in_progress|deferred|done.
CREATE TABLE settings (
    id                     TINYINT UNSIGNED NOT NULL DEFAULT 1,
    app_name               VARCHAR(200)     NOT NULL,
    deadline_days          TINYINT UNSIGNED NOT NULL DEFAULT 2,
    workdays               JSON             NOT NULL,
    timezone               VARCHAR(64)      NOT NULL DEFAULT 'Asia/Yekaterinburg',
    deadline_reminder_time CHAR(5)          NOT NULL DEFAULT '10:00',
    push_enabled           TINYINT(1)       NOT NULL DEFAULT 1,
    status_labels          JSON             NOT NULL,
    status_colors          JSON             NOT NULL,
    updated_at             DATETIME         NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT chk_settings_singleton CHECK (id = 1),
    CONSTRAINT chk_settings_deadline_days CHECK (deadline_days BETWEEN 1 AND 30)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Справочник видов проблем (TICKET-2). Названия уникальны, чтобы сидинг был
-- идемпотентным (INSERT IGNORE).
CREATE TABLE problem_types (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(120)    NOT NULL,
    is_active  TINYINT(1)      NOT NULL DEFAULT 1,
    sort_order INT             NOT NULL DEFAULT 0,
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_problem_types_name (name),
    KEY idx_problem_types_active (is_active, sort_order)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Справочник типов оборудования (EQ-2).
CREATE TABLE equipment_types (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name       VARCHAR(120)    NOT NULL,
    is_active  TINYINT(1)      NOT NULL DEFAULT 1,
    sort_order INT             NOT NULL DEFAULT 0,
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_equipment_types_name (name),
    KEY idx_equipment_types_active (is_active, sort_order)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Праздники — исключаемые даты рабочего календаря (CAL-1). Дата уникальна.
CREATE TABLE holidays (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    holiday_date DATE            NOT NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_holidays_date (holiday_date)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
