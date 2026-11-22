-- 0003_files: файлы и медиа (блок Б3).
--
-- files — метаданные загруженного файла. Сам файл лежит на локальном диске
-- (UPLOAD_DIR), в БД хранится путь относительно корня загрузок
-- (YYYY/MM/{uuid}.{ext}), MIME, определённый по сигнатуре (magic bytes), а не
-- по расширению, размер и sha256 для контроля целостности.
--
-- linked_at IS NULL — файл ещё не привязан к комментарию/статье и является
-- кандидатом на фоновую очистку (Б3: удаление через 24 часа).
-- uploader_id NULL + is_anonymous=1 — загрузка анонимом по токену заявки
-- (TICKET-13, TICKET-14).
CREATE TABLE files
(
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    original_name VARCHAR(255)    NOT NULL,
    stored_path   VARCHAR(500)    NOT NULL,
    mime          VARCHAR(100)    NOT NULL,
    size_bytes    BIGINT UNSIGNED NOT NULL,
    sha256        CHAR(64)        NOT NULL,
    scope         ENUM ('ticket', 'kb') NOT NULL,
    uploader_id   BIGINT UNSIGNED NULL,
    is_anonymous  TINYINT(1)      NOT NULL DEFAULT 0,
    linked_at     DATETIME        NULL,
    created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    -- Один и тот же путь на диске не может принадлежать двум записям.
    UNIQUE KEY uq_files_stored_path (stored_path),
    -- Индекс фоновой очистки orphan-файлов: linked_at IS NULL AND created_at < ?.
    KEY idx_files_orphans (linked_at, created_at),
    -- Поиск дублей по содержимому (диагностика, дедупликация в будущем).
    KEY idx_files_sha256 (sha256),
    KEY idx_files_scope_created (scope, created_at),
    CONSTRAINT fk_files_uploader
        FOREIGN KEY (uploader_id) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- comment_files — связь «комментарий заявки ↔ файл» (TICKET-14: до 5 вложений
-- на комментарий). FK на comments появится в Б5 вместе с таблицей comments;
-- сейчас её нет, поэтому ограничение целостности по comment_id не задаётся.
-- FK на files — ON DELETE CASCADE: удаление файла убирает и связи.
CREATE TABLE comment_files
(
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    comment_id BIGINT UNSIGNED NOT NULL,
    file_id    BIGINT UNSIGNED NOT NULL,
    created_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_comment_files_pair (comment_id, file_id),
    -- Выборка вложений комментария (список комментариев заявки).
    KEY idx_comment_files_comment (comment_id),
    -- Проверка «файл привязан к комментарию» и каскад при удалении файла.
    KEY idx_comment_files_file (file_id),
    CONSTRAINT fk_comment_files_file
        FOREIGN KEY (file_id) REFERENCES files (id)
            ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
