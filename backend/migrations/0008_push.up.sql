-- 0008_push: Web Push подписки и outbox уведомлений (NOTIF-1..NOTIF-7).
--
-- Подписка принадлежит пользователю; endpoint уникален (одно устройство = одна
-- подписка). is_active=0 выставляется при 404/410 от push-сервиса или удалении.
-- Outbox хранит по одной строке на получателя: payload уже содержит title/body/
-- url/tag/icon, повторные попытки идут с экспоненциальной задержкой.
-- idempotency_key NULL — обычные события; для напоминаний заполняется
-- event_type:ticket_id:user_id:local_date, чтобы повторный запуск джобы не
-- дублировал доставку (NOTIF-7).

CREATE TABLE push_subscriptions
(
    id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id         BIGINT UNSIGNED NOT NULL,
    endpoint        VARCHAR(500)    NOT NULL,
    p256dh          VARCHAR(255)    NOT NULL,
    auth            VARCHAR(255)    NOT NULL,
    user_agent      VARCHAR(512)    NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_success_at DATETIME        NULL,
    is_active       TINYINT(1)      NOT NULL DEFAULT 1,
    PRIMARY KEY (id),
    UNIQUE KEY uq_push_subscriptions_endpoint (endpoint),
    KEY idx_push_subscriptions_user_active (user_id, is_active),
    CONSTRAINT fk_push_subscriptions_user
        FOREIGN KEY (user_id) REFERENCES users (id)
            ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE notification_outbox
(
    id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    event_type       VARCHAR(32)     NOT NULL,
    user_id          BIGINT UNSIGNED NOT NULL,
    payload          JSON            NOT NULL,
    status           ENUM ('pending', 'sent', 'failed') NOT NULL DEFAULT 'pending',
    attempts         INT UNSIGNED    NOT NULL DEFAULT 0,
    next_attempt_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    idempotency_key  VARCHAR(191)    NULL,
    created_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    sent_at          DATETIME        NULL,
    last_error       TEXT            NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_notification_outbox_idempotency (idempotency_key),
    KEY idx_notification_outbox_due (status, next_attempt_at),
    KEY idx_notification_outbox_user (user_id, created_at),
    CONSTRAINT fk_notification_outbox_user
        FOREIGN KEY (user_id) REFERENCES users (id)
            ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
