-- CSP 1.0 Group Position and Primary Subscription
-- Migration: 000023_group_position_and_primary_subscription.sql

ALTER TABLE node_groups ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0;
ALTER TABLE subscriptions ADD COLUMN userinfo TEXT NOT NULL DEFAULT '';
