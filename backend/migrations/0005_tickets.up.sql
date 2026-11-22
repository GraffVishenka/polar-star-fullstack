-- 0005_tickets: заявки, комментарии и связи с файлами (TICKET-1..TICKET-14).
--
-- Здание обязательно (TICKET-2/TICKET-4); дедлайн — DATE, считается сервером по
-- рабочему календарю (TICKET-3). Удаление — только в корзину (deleted_at).
-- access_token_hash — SHA-256 токена анонимной заявки (TICKET-13), открытый
-- токен в БД не хранится.
--
-- ВНИМАНИЕ: comment_files.file_id не получает FK на files, потому что таблица
-- files создаётся в Б3 (миграция 0003) и на момент Б5 ещё не смержена.
-- FK нужно добавить отдельной миграцией после мержа Б3 (см. отчёт Б5).

CREATE TABLE tickets (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    building_id        BIGINT UNSIGNED NOT NULL,
    applicant_name     VARCHAR(120)    NOT NULL,
    cabinet            VARCHAR(32)     NOT NULL,
    problem_type_id    BIGINT UNSIGNED NOT NULL,
    description        TEXT            NOT NULL,
    deadline           DATE            NULL,
    executor_id        BIGINT UNSIGNED NULL,
    status             ENUM ('new', 'in_progress', 'deferred', 'done') NOT NULL DEFAULT 'new',
    access_token_hash  CHAR(64)        NULL,
    created_by_user_id BIGINT UNSIGNED NULL,
    completed_at       DATETIME        NULL,
    created_at         DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at         DATETIME        NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_tickets_access_token_hash (access_token_hash),
    KEY idx_tickets_building_status_created (building_id, status, created_at),
    KEY idx_tickets_building_deadline (building_id, deadline),
    KEY idx_tickets_executor_status (executor_id, status),
    KEY idx_tickets_deleted (deleted_at),
    CONSTRAINT fk_tickets_building
        FOREIGN KEY (building_id) REFERENCES buildings (id)
            ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_tickets_problem_type
        FOREIGN KEY (problem_type_id) REFERENCES problem_types (id)
            ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_tickets_executor
        FOREIGN KEY (executor_id) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE,
    CONSTRAINT fk_tickets_created_by
        FOREIGN KEY (created_by_user_id) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Комментарии: плоский список (TICKET-14), автор — пользователь или имя
-- анонима; удаление мягкое (в корзину вместе с заявкой/B11).
CREATE TABLE ticket_comments (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    ticket_id      BIGINT UNSIGNED NOT NULL,
    author_user_id BIGINT UNSIGNED NULL,
    author_name    VARCHAR(200)    NOT NULL,
    body           TEXT            NOT NULL,
    created_at     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at     DATETIME        NULL,
    PRIMARY KEY (id),
    KEY idx_ticket_comments_ticket (ticket_id, created_at),
    CONSTRAINT fk_ticket_comments_ticket
        FOREIGN KEY (ticket_id) REFERENCES tickets (id)
            ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT fk_ticket_comments_author
        FOREIGN KEY (author_user_id) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Вложения комментария: до 5 файлов (TICKET-14). FK на files добавляется после
-- мержа Б3; уникальность пары защищает от дублей при повторной привязке.
CREATE TABLE comment_files (
    comment_id BIGINT UNSIGNED NOT NULL,
    file_id    BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (comment_id, file_id),
    KEY idx_comment_files_file (file_id),
    CONSTRAINT fk_comment_files_comment
        FOREIGN KEY (comment_id) REFERENCES ticket_comments (id)
            ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
