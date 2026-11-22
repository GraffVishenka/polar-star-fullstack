-- Откат 0005_tickets (локальная разработка; в production миграции только вперёд).
DROP TABLE IF EXISTS comment_files;
DROP TABLE IF EXISTS ticket_comments;
DROP TABLE IF EXISTS tickets;
