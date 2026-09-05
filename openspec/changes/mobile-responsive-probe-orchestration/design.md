## Context

See proposal.md and the delta specs. The repository already contains a Vue 3 Workbench, Tailwind design tokens, a shared `BaseDrawer`, desktop virtual NodeLedger, mobile cards for several domain views, a FastAPI probe API, a singleton SQLite `probe_config`, an APScheduler tick, and a sing-box runner with a 100-port loopback pool. These pieces are incomplete rather than absent.

The current probe implementation has three semantic defects that must be corrected without losing real outbound verification: the persisted default concurrency is 5 rather than 10; `probe_timeout_ms` is passed as one shared timeout to several services; and the scheduler treats `probe_interval_minutes` as the scheduled-probe switch and period. Frontend manual batches additionally hard-code concurrency 5. The legacy SQLite schema is protected by Alembic readiness checks and configuration export/import relies on a table-column allowlist.

## Goals / Non-Goals

**Goals:**

- Deliver one responsive, accessible Workbench behavior across 375, 768, 1024, and 1440px without creating a second mobile product.
- Preserve all existing non-SCRIPT management and quality-check operations while making mobile actions reachable and horizontally safe.
- Separate probe configuration domains: node workflow budget, service deadline, node concurrency, and scheduled-run cadence.
- Guarantee that every network-facing service and isolated runner startup gets an independent service-level deadline, never turns a timed-out service into a host-egress result, and cannot starve other queued nodes.
- Persist and hot-apply scheduled-probe configuration with an additive SQLite migration and regression tests.

**Non-Goals:**

- Do not replace the existing Vue, FastAPI, SQLAlchemy, APScheduler, httpx, sing-box, or Tailwind stack.
- Do not change protocol parser behavior, node identity keys, historical `node_probe_results` rows, provider endpoint selection, Quick Export, or any feature other than SCRIPT.
- Do not promise browser-side cancellation of a server batch that has already been accepted; the current user interface only stops subsequent client chunks.
- Do not add a generic mobile component library or a new persistent bottom navigation.
- Do not modify production data or run deployment in implementation cards.

## Decisions

### D1. Responsive behavior is renderer adaptation, not a mobile fork

Keep the existing route tree, stores, API client, and command semantics. Apply the following viewport ownership rules:

| Width | Shell/navigation | Data views | Overlays |
| --- | --- | --- | --- |
| `< 640px` | 44px mobile menu trigger; edge navigation drawer | NodeLedger compact list; other domains use single-column list/cards and wrapping toolbars | Secondary panels are bottom sheets; navigation remains left drawer |
| `640px–767px` | mobile navigation retained | wider compact lists/cards; no desktop multi-column assumptions | bottom sheets retained for secondary panels |
| `>= 768px` | persistent sidebar | existing desktop/table and grid behavior | existing desktop drawer direction and dimensions |

The application root, shell flex children, and route containers use `min-w-0`; document-level `overflow-x` is clipped only at shell/body scope, not as a substitute for fixing individual wide controls. Tables that must retain tabular semantics are locally wrapped; this project’s domain views already use mobile list renderers, so the implementation should prefer them over horizontal table scrolling.

The current hamburger is 32px and violates touch-target requirements. It will become at least 44px while the 48px header height stays unchanged by allowing an internally centered button with negative-safe visual density only if the hit target itself remains 44px. Header quick actions must either preserve a 44px target or collapse textual decoration at narrow widths; visual compactness cannot shrink hit testing.

Alternatives considered:

- A fixed bottom navigation would create an incomplete duplicate information architecture and hide less common CSP routes.
- CSS-only squeezing of existing dense NodeLedger rows retains inaccessible actions; a dedicated mobile renderer is required.

### D2. One shared overlay primitive owns mobile sheet behavior

`BaseDrawer` remains the single entry point for side drawers and bottom sheets. It will accept a presentation contract rather than force every caller to know breakpoints:

- `placement="left"` remains the navigation edge drawer at every width.
- `placement="right"` becomes a bottom sheet below 640px and its existing right drawer at wider widths.
- Its existing modal accessibility helper remains authoritative for focus capture, Escape, restore focus, scroll lock, dialog labelling, and close control.
- Bottom sheets have a visible drag affordance, explicit close control, safe-area footer/body padding, and a max usable height. Drag dismissal is optional but, if implemented, only begins from the sheet handle/header non-control zone. Close requires downward displacement of at least 96 CSS pixels or downward velocity of at least 0.5 px/ms. Pointer interaction on inputs, textareas, selects, buttons, links, or scrollable content must retain native behavior.

This preserves the current proven a11y mechanism instead of adding an unverified dialog dependency. The native `<dialog>` alternative was rejected because the current Teleport, focus helper, and route-level usage would need a broad rework with no user-visible gain.

### D3. NodeLedger gets an explicit compact mobile list

