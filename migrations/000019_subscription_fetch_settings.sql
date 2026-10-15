-- Persist the subscription-fetch route independently from authentication settings.
ALTER TABLE settings ADD COLUMN subscription_fetch_mode TEXT NOT NULL DEFAULT 'environment'
    CHECK (subscription_fetch_mode IN ('environment', 'direct', 'proxy'));
ALTER TABLE settings ADD COLUMN subscription_fetch_proxy_url TEXT NOT NULL DEFAULT '';
