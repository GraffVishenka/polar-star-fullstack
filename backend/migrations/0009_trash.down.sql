-- Откат 0009_trash: порядок обратен созданию.
ALTER TABLE equipment
    DROP KEY idx_equipment_deleted_by,
    DROP FOREIGN KEY fk_equipment_deleted_by,
    DROP COLUMN deleted_by;

ALTER TABLE kb_nodes
    DROP KEY idx_kb_deleted_by,
    DROP FOREIGN KEY fk_kb_deleted_by,
    DROP COLUMN deleted_by;

ALTER TABLE tickets
    DROP KEY idx_tickets_deleted_by,
    DROP FOREIGN KEY fk_tickets_deleted_by,
    DROP COLUMN deleted_by;
