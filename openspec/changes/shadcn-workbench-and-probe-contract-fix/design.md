## Context

See `proposal.md` and the three delta specifications. The verified current read path is `Subscription.raw_nodes → proxy_chain_service.list_final_nodes() → list_node_ledger() → GET /api/proxy-chains/meta/node-ledger → NodeLedger.vue`; probe persistence uses `name|type|server:port` in `probe/service.py`, while `GET /api/probe/results` pages by stored `node_key`. The ledger schema and `LedgerNodeItem` already reserve `node_key`, but the backend does not supply it. Summary entries are keyed by `node_key` but omit identity. The Node Ledger has both the intended `getProbeForNode()` helper and older direct `getProbe(item.name)` template calls, so desktop paths can lose a real record after reload while mobile paths work differently.

The already-approved bounded-summary work intentionally excludes name and node key from summary values. This change supersedes that projection boundary only to restore the minimal identity pair; it does not relax the privacy boundary or reinstate full-record/name-key duplication. It does not modify stored probe schema.

`node_probe_results` is unique by node_key and currently accumulates until the global delete endpoint is used. `fetch_subscription_nodes()` replaces a subscription's `raw_nodes` only after a successful remote fetch; manual recomputation and selection-affecting updates likewise establish a new final-node set. The current final-node service deduplicates by display name, so identity work must also explicitly settle duplicate-name behavior rather than treating display text as a key.

The frontend is Vue 3 + Vite + Tailwind CSS v4 with a `@/*` TypeScript alias, Lucide already installed, local Button/AppModal/BaseDrawer primitives, and source-visible DOM tests. It lacks `components.json`, a shared class helper, shadcn-vue/Reka UI dependencies, and component family ownership. shadcn-vue's current v3 guidance uses Reka UI v2, the successor to Radix Vue; using the former Radix Vue package would be deliberately outdated.

## Goals / Non-Goals

**Goals:**

- Make a current ledger row and a persisted probe record agree on one immutable exact identity at every read and update boundary.
- Make untested a true absence state, not a fallback for identity, pagination, or unknown-status failures.
- Bound historical probe data by the current enabled ledger set without retaining a duplicate compatibility store.
- Establish a locally inspectable shadcn-vue + Reka UI foundation, then migrate shared and Node Ledger controls under frozen component and visual contracts.
- Maximize parallel implementation after the contracts below are frozen: backend identity/convergence, frontend foundation, and fixture/test scaffolding have isolated ownership; Node Ledger integration waits only for the published API/schema contract, not for unrelated UI migrations.

**Non-Goals:**

- No full clean-slate replacement, database migration, new runtime service, deployment, config change, secret movement, or traffic cutover.
- No automatic preservation of historical probes outside the current enabled node set; stale results are intentionally discarded on the specified successful lifecycle changes.
- No frontend reimplementation of node-key formatting, fuzzy lookup, shadow ledger, dual write, API-version compatibility endpoint, or duplicated name-key summary map.
- No change to probe transport, egress isolation, capability qualification, subscriptions, generator behavior, route authorization, or existing response-detail sanitization.
- No blanket visual rewrite in one hot file and no unaudited replacement of all raw controls. Each primitive migration must retain its existing props/emits and behavioral test coverage.

## Decisions

### D1. One shared backend identity constructor is the sole key authority

Add a pure, dependency-free module owned by backend domain services, proposed path `backend/app/services/node_identity.py`, exporting `canonical_node_key(node: Mapping[str, Any]) -> str`. It normalizes the existing persisted format exactly: trimmed `name`, trimmed `type`, trimmed `server`, and stringified port, joined as `name|type|server:port`. `probe/service.py`, proxy-chain ledger projection, scheduler inventory and convergence code import it. The frontend receives the server-produced key and never derives one.

