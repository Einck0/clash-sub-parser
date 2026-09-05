## Context

See proposal.md and delta specs. This is an evolution of the existing node-ledger and probe pipeline, not a replacement. The current UI holds `FilterState` in `NodeLedger.vue` and applies it through the pure `filterAndSortNodes()` helper; `LedgerSearchFilter.vue` renders every media platform and country as a persistent Chip row. That is the exact visual-space hotspot.

The backend flow is `POST /probe/node|batch` → frozen `ResolvedProbeConfig` → `probe_single_node()` → isolated `spawn_node_runner()` → transport → geo consensus → `check_media_unlock()` → persistence. Current platform checks run concurrently, but `check_media_unlock()` creates one `httpx.AsyncClient` per platform; `ResolvedProbeConfig.media_timeout_s` is resolved from settings but `probe_single_node()` passes `service_timeout_s` to the media dispatcher instead. The backend already enforces loopback proxy pinning and `trust_env=False`, and its result/persistence contract is additive JSON. These boundaries are retained.

Prior art is narrowly adopted. Subs-Check uses a Fast.com Netflix endpoint for a fast region or 403-ban conclusion, then falls back to title probing if it cannot conclude; it also reuses pooled buffers for response parsing.[1][2] Its OpenAI and YouTube implementations demonstrate multiple public signals and response-body reuse, but CSP will not copy its provider-specific browser-like header choreography, cookies, opaque tokens, or source-level buffer pool because Python `httpx` already owns connection pooling and CSP's existing evidence contract forbids raw bodies.[3][4]

## Goals / Non-Goals

**Goals:**

- Compress the filter information architecture without removing any filtering power or changing existing no-selection behavior.
- Define one canonical facet-selection schema and one pure matching algorithm for UI and regression tests.
- Reduce media-stage connection setup and Netflix's common-case network round trips while keeping every request on the candidate node egress.
- Make media-stage time budgeting explicit, observable, bounded and independent from the general service timeout.

**Non-Goals:**

- No new server-side filtering API, URL/query persistence, database migration, schema table, service process, dependency or parallel implementation path.
- No alteration of runner lifecycle, transport liveness authority, identity consensus, batch semaphore cap, node-wide timeout semantics, speed-test semantics, result taxonomy or public route shape.
- No generic "increase concurrency" change. The configured node concurrency remains capped at 20 and media parallelism remains bounded by the existing selected-platform set.
- No login, browser automation, private credential, copied request token, raw body persistence or secret-bearing diagnostic output.

## Decisions

### D1. Freeze a canonical `FacetFilterState` at the view-domain boundary

The existing `FilterState` is evolved in `nodeLedgerDomain.ts` to this frozen shape:

```ts
interface FacetFilterState {
  keyword: string
  subscription: string
  protocols: string[]
  statuses: string[]
  countries: string[]
  chain: 'all' | 'chained' | 'plain'
  minSpeed: number
  mediaPlatforms: string[]
  sortBy: string
}
```

`protocol`, `status`, and `country` are replaced rather than duplicated. There is no shadow state, compatibility alias, URL copy, or second ledger filter engine. All collections are normalized to lower-case platform/protocol keys and upper-case ISO country codes, deduplicated and stable in insertion order before entering the pure filter function.

Matching is frozen as:

- empty collection: no criterion;
- `protocols`, `statuses`, `countries`: any selected value matches the corresponding node property (OR);
- `mediaPlatforms`: every selected platform must satisfy the existing verified-full predicate (AND);
- all populated dimensions plus subscription, chain, minimum speed and keyword: AND.

This exactly preserves existing media AND behavior while granting requested multi-selection to country/protocol/health. Existing metrics and reset actions produce the new canonical shape. The historical simple media values remain accepted only through the existing canonical full-unlock predicate.

Alternative rejected: add arrays alongside old singular fields and merge them in components. That produces two sources of truth and lets empty legacy values silently override multi-select state.

### D2. Use one accessible `FacetMultiSelect` UI primitive owned by the ledger filter

