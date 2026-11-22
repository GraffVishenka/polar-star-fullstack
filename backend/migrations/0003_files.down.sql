-- Откат 0003_files (Б3). Порядок обратен созданию: сначала связь
-- «комментарий ↔ файл», затем сами файлы.
DROP TABLE IF EXISTS comment_files;
DROP TABLE IF EXISTS files;
