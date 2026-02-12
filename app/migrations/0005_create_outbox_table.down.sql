DROP TRIGGER IF EXISTS trigger_auto_delete_processed ON outbox_events;

-- Удаление функции
DROP FUNCTION IF EXISTS auto_delete_processed_outbox();

-- Удаление индекса
DROP INDEX IF EXISTS idx_outbox_unprocessed;

-- Удаление таблицы
DROP TABLE IF EXISTS outbox_events;