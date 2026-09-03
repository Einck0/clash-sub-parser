## 1. Data Models and DB Persistence

- [x] 1.1 Create `NodeProbeResult` model and update `ProbeConfig`, `Subscription`, `NodeGroup` models.
- [x] 1.2 Implement database persistence in `probe_service.py` (`save_probe_result`, `get_node_probe_results`).

## 2. Capability Filtering Implementation

- [x] 2.1 Implement `filter_nodes_by_capabilities` utility in `app/utils/capability_filter.py`.
- [x] 2.2 Integrate capability filtering into `SubscriptionService` and `NodeGroupService` / `group_utils.py`.

## 3. Periodic Background Scheduler

- [x] 3.1 Implement `_poll_node_probes` in `app/services/scheduler.py` based on `probe_interval_minutes`.
- [x] 3.2 Add API endpoints to query latest probe results and trigger background probe tasks.

## 4. Frontend Integration

- [x] 4.1 Update `Settings.vue` with `probe_interval_minutes` input.
- [x] 4.2 Update `Subscriptions.vue` with speed threshold and media unlock filter options.
- [x] 4.3 Update `NodeGroups.vue` with speed threshold and media unlock filter options.
- [x] 4.4 Update `NodeLedger.vue` and `NodePreviewList.vue` with correct status tag bindings.

## 5. Testing and Validation

- [x] 5.1 Add unit tests for `NodeProbeResult` persistence and capability filtering.
- [x] 5.2 Run full test suite and verify end-to-end container rebuild.