At `<640px`, NodeLedger does not render `VirtualNodeTable`’s desktop multi-column row. It renders a keyed compact list from `filteredRows` while continuing to use the existing `selectedNodeNames`, probe map, filters, selection helpers, `inspectNode`, and `handleProbeSingle` functions. Each item has:

- a 44px checkbox/select target;
- a two-line identity block (name with truncation and protocol badge);
- a status/latency line and optional speed summary;
- detail and probe actions with at least 44px hit targets;
- any endpoint, subscription, chain, media, and mutation details inside the existing detail sheet.

The current desktop virtual window remains unchanged. Mobile does not require virtualizing a typical narrow viewport in this increment; it must be paged or bounded before rendering if measurements demonstrate a long-list regression. The existing 48-card grid pagination is not used to silently drop the table/list selection semantics. The implementation must add a focused component test for shared selected state and a browser check using a realistic large fixture.

### D4. Quality-control configuration has four independent meanings

The persisted singleton and API DTO receive additive fields:

| Field | Default | Valid range | Meaning |
| --- | --- | --- | --- |
| `probe_concurrency` | 10 | 1–20 | Maximum simultaneous node workflows in a batch |
| `probe_service_timeout_ms` | 2000 | 500–30000 | Deadline for one isolated service operation |
| `probe_cron_enabled` | true | boolean | Enables creation of new scheduled batches |
| `probe_cron_interval_minutes` | 60 | 1–1440 | Minimum elapsed time between scheduled batch starts |

`probe_timeout_ms` remains in the API for compatibility but is explicitly treated as an optional per-node total budget, not a service or batch timeout. A value of 0 in new writes means no node-wide deadline; pre-existing positive values are preserved during migration and apply only to newly started runs that read them. `probe_interval_minutes` remains readable/exportable as a legacy manual field and is not consulted by the new scheduler. Its confusing Settings control is removed or labelled legacy only; new Settings shows the dedicated cron switch and interval.

To avoid accidental resource overload, manual API `concurrency` is `effective_concurrency = min(client_override or saved_config.probe_concurrency, saved_config.probe_concurrency)` after validating it is at least 1. A manual caller can lower concurrency but cannot exceed the persisted administrator limit. The frontend omits the override unless a future UI explicitly exposes a lower manual limit.

The default is changed from 5 to 10 only for new singleton rows and the additive migration’s fill value. Existing explicit persisted concurrency remains a user preference and is not overwritten. This avoids silently increasing load on an established deployment.

### D5. Probe batches freeze their configuration, and services own their deadlines

A `ResolvedProbeConfig` value object is created once at the entry of each manual or scheduled batch. It contains booleans, platforms, speed test limits, concurrency, `service_timeout_s`, and optional `node_timeout_s`. The scheduler and API explicitly pass it to `probe_batch_nodes`; worker coroutines never reread mutable config mid-batch.

One node worker holds one `asyncio.Semaphore` slot from runner acquisition through final cleanup. Inside the worker:

1. Runner startup uses the configured service deadline, including `wait_for_port_ready`.
2. Transport is one service operation with one deadline across its fallback endpoint strategy.
3. Geo identity is one service operation with an overall deadline; internal provider fallback cannot cumulatively exceed it.
4. Each selected media or AI provider is a separate service operation with its own deadline. Provider result shape includes `status: "timeout"` plus a sanitized reason when it expires.
5. Speed testing is one service operation with its own deadline, capped by the lower of a configured legacy speed-test limit and service deadline. Partial bytes may be reported, but the outcome explicitly indicates timeout.

The node-wide total budget, when positive, wraps the complete node workflow outside these services. Its expiry yields node `status: "timeout"`, relies on cancellation-safe `asynccontextmanager` cleanup in `spawn_node_runner`, and cannot change the result shape of completed independent services. A cancelled node must not become `fail` merely because `CancelledError` was stringified.

The transport result remains node liveness authority: no successful transport means subsequent geo/media/speed work is skipped. Conversely, a timed-out media/AI/geo/speed service does not turn an otherwise reachable node into `fail`. All HTTP clients retain `proxy=<node loopback URL>` and `trust_env=False`; there is no direct retry through a process environment proxy.

Alternatives considered:

- Wrapping the entire server list in `asyncio.wait_for(2)` violates the stated service-level semantics.
- Applying a separate 2-second timeout to every fallback URL would allow transport to consume 8 seconds; the outer service deadline bounds the entire transport service.
- Letting each media provider share a single `httpx.AsyncClient` is acceptable only if every provider coroutine is individually wrapped and closing the client occurs after all children settle; otherwise timeout cancellation may leak or serialize providers.

### D6. Scheduler reads persisted state at tick boundaries and prevents overlap

Keep the existing APScheduler one-minute `node-probe-scheduler` job because it gives simple dynamic reconfiguration without requiring job rescheduling. At each tick it opens a short-lived session, loads `ProbeConfig`, and checks `probe_cron_enabled`, interval elapsed from `_last_scheduled_probe_started_at`, and a single scheduler-local async lock. Once eligible, it records the scheduled start timestamp and captures the resolved config before loading active nodes and running the batch.

