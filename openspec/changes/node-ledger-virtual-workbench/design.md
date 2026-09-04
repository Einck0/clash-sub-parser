## Context

See proposal.md for motivation and `specs/node-ledger-workbench/spec.md` for behavior. The worktree has a partially rewritten `NodeLedger.vue` that imports `getNodes`, `runProbeJob`, and `cancelProbeJob`, none of which exist in `frontend/src/api/index.ts`; `npm run build` consequently fails. `HEAD:frontend/src/views/NodeLedger.vue` contains the pre-rewrite business behavior, while the new `VirtualNodeTable`, metrics, search filter, and drawer are incomplete presentational shells.

The backend already exposes the complete source contracts: `GET /api/proxy-chains/meta/node-ledger`, `GET /api/probe/results`, `/api/probe/node`, `/api/probe/batch`, probe-result deletion, and proxy-chain CRUD. `NodeLedgerItem` carries the effective chain metadata and node probe requests accept the full node object. Probe batching is client-orchestrated, so stopping means preventing future chunks rather than claiming to cancel an unexposed server job.

## Goals / Non-Goals

**Goals:**

- Preserve every NodeLedger business operation from the last working `HEAD` implementation except SCRIPT-related functions, while moving filtering and action state out of the view shell.
- Make dense table rendering safe for at least 2,080 rows by virtualizing row presentation only, not inventory semantics.
- Reuse existing API methods, shared UI primitives, Tailwind tokens, and the already-installed virtualizer with no persistence or backend contract change.
- Make the page testable through typed, pure ledger-domain helpers plus focused component-contract tests.

**Non-Goals:**

- No database migration, probe-engine redesign, new server job endpoint, API renaming, or replacement of existing Workbench layout.
- No unbounded card-grid rendering; card mode remains paged or otherwise bounded.
- No new front-end framework, state library, or runtime dependency.

## Decisions

### Use the existing endpoint vocabulary rather than compatibility aliases

The page imports `getNodeLedger`, `getProbeResults`, `probeNode`, `probeNodesFull`, `clearProbeResults`, `getProxyChains`, `getNodeGroups`, `createProxyChain`, and `deleteProxyChain`. The implementation MUST remove imports of nonexistent methods rather than exporting aliases such as `getNodes`; aliases conceal the wrong resource semantics and would couple a legacy name to an effective-chain ledger.

The API boundary maps `NodeLedgerItem` to typed view-domain records. Probe records are indexed by both returned `name` and `node_key` when present, preserving legacy lookup behavior without mutating backend data.

Alternative considered: add `getNodes` and job APIs as thin exports. Rejected because no corresponding backend resource or cancellation protocol exists and it would leave the page’s omitted behavior unresolved.

### Separate ledger-domain state from page and presentational components

Extract typed domain helpers or a composable responsible for probe-record indexing, filtering, sorting, target selection, progress aggregation, and effective node-level chain operations. `NodeLedger.vue` owns route-level loading and composes the Workbench; `LedgerSearchFilter`, `LedgerMetricsBar`, `LedgerDrawer`, and `VirtualNodeTable` receive explicit props and emit typed events.

The component boundary is intentionally one-directional: state and action handlers flow down, semantic events flow up. Components MUST NOT issue duplicate inventory requests or maintain divergent copies of filter or selection state.

Alternative considered: retain all logic in a larger SFC. Rejected because the present rewrite already shows contract drift and makes unit-level filter and target semantics hard to verify.

### Virtualize rendering, never the data operation

The domain produces one sorted `filteredRows` list. `VirtualNodeTable` receives that complete list and owns only scroll range, height estimation, overscan, and row placement. It exposes enough slot context for stable keys and selection controls. The row selection set is keyed by node name and lives above the virtualizer. Batch targets are `selectedRows` when the selection set is nonempty, otherwise the complete `filteredRows` list; they are never calculated from `getVirtualItems()`.

The virtual table uses fixed estimated row height with modest overscan, an explicit scroll container, and no per-row data requests. Grid view retains a bounded page slice to avoid recreating the same large-DOM performance failure.

Alternative considered: server-side pagination. Rejected because the current ledger endpoint returns effective, merged inventory and full filtered-result probing must remain available without creating a new backend query and job contract.

### Preserve rich filtering and probe semantics in a normalized FilterState

The filter state covers text, subscription, protocol, detailed probe status, country, chain state, minimum speed, selected media capabilities, and sort mode. Options are derived from loaded inventory and probe data rather than a hard-coded protocol or country catalogue. Multiple media filters use AND semantics, matching the prior behavior. Metric clicks set or toggle the same state fields used by the full controls, including fast and chained states.

A probe batch retains the legacy chunked full-protocol request flow for progress feedback. A cancellation token or generation guard is checked before scheduling each next chunk; the UI truthfully says it stops later chunks and retains records returned by completed requests.

### Make details and dialer-chain controls complete rather than cosmetic

`LedgerDrawer` receives the selected row, probe record, available node and group dialer candidates, effective-chain source, and operation state. It renders diagnostic and raw-node detail, a single-probe action, and node-level chain create or clear actions. Replacement first removes existing node-level bindings for that node, creates the validated binding, then reloads canonical ledger data. Inherited binding state is shown but cannot be deleted from the node surface.

### Test contracts before implementation and verify production paths

The executor writes a failing test for each new domain behavior before code: complete filtered targeting, selected targeting across virtual positions, media-and-speed filtering, stop-before-next-chunk behavior, API method use, and node-level chain safety. Existing file-presence tests are retained but do not constitute acceptance evidence. Verification runs frontend tests, `vue-tsc --noEmit`, production Vite build, backend tests through the project interpreter, Compose image build, service health, and a public HTTPS response. Public deployment occurs only after local verification succeeds.

## Risks / Trade-offs

- [A user’s uncommitted NodeLedger edits can be overwritten] → Re-read every touched file immediately before edit, preserve behavior through current worktree plus `HEAD` comparison, and keep the diff minimal.
- [Duplicate node names can collide in selection or probe maps] → Retain existing name plus `node_key` probe lookup and surface a concrete finding rather than silently changing persistent identity; current API exposes name as the stable NodeLedger action target.
- [Probe cancellation cannot interrupt an in-flight HTTP request] → State the exact semantics in UI and tests; do not expose a false server-cancel guarantee.
- [Container build or public verification changes runtime state] → Run after all local gates, capture service/health output, and roll back with the prior image or `docker compose up -d --no-build app` only if the new container fails health checks.
- [Virtualized rows weaken keyboard or table semantics] → Preserve labelled controls, focus styles, header-to-row associations where feasible, and verify keyboard focus remains within the scrollable table.

## Migration Plan

1. Take the existing compose service and Git status as the rollback baseline; do not alter database volume or schema.
2. Add tests, implement against the live API exports, and run frontend and backend gates before creating an image.
3. Build the `app` image, start or recreate only the `app` service using the existing Compose definition, and wait for its configured health check.
4. Verify the local health endpoint and authenticated or public HTTPS response without logging secrets.
5. If health or response verification fails, restore the previous known-good `app` image or re-run the prior Compose service definition without the newly built image; preserve database volume and record the failing evidence.

## Open Questions

None. The existing server APIs and legacy page establish the required behavior; no unresolved decision changes the implementation plan.
