# Proposal: Node Probe Persistence, Periodic Probing, and Capability-Based Filtering

## 1. Problem Statement
1. **No Result Persistence**: Probe results were only cached in process memory and lost across requests/restarts.
2. **Missing Periodic Probe Scheduler**: Probe tests were purely manual without background polling intervals.
3. **Missing Capability Filtering in Subscriptions & Node Groups**: Users could not filter nodes in subscriptions or strategy groups based on test results (e.g. speed >= X Mbps, ChatGPT unlocked, Gemini unlocked).
4. **UI Response Format Disconnect**: Backend probe outputs (`latency_ms`, `ip`, `country`, `media`) and frontend property expectations (`handshake_ms`, `outbound_ip`, `streaming_unlock`) had mismatches, causing UI to show fail/timeout even when tests passed.

## 2. Proposed Changes
- **Persistent Storage**: Create `node_probe_results` table in SQLite/PostgreSQL to store node test records (`name`, `server`, `port`, `type`, `status`, `latency_ms`, `speed_mbps`, `ip`, `country`, `media`, `checked_at`).
- **Periodic Scheduler**: Add `probe_interval_minutes` to `probe_config` and register scheduled job in `app/services/scheduler.py` to auto-probe all enabled subscription nodes.
- **Filter Schema & Engine**:
  - Extend `Subscription` with `filter_min_speed_mbps: float | None` and `filter_media_unlock: list[str]`.
  - Extend `NodeGroup` with `filter_min_speed_mbps: float | None` and `filter_media_unlock: list[str]`.
  - Update `resolve_group_members` and `fetch_subscription_nodes` to apply capability filters against persistent probe results.
- **Frontend Enhancements**:
  - Add periodic probe interval setting in `/settings`.
  - Add Capability Filter section (min speed threshold + multi-select media unlock targets: YouTube, Netflix, ChatGPT, Gemini, Meta AI, etc.) in `SubscriptionDetail` / `Subscriptions.vue` and `NodeGroups.vue`.
  - Fix frontend mapping for probe status tags.

## 3. Backward Compatibility
- Default filter values (`None`, `[]`) preserve existing node selection behavior completely.
- Existing database instances automatically create `node_probe_results` on startup.
