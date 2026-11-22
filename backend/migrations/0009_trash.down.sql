-- Откат 0009_trash: сначала FK (пока индекс ещё нужен), затем индекс и колонка.
ALTER TABLE equipment
    DROP FOREIGN KEY fk_equipment_deleted_by,
    DROP KEY idx_equipment_deleted_by,
    DROP COLUMN deleted_by;

ALTER TABLE kb_nodes
    DROP FOREIGN KEY fk_kb_deleted_by,
    DROP KEY idx_kb_deleted_by,
    DROP COLUMN deleted_by;

ALTER TABLE tickets
    DROP FOREIGN KEY fk_tickets_deleted_by,
    DROP KEY idx_tickets_deleted_by,
    DROP COLUMN deleted_by;
