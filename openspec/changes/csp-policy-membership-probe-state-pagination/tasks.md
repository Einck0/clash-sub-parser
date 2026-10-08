## 1. Backend Package: Full Inventory Resolution (Go/Migrations)

- [x] 1.1 In `internal/domain/ports.go` and `internal/repository/sqlite/nodes.go`, implement `ListAll(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, error)` with unpaginated query execution, preserving subscription scope, active-only constraints, notice exclusions, and IP risk decision admissions. Verify with sqlite node repository tests.
- [x] 1.2 In `internal/application/publication/service.go:812` and `internal/application/policy/service.go:932`, replace `nodeRepo.List` with `nodeRepo.ListAll` in resolver snapshot provider and export compiler. Verify publication preflight and policy service tests pass.
- [x] 1.3 Create unit and integration fixtures with 250 active nodes where target nodes (e.g. Taiwan at 70-72, Canada at 78-80, and extra targets at > 100) are resolved. Verify that both export artifact compilation and policy graph validation resolve all target groups without empty group errors.

## 2. Backend Package: Probe Health Conservation & Server Health Filter (Go/Migrations)

- [x] 2.1 In `internal/domain/probe.go` and `internal/domain/probe_health.go`, add `UndeterminedCount` to `ProbePoolStatus`, and establish strict mutual exclusion conservation: `total_count == healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count` and `available_count == healthy_count + degraded_count`. Verify domain health tests pass.
- [x] 2.2 In `internal/application/probe/service.go`, update pool status calculation to separate nodes with 0 observations (`UntestedCount`) from nodes with stale, revision-mismatched, or non-baseline-only observations (`UndeterminedCount`). Verify probe service tests pass.
- [x] 2.3 In `internal/repository/sqlite/nodes.go`, `internal/repository/sqlite/node_predicates.go`, and `internal/transport/http/nodes.go`, add full-scope server-side `health_status` filtering to `buildNodeFilterPredicates` and query parameter parsing, returning accurate paginated subsets. Verify HTTP nodes test suite passes.

## 3. Frontend Package: Membership Semantics & Policy Pagination (Web)

- [x] 3.1 In `web/src/features/policy/PolicyEditorSheet.vue` and `web/src/features/policy/GroupCard.vue`, clarify explicit edges mode vs dynamic pool filter mode with visual badges and guidance, without altering the underlying algebra or deleting existing edges/filters. Verify policy component tests pass.
- [x] 3.2 In `web/src/features/policy/usePolicy.ts` and `web/src/features/policy/PolicyView.vue`, implement server-side pagination (default 50, max 100) and keyword search for policy groups, with pagination navigation controls. Verify `policy.test.ts` passes.
- [x] 3.3 In `web/src/features/policy/PolicyEditorSheet.vue`, implement asynchronous debounced search and paginated loading for candidate node selection, preventing silent truncation beyond 100 nodes. Verify editor sheet tests pass.
- [x] 3.4 In `web/src/features/policy/PolicyView.vue`, separate the full topology graph rendering from the paginated group cards list, ensuring topology validation reflects all groups regardless of the current card page. Verify `PolicyView.test.ts` passes.

## 4. Frontend Package: Probe Health Authority & Layout Resilience (Web)

- [x] 4.1 In `web/src/features/nodes/nodeView.ts`, remove the dangerous fallback `if (values.includes('available')) return 'healthy'`, enforce baseline-authoritative health evaluation, and introduce the `undetermined` category. Verify `nodeView.test.ts` passes.
- [x] 4.2 In `web/src/features/probes/useProbes.ts` and `web/src/features/probes/ProbesView.vue`, remove the `while (page <= 20)` waterfall loop, consume server pool status metrics, update KPI cards, and add the undetermined category filter. Verify `probes.test.ts` passes.
- [x] 4.3 In `web/src/App.vue`, update the desktop `<aside>` element with `sticky top-0 h-screen shrink-0 overflow-y-auto` so it remains anchored during page scrolling. Verify layout in desktop view.
- [x] 4.4 In `web/src/App.vue` and `web/src/features/policy/PolicyEditorSheet.vue`, implement body scroll locking (`overflow: hidden`) when mobile modal sheets or drawers are open. Verify mobile interaction in tests.
- [x] 4.5 In `web/e2e/multidevice.test.ts`, run multi-viewport tests across Desktop (1280x800), Landscape (667x375), Mobile (375x667), and Mobile Large (392x872), ensuring zero horizontal overflow, visible navigation, and accessible targets.

## 5. Verification, Gate Enforcement, Review, and Closure Pipeline

- [x] 5.1 Run full repository deterministic compile and test gates: `go test -v ./internal/...` (exit code 0) and `cd web && npm run type-check && npm test` (exit code 0).
- [x] 5.2 Execute independent read-only reviewer verification against the final Git diff and OpenSpec change contract.
- [x] 5.3 Deploy isolated preview build, run health check, and execute Critic visual inspection across the four standard viewports.
- [ ] 5.4 Synchronize web assets with `COPY` flag preservation, execute database backup, perform authorized `git_commit`, and verify zero-downtime service launch.