A local presentational component under `components/ledger/` is permitted only if it receives immutable facet props and emits the complete selected key set. It is not allowed to fetch inventory, inspect probe records or own another `FilterState` copy. It must expose a button with `aria-expanded`, labelled popover/listbox or checkbox group, roving/ordinary tab focus, Escape close, and visible focus styling. A click outside closes the currently open facet; opening another switches the active facet. Mobile uses the same markup and token system, not a separate picker.

Each collapsed facet shows label and selected count. Active selections render in a bounded summary strip as removable tokens; this is the only remaining multi-value chip surface. Candidate options are not mounted until the facet is opened. The summary must use `min-w-0`, text truncation where needed, and no document horizontal overflow at 375px.

Alternative rejected: native `<select multiple>` would satisfy multi-select mechanically but neither exposes counts nor offers a compact touch and keyboard workflow. A general-purpose global component is also rejected: the behavior is ledger-specific and a local component avoids an unrequested design-system abstraction.

### D3. The media dispatcher owns one session and one media deadline per node

`probe_single_node()` continues to decide whether transport succeeded and whether media is enabled. After a successful handshake it calls the dispatcher with an explicit `media_timeout_s`. `check_media_unlock()` creates exactly one `httpx.AsyncClient` with:

```python
proxy=proxy_url
trust_env=False
follow_redirects=True
headers={"User-Agent": DEFAULT_UA}
```

It calculates `deadline_monotonic = monotonic() + media_timeout_s` once, creates concurrent evaluator coroutines against that client, and each evaluator derives the current remaining time before every request. A platform gets a structured timeout without a new request if its remaining budget is exhausted. `asyncio.gather(..., return_exceptions=True)` retains sibling outcomes. The outer `wait_for` is the stage backstop only; its expiration is converted into deterministic timeout results for not-yet-returned platform keys rather than losing the whole media map.

The existing catalogue request helper gains an optional caller-provided deadline and never expands a request beyond the lower of its own deadline and the media-stage deadline. Its at-most-one transient retry rule remains unchanged. Media stage must use `media_timeout_s`; geo and transport remain on `service_timeout_s`; node-wide `node_timeout_s` stays the outer cap. Therefore actual media time is `min(remaining node-wide budget, media-stage deadline)`, never a new ability to overrun the node budget.

Alternative rejected: sequential reuse of one client would lower connect cost but turn `N` platform waits into accumulated latency. A global cross-node client is rejected because it risks connection and proxy identity reuse across different isolated node runners.

### D4. Netflix fast path is an evidence-preserving short circuit, never a weaker classifier

`eval_netflix()` receives the shared client and shared deadline. It first requests the public Fast.com Netflix endpoint with bounded response parsing. It may return early only for:

- HTTP 403: existing `ip_blocked` / `blocked` contract;
- HTTP 200 with a syntactically valid two-letter country in the first target location: existing `verified` / `full` result with an additional safe signal such as `fast_com_region`.

All other outcomes — transport failure, timeout, non-200 other than 403, invalid JSON, empty target or invalid country — are inconclusive for the fast path only and must execute the existing title evidence fallback if budget remains. The fallback retains current title classification and its own error taxonomy. If no budget remains, it returns `timeout` or `inconclusive` based on the observed fast-path failure; it must not claim full access. Fast path evidence only retains allowlisted status, final host, elapsed time and symbolic signal; it never stores endpoint token/query string, body, headers or target IP.

We deliberately do not copy the opaque Fast.com token from upstream. The executor must validate that the unauthenticated, non-secret endpoint contract is viable with a fixture and a controlled actual-node smoke test before enabling it. If it is not viable, the feature is disabled behind the existing fallback and a plan revision is required; no secret substitute is allowed.

### D5. Configuration and API compatibility remain additive and no migration is planned

`media_timeout_s` already exists in ORM, schema and settings API. The only contract correction is to pass its resolved value into the dispatcher. Its validation stays `1..30` seconds. No route request or response adds mandatory fields. No database migration or rewrite occurs; changed media observations still merge into `NodeProbeResult.media` per platform.