`probe_cron_enabled=false` prevents new scheduled batches but does not cancel a running one. A configuration PATCH commits before returning; the next tick observes the new row. Clock movement is handled by resetting the elapsed baseline if wall-clock time is earlier than the last recorded start, preventing a negative interval from suppressing work indefinitely. Startup has no immediate catch-up run: it waits for the next tick and the regular elapsed rule, avoiding surprise large probes after a restart.

The current `probe_enabled` remains a master quality-check feature gate for manual and scheduled probes. Therefore Settings labels distinguish “Enable node outbound quality checks” from “Run scheduled quality checks.” `probe_cron_enabled` cannot override a disabled master probe gate.

### D7. Data, configuration transfer, and tests are an atomic backend slice

A single Alembic revision adds the four additive fields to `probe_config` with server defaults and backfills existing singleton rows without touching `node_probe_results`. The corresponding ORM, Pydantic read/update schemas, model conversion, Alembic bridge allowed-column map, config-transfer export/import path, reset defaults, and API tests are changed together. API consumers always receive all new fields; old clients may omit them on PATCH.

The Settings form uses one reactive configuration object including all new fields, sends them in its existing atomic PATCH call, and updates local state from the response. It displays a scheduled-probe toggle, interval input with minutes hint, concurrency, and service timeout with explicit statement: “applies to each handshake, egress, provider, or speed service; not the whole batch.” Number inputs reflect server validation ranges. A failed save does not claim the scheduler changed.

### D8. Verification and deployment gate

Backend unit tests use controllable awaitables and a fake clock to prove semaphore maximum, provider-specific timeout classification, resource cleanup, frozen configuration, and scheduler hot-update behavior without real outbound traffic or sing-box. Route tests prove defaults, validation, and config transfer. Existing real-proxy integration behavior is not replaced by mocks.

Frontend tests add component/unit coverage for narrow renderers and Settings payload/state. Playwright or an equivalent browser harness exercises 375, 768, 1024, and 1440px, no document horizontal scroll, mobile drawer close/focus lifecycle, NodeLedger select/probe controls, and Settings cron field usability. Existing frontend typecheck, production build, visual audit, and backend tests must all pass.

No implementation task rebuilds or starts production services. A separate release card, unlocked only after independent review APPROVED, will run `docker compose up -d --build`, wait for health, validate `/health` and authenticated API behavior, then test a non-destructive Settings round trip and responsiveness against the deployed UI. If migration or health fails, it stops the rollout and restores the prior image/container while retaining the volume; additive schema fields are harmless to the older application because it ignores unknown columns.

## Risks / Trade-offs

- [Default concurrency 10 can exhaust loopback ports or host CPU with expensive providers] → Hard cap at 20, shared 100-port pool, saved user overrides preserved, and stress test tracks active runners.
- [Service timeouts still leak subprocesses or file descriptors under cancellation] → Runner cleanup is covered by cancellation tests that assert port return and process termination before semaphore release.
- [Concurrent media results race while sharing one client] → Each provider returns an independent tuple/result; collection is deterministic and does not mutate a shared result dictionary from child tasks.
- [Mobile CSS fixes hide rather than solve an overflow] → Browser checks inspect document scroll width and test action visibility at four fixed viewports.
- [Existing positive `probe_timeout_ms` makes a whole node time out before all 2-second services] → Preserve it only as explicit legacy node budget, label it clearly, and default new writes to unlimited/0; no service deadline is inferred from it.
- [A scheduler tick overlaps after slow work] → Scheduler-local lock and APScheduler `max_instances=1` prevent duplicate scheduled batches; manual batches remain independent by design.
- [Configuration import misses new fields] → Extend model/allowlist contract and add export-import round-trip regression before release.

## Migration Plan

1. Executor creates an additive Alembic revision and updates the model/API/config-transfer contract. Run schema readiness and migration tests against a SQLite copy.
2. Executor implements probe orchestration behind the new configuration snapshot, then runs unit/route regressions without real external traffic.
3. Frontend executors update the shared shell/overlay primitive, domain mobile renderers, NodeLedger mobile list, and Settings UI; run type, build, visual, and responsive checks.
4. reviewcommon independently verifies every changed file against the OpenSpec requirements, runs the declared test commands, and either approves or returns precise fixes.
5. Only after APPROVED does the release executor build and start the Compose service, verify health and UI/API smoke paths, and report actual results.

Rollback before deployment is a normal code rollback. During deployment, keep the existing `backend-data` volume: the migration only adds nullable/defaulted columns and does not mutate historical probe rows. If the new image fails readiness or smoke validation, return to the prior image/container configuration; the prior code remains compatible with added SQLite columns. Do not downgrade or delete user data as part of rollback.
