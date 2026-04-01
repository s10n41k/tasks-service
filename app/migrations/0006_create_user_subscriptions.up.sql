CREATE TABLE user_subscriptions (
    user_id          UUID PRIMARY KEY,
    has_subscription BOOLEAN   DEFAULT FALSE,
    expires_at       TIMESTAMP,
    telegram_chat_id BIGINT,
    updated_at       TIMESTAMP DEFAULT NOW()
);
