## Context

See `proposal.md` and the two added capability specifications. The verified execution path is periodic tick → `scheduler._poll_node_probes()` → `probe_batch_nodes()` → one `probe_single_node()` per candidate. The scheduler uses an in-process `asyncio.Lock`, APScheduler `max_instances=1`, and a wall-clock interval gate (`backend/app/services/scheduler.py:167-258`). The batch hydrates once, creates an `asyncio.Semaphore` capped at 20, then schedules one coroutine per hydrated node (`backend/app/services/probe/service.py:941-1009`).

For a non-direct node, `probe_single_node()` enters `spawn_node_runner()` before the existing proxy transport check (`backend/app/services/probe/service.py:839-862`). The runner creates a temporary configuration, starts `sing-box run -c`, waits for a loopback listener, and terminates/kills the process during context exit (`backend/app/services/probe/runner.py:154-206`). A separate direct TCP service already has server/port validation, a 2,000 ms default, a 20-way bounded semaphore, and per-target outcomes (`backend/app/services/tcp_probe_service.py:14-147`); it is not currently in the deep-probe call path.

The upstream source review of sinspired/subs-check-pro establishes a bounded staged Go pipeline and configurable stage concurrency, but it does not provide independently reproduced resource or throughput measurements. No CSP controlled benchmark, production RSS/CPU trace, active container inspection, actual candidate failure-rate distribution, or verified process count was available to this planning run. Numeric claims from the IntentPackage are therefore treated as unverified hypotheses, not design inputs.

## Goals / Non-Goals

**Goals:**

- Insert a single bounded preflight admission stage before every expensive non-direct deep probe.
- Preserve the existing isolated proxy execution as the sole authority for proxy transport, identity, media, AI, and speed evidence.
- Replace current eager coroutine creation with a bounded worker-pool architecture whose queue size, active work, cancellation, and completion are observable.
- Define an empirical decision gate that can reject or retain the Python architecture before any Go proposal exists.
- Keep scheduler behavior responsive and make overload a terminal, explainable outcome rather than scheduler starvation.

**Non-Goals:**

- No full Go rewrite, no Go sidecar, no embedded sing-box Go library, no dependency addition, and no running service change in this change.
- No long-lived multi-outbound sing-box pool. The existing runtime has one isolated runner per deep-probe candidate; a pooled multi-outbound process changes process-isolation, config lifecycle, protocol support, and cleanup assumptions and needs separate source and failure-mode evidence.
- No TLS preflight. Direct TCP is deliberately the narrow first stage because protocol-specific TLS assumptions would reject valid non-TLS or custom-protocol candidates.
- No change to `node_key`, Node Ledger, probe persistence ownership, proxy-chain semantics, capability policy, media catalog, or public result privacy contract except additive aggregate telemetry if a route exposes it.
- No live database cleanup, production scheduler tuning, benchmark against real subscription credentials, or deploy/cutover.

## Decisions

### D1. Two stage funnel with explicitly non-equivalent evidence

Stage 1 accepts a minimal normalized target `{node_key, server, port, type}` and returns one of `preflight_ok`, `preflight_timeout`, `preflight_error`, or `preflight_invalid`, with elapsed time. It uses a direct `asyncio.open_connection` only. It creates no proxy URL and cannot set a persisted capability status.

Stage 2 receives only candidates with `preflight_ok`, then invokes the current isolated runner and existing deep probe flow. Its result is the only candidate result eligible for `NodeProbeResult` persistence and capability filtering. A stage-1 success followed by stage-2 failure remains a stage-2 failure.

Existing TCP utility is the reuse point because it already validates address/port, bounds timeouts and concurrency, and uses monotonic elapsed time. It requires an internal API reshaping: batch use must stream/admit bounded targets rather than return a fully materialized list before deep work begins.

Alternative: process every node and record TCP preflight only as another diagnostic. Rejected: it does not reduce runner allocation. Alternative: infer liveness from the existing proxy transport check. Rejected: that check is downstream of runner creation. Alternative: make preflight a proxy request. Rejected: it reintroduces the runtime cost the funnel exists to avoid.

### D2. Bounded workers, not `gather` over the inventory

The funnel shall use a bounded producer/worker model. Input enumeration must not create one task per candidate. Stage 1 has `preflight_workers`; its success output feeds a bounded `deep_queue`; Stage 2 has `deep_workers`, capped by runner port availability and configuration. Queue capacity is a finite multiple of worker count and must be recorded as telemetry. An executor must preserve input identity and emit one terminal outcome per accepted request item.

The current hard cap of 20 deep probes (`service.py:976`) becomes an explicit deep-runner capacity, not a justification for unbounded waiting tasks. The actual initial worker and queue values are NOT frozen: they require the benchmark evidence card below. During implementation, test fixtures inject capacities; production defaults cannot be selected or changed without evidence and release authorization.

Alternative: retain `asyncio.gather` and only add a preflight semaphore. Rejected: task allocation and result retention still scale with entire batch, and queue/backpressure is unobservable. Alternative: use an unbounded `asyncio.Queue`. Rejected: it transfers memory pressure from processes to Python objects and hides overload.

### D3. Scheduler lock scope is local and its limitation is declared

One process retains three local safeguards: APScheduler `max_instances=1`, `_probe_lock`, and elapsed-time gate. The periodic owner records a run token and starts the bounded funnel after inventory snapshot acquisition. It never holds an ORM transaction across queued deep work; persistence is batch-owned at completion or in documented bounded chunks. A failed/cancelled run records an aggregate failure and releases all local worker/runner resources.

No distributed lock is added. The verified system has module-global locks, scheduler state, cache, and runner port pool, so multi-process/multi-container single-run semantics cannot be claimed. Deployment stays single-application-process unless a future separately designed distributed lease or leader-election contract is introduced.