`NodeLedgerItem` gains required `node_key: str`. A summary value repeats `node_key` and `name`, while the map remains keyed only by the exact key. Frontend normalization verifies `entry.node_key === mapKey`; invalid/mismatched records are rejected from the in-memory map and surfaced as a load-warning telemetry-free UI error, not silently matched by name.

Alternative: expose only keys as map keys and build keys in TypeScript. Rejected: two string algorithms already drifted; user evidence proves this is the root cause. Alternative: use display names as a global primary key. Rejected: they change with subscription labels and are non-unique.

### D2. Treat duplicate display names as a controlled ambiguity, not an alias

The current ledger must carry each unique canonical identity; it may no longer remove a node solely because its display name was seen. Its ledger row, Vue `:key`, selection set, incremental probe state, and probe-in-progress marker use `node_key`; name remains display text and is only an API field for current name-based chain endpoints.

The existing proxy-chain model and mutation endpoints are name-addressed. The Node Ledger shall mark chain mutations unavailable for duplicate display names and present a clear “名称重复，暂不能安全编辑跳板” state, rather than risk mutating a sibling. Read-only probe, inspect, filtering and selection remain key-based. Extending chain-binding identity is explicitly out of scope and requires a separate OpenSpec because it changes persistent binding semantics.

Alternative: preserve name-based selection under the new records. Rejected: a UI selection or chain action could affect a sibling node, violating integrity. Alternative: silently choose the first same-name row. Rejected: not deterministic from the operator's perspective.

### D3. Normalized status is a closed presentation algebra

Add a pure frontend helper, proposed `normalizeProbeStatus`, that accepts string input and returns one of `ok`, `fail`, `timeout`, `skipped`, `unknown`, or `untested`. `untested` is constructed only when lookup returns no record. Any present unsupported/empty status normalizes to `unknown`. `getProbeForNode` takes only a node-key primary lookup; a narrowly scoped helper can apply a live-response name fallback after counting current rows by name and only where count equals one.

Every Node Ledger renderer calls a single `getProbePresentation(node, probes)` adapter. That adapter owns label, status icon/tone, latency eligibility, and status filtering semantics. Delete the local `getProbe(name)` function and ban direct bracket lookup from Ledger templates via a static source test. `unknown` and `skipped` have separate badge labels and counters; neither can enter the untested query/filter branch.

Alternative: add `v-else-if` branches around direct name lookups. Rejected: it treats a systemic key mismatch as presentation logic and misses filters/counters/cards. Alternative: index every result under both keys. Rejected: it recreates ambiguity, memory duplication, and the stale name fallback that caused the defect.

### D4. Convergence uses current-set membership at both mutation and read boundaries

Create a single service operation, proposed `converge_probe_results_to_current_nodes(db)`, which:

1. derives the current enabled final-node key set from raw node data using the shared identity constructor
2. deletes rows whose key is absent from that set
3. removes matching process-cache entries
4. returns only aggregate counts (`retained`, `deleted`) for test/logging, never node secrets

Call it only after a successful transaction publishes a changed `raw_nodes` or after deletion commits. For remote fetch, it runs only after the new `raw_nodes` commit; error handling must retain the old set and skip convergence. For a manual-node recomputation or selection-changing update, it runs only when the final raw-node content changes. The current-set calculator must avoid proxy-chain operations and must be callable within the subscription lifecycle without circular import.

`get_paged_db_probe_summary()` additionally intersects records with the current key set before ordering/cursoring. This read fence covers an unavoidable race in which a probe started before refresh commits later: the late row can exist transiently but cannot appear in summary pages and will be deleted at the next convergence point. Probe execution itself retains its existing writer contract; no distributed lock, retry queue, or parallel table is introduced.

The source refresh pipeline uses normalized inventory separately and is not a valid authority for the legacy Node Ledger; it must not become an alternate probe ledger. The legacy current set remains enabled `Subscription.raw_nodes` until a separately planned domain migration changes that ownership.

