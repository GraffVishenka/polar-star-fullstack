-- 0009_trash: корзина и восстановление (Б10, TRASH-1..TRASH-5).
--
-- `deleted_at` в tickets/kb_nodes/equipment уже есть (0004..0006) — здесь
-- добавляется только автор удаления и индекс под список корзины. FK на users
-- ON DELETE SET NULL: пользователи не удаляются, но связь не должна мешать.
-- Физическое удаление (TRASH-3) выполняется блоком Б10 и эти колонки очищает.

ALTER TABLE tickets
    ADD COLUMN deleted_by BIGINT UNSIGNED NULL,
    ADD CONSTRAINT fk_tickets_deleted_by
        FOREIGN KEY (deleted_by) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE,
    ADD KEY idx_tickets_deleted_by (deleted_at, deleted_by);

ALTER TABLE kb_nodes
    ADD COLUMN deleted_by BIGINT UNSIGNED NULL,
    ADD CONSTRAINT fk_kb_deleted_by
        FOREIGN KEY (deleted_by) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE,
    ADD KEY idx_kb_deleted_by (deleted_at, deleted_by);

ALTER TABLE equipment
    ADD COLUMN deleted_by BIGINT UNSIGNED NULL,
    ADD CONSTRAINT fk_equipment_deleted_by
        FOREIGN KEY (deleted_by) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE,
    ADD KEY idx_equipment_deleted_by (deleted_at, deleted_by);
