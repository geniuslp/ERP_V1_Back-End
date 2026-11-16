-- Web Push subscriptions (phone / desktop notifications when the ERP page is not open).
-- One user can have many rows (one per browser / device). The API sends a push for each new notification row
-- (see internal/notify/push.go). A row is deleted when the push service answers 404/410 (subscription expired).
-- Idempotent. Dev: run through cmd/migrate. UAT/prod: run this file manually.

CREATE TABLE IF NOT EXISTS public.push_subscription (
    id          bigserial PRIMARY KEY,
    user_id     bigint       NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    endpoint    text         NOT NULL,
    p256dh      text         NOT NULL,
    auth        text         NOT NULL,
    user_agent  varchar(300),
    created_at  timestamp    NOT NULL DEFAULT now(),
    CONSTRAINT uq_push_subscription_endpoint UNIQUE (endpoint)
);
CREATE INDEX IF NOT EXISTS idx_push_subscription_user ON public.push_subscription (user_id);