Alternative: delete by node name during refresh. Rejected: renamed traffic labels and duplicate names would delete the wrong row. Alternative: retain all rows but filter client-side. Rejected: cursor starvation and raw database growth persist. Alternative: delete every probe record before refresh. Rejected: a failed refresh would destroy active evidence.

### D5. Adopt shadcn-vue current distribution as local source, not an opaque framework replacement

Frontend foundation owner first adds a `components.json` compatible with Vite, Tailwind v4, TypeScript alias `@`, and source locations. It adds a local `src/lib/utils.ts` `cn` helper and pins the current shadcn-vue-compatible Reka UI v2 dependency plus any direct shadcn-vue-required utility dependencies to exact versions resolved by the package manager; it must not guess version strings or use a remote CDN. It generates/adopts source into project-owned `src/components/ui/` using the established lowercase directory convention for multi-part primitives and keeps public imports stable through a deliberate barrel/compatibility mapping only during the migration.

The component family contract is:

| Primitive | Owner | Required behavior | Existing boundary retained |
| --- | --- | --- | --- |
| Button, IconButton | shared UI | variants, loading, disabled, icon slot, focus state | current `variant`, `size`, `loading`, `icon`, `ariaLabel` consumers |
| Badge, status presentation | shared UI + ledger domain | textual state plus semantic tone | no status-color-only meaning |
| Input, Select, Checkbox | shared UI | label/description/error/focus and 44px mobile targets | v-model/form events |
| Tooltip | shared UI | accessible delayed descriptive hint, keyboard safe | icon-button labels remain explicit |
| Dialog, AlertDialog | shared overlay | focus trap, scroll lock, Escape, restore, backdrop | AppModal/ConfirmDialog caller semantics |
| Sheet/Drawer | shared overlay | 320px left, 480px right desktop, mobile bottom sheet | BaseDrawer drag and close behavior |
| DropdownMenu, Tabs | shared UI | keyboard navigation and controlled state | current action/menu/tab events |

Do not migrate all pages in one card. Adopt foundation and overlay adapter first; migrate Node Ledger next; then split the remaining consumers by non-overlapping file ownership. Delete an adapter only after all of its repository consumers move and browser tests prove unchanged behavior.

Alternative: manually restyle existing components and call them shadcn. Rejected: it does not meet the requested shadcn-vue component foundation or create reliable primitive ownership. Alternative: install the historical Radix Vue package. Rejected: current upstream shadcn-vue has moved to Reka UI v2. Alternative: use a remote UI runtime. Rejected: not inspectable, reproducible, or acceptable for an industrial control plane.

### D6. Preserve the existing design governance while correcting its contradictory history

Existing unarchived visual proposals disagree: one freezes Slate/teal surfaces while another freezes the Void palette. The current user instruction is authoritative: dark default uses Void `#08090a`, panel `#0d0f12`, card `#121417`, semantic status tones, and Linear/Raycast-like density without copied branding. `theme.css` becomes the only authored semantic color token source; `style.css` may consume semantic variables but may not supply fallback raw palettes for migrated primitives. A codemod is not authorized; executor inventories and migrates only allocated selectors.

Desktop ledger density remains 36px. At 375px actionable controls have a minimum 44px target, and all declared viewports have no horizontal document overflow. CSS animations remain targeted to transform/opacity/colors; `prefers-reduced-motion` removes nonessential animation. Overlay blur stays limited to one shared overlay backdrop with an opaque fallback.

## Risks / Trade-offs