The plan explicitly forbids using the Fast.com result to change node `country`, identity confidence, speed, cache key or geo consensus. It is a Netflix capability region only. No new server endpoint is needed because settings and existing probe endpoints already carry all required inputs.

### D6. File ownership and conflict policy

Executor A owns only:

- `frontend/src/components/ledger/LedgerSearchFilter.vue`
- a new local ledger facet component if necessary
- `frontend/src/views/NodeLedger.vue`
- `frontend/src/views/nodeLedgerDomain.ts`
- directly related frontend tests

Executor B owns only:

- `backend/app/services/probe/providers.py`
- `backend/app/services/probe/catalogue.py`
- `backend/app/services/probe/service.py`
- `backend/app/schemas/probe.py` only if a typing-only contract adjustment is demonstrated necessary
- directly related backend tests and fixtures

`probe_settings_service.py`, ORM, migrations, router, other UI primitives, package manifests, compose and deployment files are read-only unless a StrategyConsultation revises this plan. The executor must re-read every owned file immediately before editing. A collision in an owned hot file is reported as `hotspot:` on its board card; no side-channel merge or unrelated refactor is allowed. Executor B begins only after Executor A's domain contract revision is complete, avoiding duplicate decisions about names and filter semantics. Reviewers own no business code.

## Risks / Trade-offs

- [Fast.com endpoint becomes unavailable or changes schema] → fixture contract, deadline-bounded fallback, provider result `inconclusive` or `timeout`; never silently produce an unlock claim.
- [A public Fast.com query parameter is copied as opaque compatibility cargo] → do not copy it. Only a non-secret documented or runtime-validated form is allowed; otherwise retain title fallback and request plan revision.
- [Shared session weakens node egress isolation] → per-node lifetime only; test captured `proxy` and `trust_env=False` for every constructed session; no client escapes runner context.
- [Media-stage timeout plus node timeout conflicts] → compute all requests from monotonic remaining budget; node-wide timeout is always the final cap.
- [Facet multi-selection gives surprising results] → frozen OR within discrete facets, AND across facets, explicit active-filter summary and pure table-driven tests.
- [Dense filter UI breaks narrow layouts/accessibility] → browser tests at 375/768/1024/1440, keyboard and ARIA assertions, no horizontal overflow; Reviewer and Critic independently rerun them.
- [Two executor changes collide in a hot file] → sequential dependency plus fixed ownership. The frontend domain contract lands first; backend has no permission to edit it.

## Migration Plan

1. Deploy no schema migration and make no change to historical rows.
2. Executor A lands canonical frontend state and tests; Executor B then lands probe-session change and backend tests in its exclusive files.
3. Run focused frontend and backend suites, typecheck, production build and actual browser geometry harness against the combined tree.
4. Reviewer independently reruns declared checks and audits egress pinning, timeout budget, fallback and legacy filter behavior. Critic separately attacks no-subscription, all-timeout, cancellation, mobile focus and notification-free paths.
5. Planner may create release work only after both verdicts pass. Existing production configuration remains compatible because `media_timeout_s` already exists and request shapes are unchanged.
6. Roll back by deploying the prior application image. No database downgrade, data deletion or cache key alteration is required. New in-memory session behavior disappears with the previous image; persisted JSON continues to be read by old code.

## Open Questions

None that alter task scope. The Fast.com endpoint viability is an implementation acceptance gate: failure selects the already specified safe fallback and blocks provider fast-path enablement rather than authorizing secret-bearing workaround.

## Sources

[1] https://github.com/beck-8/subs-check
[2] https://raw.githubusercontent.com/beck-8/subs-check/master/check/platform/netflix.go
[3] https://raw.githubusercontent.com/beck-8/subs-check/master/check/platform/openai.go
[4] https://raw.githubusercontent.com/beck-8/subs-check/master/check/platform/youtube.go
