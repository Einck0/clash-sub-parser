-- CSP 1.0 Subscription Advanced Configuration
-- Migration: 000003_subscription_config.sql

ALTER TABLE subscriptions ADD COLUMN config_json TEXT NOT NULL DEFAULT '{}';