- [Canonical identity length can exceed legacy `String(255)` for unusually long node names] → unit-test boundary length before release; reject/diagnose oversize source nodes through existing validation/quarantine paths rather than truncate a key.
- [Changing final-node dedupe exposes duplicate names to a name-addressed chain surface] → use key-based read/probe selection and disable ambiguous chain mutation; do not silently mutate one node.
- [Convergence performs a potentially large SQLite delete after source changes] → perform set-derived bulk delete in the same service boundary, test at thousands of synthetic rows, and never build an unbounded `NOT IN` parameter list; use a temporary/query strategy compatible with supported SQLite/PostgreSQL dialects.
- [Late writer race] → summary read fence prevents exposure; next lifecycle convergence deletes stale result. No claim of serializable global locking is made.
- [Package/API differences across shadcn-vue releases] → implementation resolves and pins current compatible package versions from the authoritative package manager/docs before editing; dependency installation is isolated from UI migration.
- [Overlay conversion risks focus or drag regressions] → adapter preserves current props/emits and `useModalA11y` behavior until Playwright confirms equivalent lifecycle; no all-at-once replacement.
- [Concurrent frontend changes create hot files] → declare `frontend/src/assets/theme.css`, `frontend/src/style.css`, `frontend/src/components/ui/*`, and `frontend/src/views/NodeLedger.vue` hotspots; planner must serialize or assign mutually exclusive ownership.

## Migration Plan

1. Freeze schemas, named exports, status matrix, fixture contract, file ownership, and hot-file policy in `tasks.md`; do not merge implementation before the contract tests exist.
2. Backend identity/convergence owner adds pure tests, summary/ledger contract tests, lifecycle/concurrency tests, and performance-safe pruning. No database migration, reset, or live data access is allowed; all fixtures use temporary test databases.
3. In parallel, frontend foundation owner resolves/pins dependencies, adds the manifest/token/class-helper/primitive source and adapter tests without editing Node Ledger business state. In parallel, frontend test owner creates synthetic API fixtures and status/identity browser cases, but does not write production components.
4. After backend and foundation interface gates are green, Node Ledger owner migrates key-based state and all renderers, then consumes shared components. It removes direct-name lookup paths and adds duplicate-name chain-action guard.
5. The general workbench migration splits remaining screens into independent non-overlapping batches after shared primitive contracts stabilize. Every batch requires typecheck, targeted component tests, and static token/primitive audit.
6. Run focused backend pytest, frontend unit/domain tests, typecheck, visual audit, production build, fixture-backed Playwright at 375/768/1024/1440, then independent Reviewer and Critic gates. No live probe, subscription fetch, source refresh, database cleanup, service restart, or deployment is part of verification.
7. Roll back by reverting this change's backend/frontend source and dependency lock diff. Convergence deletion is intentionally irreversible for orphaned historical results; before enabling it against a live database, the release owner must take an existing operational backup and explicitly approve the destructive retention change. If no approval is granted, do not deploy this change.

## Open Questions

None. The only operational decision deferred to the release gate is user approval of live orphaned-probe deletion after a verified backup; it does not change the architecture or task graph.

## Strategy Revision 2 — downstream canonical-key closure

### Trigger and evidence

The first implementation completed the ledger-facing `node_key` path, but the verified source graph still leaves a second, name-addressed probe-result interface active outside the Ledger:

- `get_all_db_probe_results()` emits both `results_map[item.node_key]` and `results_map[item.name]` in `backend/app/services/probe/service.py`.
- `generate_subscription_payload()` and `_collect_all_nodes()` read that name alias in `backend/app/services/generate_service.py`.
- `collect_all_subscription_nodes()` does likewise in `backend/app/services/subscription_service.py`; group resolution reads `probe_map.get(item)` in `backend/app/utils/group_utils.py`.
- `filter_nodes_by_capabilities()` rebuilds `name|type|server:port` locally, then falls back to name in `backend/app/utils/capability_filter.py`.
- On the client, `normalizeNodeLedgerMap()` indexes array input under `item.name`, `getProbeForNode()` falls back to a name-indexed map when no key is supplied, and `LedgerNodeItem.node_key` remains optional in `frontend/src/views/nodeLedgerDomain.ts`.

Those paths contradict D1/D2/D3: a duplicate display name can borrow another node's speed/media qualification or probe state. The present focused backend suite passes because its fixtures do not exercise a duplicate-name capability-filter/generation case. This is deterministic static proof, not a speculative concern.

### Frozen decisions

