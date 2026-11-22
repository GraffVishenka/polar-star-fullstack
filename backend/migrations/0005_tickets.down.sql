-- Откат 0005_tickets (локальная разработка; в production миграции только вперёд).
-- comment_files принадлежит Б3, поэтому снимаем только FK на комментарий и
-- не удаляем таблицу files/comment_files.
ALTER TABLE comment_files DROP FOREIGN KEY fk_comment_files_comment;
DROP TABLE IF EXISTS ticket_comments;
DROP TABLE IF EXISTS tickets;
