-- OUTBOX EVENTS - для асинхронной обработки
CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(50) NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type VARCHAR(30) NOT NULL,
    event_data JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    processed_at TIMESTAMPTZ NULL,
    attempts SMALLINT DEFAULT 0 CHECK (attempts BETWEEN 0 AND 100),
    last_error TEXT,
    version INTEGER DEFAULT 0,    -- Для дедупликации
    UNIQUE(aggregate_type, aggregate_id, event_type, version)
) WITH (
      autovacuum_vacuum_scale_factor = 0.01,
      autovacuum_vacuum_threshold = 1000,
      fillfactor = 95,
      toast_tuple_target = 128  -- Оптимизация для JSONB
      );

-- Основной индекс для выборки необработанных событий
CREATE INDEX CONCURRENTLY idx_outbox_events_pending
    ON outbox_events(created_at, processed_at)
    INCLUDE (aggregate_type, event_type, aggregate_id, attempts)
    WHERE processed_at IS NULL
    AND created_at > NOW() - INTERVAL '30 days'  -- Ограничиваем диапазон
    AND attempts < 10;  -- Исключаем "мертвые" события

-- BRIN для исторических данных (если таблица > 1M записей)
CREATE INDEX CONCURRENTLY idx_outbox_events_created_brin
    ON outbox_events USING BRIN (created_at)
    WITH (pages_per_range = 32);