Alternative: add a SQLite lease opportunistically. Rejected: the lock’s cross-process correctness, recovery, and database contention semantics exceed this change and are not covered by evidence. Alternative: permit concurrent scheduled batches. Rejected: it directly conflicts with the measured-in-source local ownership model.

### D4. Failure/cancellation taxonomy retains truthfulness

The externally consumable aggregate summary separates `preflight_invalid`, `preflight_timeout`, `preflight_error`, `deep_ok`, `deep_fail`, `deep_timeout`, `cancelled`, and `not_admitted`. Existing persisted status vocabulary remains unchanged unless a future API compatibility review approves a new stored enum; preflight terminal states are not serialized as `NodeProbeResult.status`.

Every worker owns its runner context with `async with`; cancellation propagates to active tasks, awaits cleanup with a bounded grace period, and reports unstarted work as `not_admitted` rather than fabricating a network failure. The batch-level deadline includes only batch admission/wait policy, while each deep probe retains its node/service limits.

### D5. Go is a decision gate, not an implementation lane

A controlled harness will compare the current baseline to the funnel using synthetic local targets only: open listener, immediate refusal, timeout blackhole, invalid target, and a mocked runner/deep service. It measures wall time, queue high-water mark, task/runner concurrency, terminal outcome correctness, cancellation cleanup, and event-loop health endpoint latency. It must not use real subscriptions, node credentials, external media services, or unbounded network traffic.

A production-like, redacted inventory benchmark may be designed only after the synthetic harness passes and data access is explicitly authorized. Thresholds must be frozen from baseline observations by the evidence owner, not invented here. A Go sidecar RFC becomes eligible only after the same fixture corpus reproducibly misses a frozen threshold, and Critic refutes the Python alternatives. A full Go rewrite is explicitly outside this plan because CSP’s FastAPI/business/UI/generator surface remains unmeasured against a migration cost and no data-migration/cutover authorization exists.

## Data Contracts

### Candidate input

`ProbeCandidate` is internal and ephemeral:

- `node_key: str` from existing canonical backend authority
- `server: str`
- `port: int` in `1..65535`
- `type: str`
- `node: Mapping[str, Any]` retained only inside the deep worker and never emitted by telemetry

### Terminal batch outcome

`FunnelOutcome` is ephemeral until deep completion:

- `node_key: str`
- `phase: "preflight" | "deep" | "admission"`
- `outcome: "preflight_invalid" | "preflight_timeout" | "preflight_error" | "deep_ok" | "deep_fail" | "deep_timeout" | "cancelled" | "not_admitted"`
- `elapsed_ms: non-negative int | null`
- `persisted: bool`

No endpoint, credentials, raw configuration, proxy URL, provider response, or error body belongs in aggregate telemetry.

### Aggregate summary

`FunnelSummary` contains `received`, individual terminal counts, `preflight_workers`, `deep_workers`, `queue_capacity`, `max_preflight_active`, `max_deep_active`, `elapsed_ms`, and optional `overload_reason`. This may be added to an internal scheduler projection; adding it to a public API requires a schema/route owner and compatibility test.

## Complexity Rationale

The two bounded queues and explicit outcome algebra are necessary because the current process boundary is expensive and cancellation must distinguish a direct endpoint hint from a proxy capability result. A single semaphore is insufficient to bound inventory task creation and cannot expose admission pressure. No permanent process pool, extra persistence model, extra scheduler, cache mirror, or dual writer is justified by verified evidence.

## Risks / Trade-offs

- [Direct TCP succeeds but the protocol is unusable] → Stage 1 is named and surfaced only as admission evidence; stage 2 remains authoritative.
- [Protocol may be UDP-oriented or otherwise TCP-atypical] → The evidence card must classify supported node types and define a per-type policy; no blanket TCP rejection is authorized until fixtures cover the type.
- [DNS resolution can consume preflight capacity] → Preflight timeout includes DNS and TCP as today; benchmark records outcomes separately enough to observe saturation, not mislabel them as capability failures.
- [A queue cancellation leaves runner subprocesses] → cancellation harness asserts port return and process-context exit before accepting implementation.
- [Scheduler remains process-local] → deployment constraints explicitly prohibit horizontal/multi-worker claims; a new architecture plan is required before changing this.
- [SQLite persistence creates lock contention] → worker phase has no shared session mutation; persistence batching/chunking is benchmarked and failure-tested before a write path changes.
- [Existing uncommitted work is a hot area] → executor receives exclusive file ownership and must re-read current files immediately before patching; all other existing files are out of scope.

## Migration Plan

1. Evidence owner freezes fixture corpus, baseline measurements, supported-type disposition, initial limits, and benchmark acceptance thresholds without altering runtime behavior.
2. Critic performs RFC refutation of the frozen contract, including false-positive preflight, cancellation, local-vs-distributed scheduler ownership, and egress isolation. Rejection returns to Strategy for a revision.
3. One executor owns the specified backend files and focused tests. It implements the funnel behind the existing batch interface, without public config/default changes.
4. Reviewer reruns focused backend harnesses, verifies no identity/persistence/UI regressions, and checks source-level rejection of unbounded task fan-out and preflight-as-capability writes.
5. Critic runs adversarial fixtures and compares benchmark evidence. Only then may Planner schedule a separate release-decision task; no deployment or configuration change follows automatically.

Rollback is a source/test revert of the funnel change. The plan creates no migration and makes no new durable writes. If future release work changes configuration or deploys an implementation, it needs a distinct authorized operation and verified backup/rollback procedure.

## Open Questions

None are safely deferrable for implementation. The exact preflight timeout, worker values, supported protocol exemptions, public telemetry shape, and any Go follow-up are evidence gates that must be resolved before the high-risk contract freezes.
