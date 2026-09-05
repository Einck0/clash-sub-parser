## 1. Baseline and data-contract migration

- [ ] 1.1 Record the existing probe settings and scheduler behavior in focused regression fixtures before changing semantics; verify the fixtures distinguish legacy `probe_interval_minutes`, node budget, service timeout, and cron configuration.
- [ ] 1.2 Create an additive Alembic revision for `probe_concurrency`, `probe_service_timeout_ms`, `probe_cron_enabled`, and `probe_cron_interval_minutes`; verify upgrade on an existing SQLite schema preserves `node_probe_results`, retains an explicitly saved old concurrency, and gives unset new fields defaults 10, 2000, true, and 60.
- [ ] 1.3 Extend the ORM model, settings schemas, serializer, config-transfer column allowlist, import/export, and reset path; verify API read/PATCH validation and an export-import round trip cover all four fields without touching auth credentials.

## 2. Probe execution orchestration

- [ ] 2.1 Add an immutable resolved probe-configuration snapshot at manual and scheduled batch start, enforcing a manual override only when it is lower than the persisted concurrency; verify a mid-batch Settings update affects only a later batch.
- [ ] 2.2 Refactor batch workers to enforce the resolved maximum node concurrency and remove the frontend’s hard-coded concurrency override; verify a controlled awaitable test observes no more than 10 active node workers by default and respects a lower override.
- [ ] 2.3 Add independent service-level deadline handling for isolated runner startup, transport, geo, every selected platform, and controlled speed tests; verify timeout classifications preserve completed sibling results, do not use host egress, and release semaphore capacity and runner resources.
- [ ] 2.4 Retain `probe_timeout_ms` only as an optional node-wide budget with cancellation-safe cleanup; verify an expired node budget yields node `timeout` without converting cancellation into generic failure or leaking ports/subprocesses.
- [ ] 2.5 Update the APScheduler tick to use `probe_cron_enabled` and `probe_cron_interval_minutes`, freeze a scheduled run’s configuration, prevent scheduled overlap, and handle clock rollback; verify enable/disable/interval PATCH behavior takes effect at the next tick without restart.

## 3. Workbench shell and shared overlay responsiveness

- [x] 3.1 Update App, WorkbenchHeader, WorkbenchSidebar, and global theme/layout styles for 44px mobile navigation targets, `min-w-0` layout boundaries, and zero document-level horizontal overflow; verify 375/768/1024/1440px browser checks and desktop sidebar behavior.
- [x] 3.2 Extend the shared BaseDrawer responsive contract so right-side secondary panels use safe-area Bottom Sheet behavior below 640px while left navigation stays an edge drawer; preserve and verify close button, focus trap, Escape, background scroll lock, focus restore, and any drag-to-dismiss threshold/control exclusions.
- [x] 3.3 Normalize Subscriptions, NodeGroups, ProxyChains, Rules, and Settings to mobile single-column/wrapping toolbars and reachable action/form controls; migrate Rules from its legacy isolated theme to Workbench semantic tokens and shared UI primitives; verify all existing commands remain present at 375px.

## 4. NodeLedger and Settings mobile integration

- [ ] 4.1 Introduce a compact NodeLedger mobile renderer below 640px that reuses existing filters, selection state, probe records, detail drawer, and single-node/batch actions; verify query and selected-node state remain consistent across the 639px/768px renderer boundary.
- [ ] 4.2 Update Settings quality-control controls for concurrency, per-service timeout, scheduled-enable, and scheduled interval with validation-compatible bounds and unambiguous units; verify PATCH payload, returned state, help text, disabled master-probe behavior, and 375px reachability.
- [ ] 4.3 Add frontend component/unit coverage for narrow renderers and Settings state plus browser regression coverage for mobile overlay lifecycle, no horizontal overflow, NodeLedger selection/probe controls, and all Settings controls at 375/768/1024/1440px.

## 5. Integrated verification and release gate

- [ ] 5.1 Run relevant backend unit/route/migration tests, frontend unit tests, `npm run typecheck`, `npm run build`, and `npm run audit:visual:strict`; verify actual command output is retained for review and no pre-existing tests were silently skipped.
- [ ] 5.2 Have an independent reviewer compare the complete diff and command output against the four delta specs and design, including timeout semantics, data preservation, accessibility, mobile layout, and absolute-path/security rules; verify verdict is APPROVED before release.
- [ ] 5.3 After APPROVED, rebuild and start the Compose application with `docker compose up -d --build`, wait for health, and smoke-test `/health`, authenticated settings round trip, scheduled-probe configuration persistence, and responsive UI; verify failure rollback retains the `backend-data` volume and returns the previous service image/config.