1. `canonical_node_key()` remains the sole identity constructor. All Python consumers that receive a node dictionary must obtain its probe result only through a shared key-based lookup helper; they may not format the key locally or probe a display-name alias.
2. `get_all_db_probe_results()` becomes a canonical-key-only internal projection. Its map has exactly one entry per `NodeProbeResult.node_key`; it must not duplicate entries by `name`.
3. Introduce one pure backend lookup boundary, proposed as `get_probe_result_for_node(probe_map, node)`. It imports `canonical_node_key`, returns `probe_map.get(canonical_node_key(node))`, and has no name fallback. Capability filtering, generation, subscriptions, and any new caller use this helper or a typed equivalent in the same owned module.
4. Group resolution is intentionally name-based for membership presentation, so it cannot safely make per-node capability decisions when duplicate names exist. Extend its contract to receive keyed node metadata when a group has speed/media constraints, or pre-filter the node dictionaries before reducing them to names. Do not retain the current `probe_map.get(name)` shortcut.
5. The browser-side summary map is canonical-key-only for every accepted transport form. Remove array name indexing. A node row without `node_key` is an invalid ledger payload and is not eligible for probe association, selection, or chain mutation. The tightly scoped name fallback allowed by D3 applies only to an immediately returned single-probe response after uniqueness is established from the current row set; it is not a general map lookup.
6. `LedgerNodeItem.node_key` is required in the frontend domain type. This aligns the existing backend `NodeLedgerItem` requirement and makes missing-key use a type error rather than a silent untested state.

### Atomic corrective proposals

| Proposal | Assignee | Depends on | Exclusive write scope | Acceptance harness |
| --- | --- | --- | --- | --- |
| CSP-13 key-closed backend consumers | executor | CSP-01 | `backend/app/services/probe/service.py`, `backend/app/utils/capability_filter.py`, `backend/app/services/generate_service.py`, `backend/app/services/subscription_service.py`, `backend/app/services/node_group_service.py`, `backend/app/utils/group_utils.py`, focused tests only | Two same-name, distinct-key records with opposite speed/media results must yield distinct subscription export, aggregate export, and group-preview outcomes. Assert no map contains a display-name alias and no source reimplements the canonical format. Run focused pytest plus `ruff check` on touched paths. |
| CSP-14 strict ledger input boundary | executor | CSP-04 | `frontend/src/views/nodeLedgerDomain.ts`, `frontend/src/views/NodeLedger.vue`, `frontend/src/components/ledger/**`, Node Ledger tests only | Array/envelope/map normalization admits only matching `node_key` pairs. Missing-key rows do not associate a name-key probe. A uniquely identified immediate probe response may update only its current key. Run domain/contract tests, typecheck, and fixture browser contract. |
| CSP-15 integration and adversarial review | reviewcommon | CSP-13, CSP-14 | read-only | Re-run backend/frontend harnesses and add a negative duplicate-name generation/capability case. Check summary/detail privacy, late-write fence, absence-only `untested`, and that no name-key alias or local canonical formatter remains. |

### Verification and release boundary

Required green evidence after CSP-13/14 is: backend focused tests for identity, persistence/API, generation/capability filtering, group preview, subscription service, routes, and proxy chains; frontend Node Ledger domain/contract tests, `npm run typecheck`, `npm run build`, and strict visual audit; then `openspec validate shadcn-workbench-and-probe-contract-fix --strict`.

Fixture Chromium checks at 375/768/1024/1440 remain mandatory but are presently environment-blocked: the test runner reports no Playwright Chromium executable. Provisioning the pinned browser is an executor/release-environment action and must be evidenced before the visual gate is marked green. It is not a code defect and does not authorize a live probe, subscription refresh, database cleanup, service restart, or deployment.

Rollback remains a source/lockfile revert. The existing live orphan-probe deletion warning is unchanged: no deployment or cleanup against live data occurs without a verified backup and explicit human authorization.
