-- 0006_kb: база знаний (блок Б6, PLAN §3.5 KB-1..KB-8).
--
-- kb_nodes — дерево статей с неограниченной вложенностью и ручной сортировкой.
--   parent_id — самоссылка на родителя; ON DELETE RESTRICT (физическое удаление
--     узлов не выполняется: удаление уходит в корзину через deleted_at, а FK
--     защищает от потери поддерева при ручных операциях с БД).
--   sort_order — шаг 10 внутри одного родителя (KB-6: переиндексация при move).
--   is_hidden_for_anon — только у корневых разделов, у потомков наследуется
--     (KB-1, KB-7).
--   deleted_at — soft delete в корзину (KB-3); фильтр "живых" узлов всегда
--     deleted_at IS NULL.
--   created_by/updated_by — автор создания/последней правки, NULL после
--     физического удаления пользователя (users не удаляются, но FK безопасен).
CREATE TABLE kb_nodes
(
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    title              VARCHAR(200)    NOT NULL,
    -- content_html до 100 000 символов (KB-1): MEDIUMTEXT, т.к. TEXT (64 КБ)
    -- не вмещает многобайтовый UTF-8 такого объёма.
    content_html       MEDIUMTEXT      NOT NULL,
    parent_id          BIGINT UNSIGNED NULL,
    sort_order         INT             NOT NULL DEFAULT 10,
    is_hidden_for_anon TINYINT(1)      NOT NULL DEFAULT 0,
    created_by         BIGINT UNSIGNED NULL,
    updated_by         BIGINT UNSIGNED NULL,
    created_at         DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at         DATETIME        NULL,
    PRIMARY KEY (id),
    -- Дерево отдаётся одним запросом с сортировкой sort_order, title внутри
    -- родителя (KB-8); индекс покрывает выборку детей.
    KEY idx_kb_parent_sort (parent_id, sort_order, title),
    -- Корзина и фильтр «живых» узлов (KB-3, TRASH-1).
    KEY idx_kb_deleted (deleted_at),
    CONSTRAINT fk_kb_parent
        FOREIGN KEY (parent_id) REFERENCES kb_nodes (id)
            ON DELETE RESTRICT ON UPDATE CASCADE,
    CONSTRAINT fk_kb_created_by
        FOREIGN KEY (created_by) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE,
    CONSTRAINT fk_kb_updated_by
        FOREIGN KEY (updated_by) REFERENCES users (id)
            ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
