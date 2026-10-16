-- CSP 1.0 Subscription Name Prefix
-- Migration: 000021_subscription_prefix.sql

ALTER TABLE subscriptions ADD COLUMN prefix TEXT NOT NULL DEFAULT '';
