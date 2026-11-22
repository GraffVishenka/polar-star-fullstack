-- Откат 0008_push (локальная разработка; в production миграции только вперёд).
DROP TABLE IF EXISTS notification_outbox;
DROP TABLE IF EXISTS push_subscriptions;
