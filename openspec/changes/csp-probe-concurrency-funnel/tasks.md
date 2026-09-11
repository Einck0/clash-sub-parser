## 1. Evidence gates and contract freeze

- [ ] 1.1 [Recon, read-only] Produce `EvidencePacket v1` from an already-authorized isolated CSP test environment: record baseline inventory size, node-type distribution, direct TCP preflight outcome distribution, current runner process high-water mark, RSS/CPU, batch wall time, event-loop health latency, current `ProbeConfig`, and active deployment topology. Redact endpoints and credentials. Verify the packet contains raw command provenance, timestamps, tool versions, and a reproducible synthetic fixture manifest.
- [ ] 1.2 [Strategy] Freeze `ImplementationContract` limits only from EvidencePacket v1: supported type disposition, preflight timeout, preflight/deep worker and queue bounds, batch deadline, overload response, aggregate public-telemetry decision, and benchmark thresholds. Verify every numeric field links to a measurement or is explicitly deferred from implementation.
- [ ] 1.3 [Critic RFC refutation, mandatory] Refute the frozen funnel contract against false-positive TCP acceptance, UDP/TCP-atypical protocols, DNS saturation, task cancellation, process/port leakage, scheduler overlap, persistence contention, multi-worker deployment, and host-proxy contamination. Verify `CRITIC_VERDICT: PASSED`; any rejection creates a Strategy revision rather than implementation.

## 2. Funneling implementation

- [ ] 2.1 [Executor backend-funnel] Implement the frozen ephemeral candidate/outcome contract using the existing canonical `node_key` authority. Exclusive write scope: `backend/app/services/tcp_probe_service.py`, new funnel module under `backend/app/services/probe/`, and new focused synthetic tests. Do not edit persistence, scheduler, routes, schemas, frontend, or deployment files. Verify fixtures cover invalid, refusal, timeout, open listener, TCP-success/deep-fail, bounded queue high-water, one terminal outcome per candidate, and no credentials in summary.
- [ ] 2.2 [Executor backend-orchestrator] Integrate the frozen bounded funnel at the existing `probe_batch_nodes()` boundary. Exclusive write scope: `backend/app/services/probe/service.py` and focused orchestration/deadline tests. Preserve `NodeProbeResult` as the only persisted authority; persist only deep results. Verify no unbounded `gather` is created over the full inventory, preflight results never enter capability filters/persistence, batch credential hydration remains one read, and cancellation releases runner contexts.
- [ ] 2.3 [Executor scheduler-owner] Integrate the frozen periodic-run ownership and aggregate status projection. Exclusive write scope: `backend/app/services/scheduler.py` and focused scheduler tests. Do not add a distributed lock or a second scheduler. Verify baseline tick, overlap skip, clock rollback, cancellation/failure reset, no held database transaction during queued deep work, and subsequent-run recovery.
- [ ] 2.4 [Executor API contract, conditional] Only if contract 1.2 explicitly exposes aggregate funnel telemetry, update the owned probe schema/router and focused route tests. Exclusive write scope: `backend/app/schemas/probe.py`, `backend/app/routers/probe.py`, and route tests. Verify additive privacy-safe fields, no endpoint/config/credential/provider-body exposure, and existing client response contracts remain green. Otherwise mark as deliberately skipped.

## 3. Verification and decision gates

- [ ] 3.1 [Executor verification] Run the frozen synthetic benchmark harness against baseline and funnel variants, with no external subscriptions or media requests. Verify the recorded output includes command, commit/worktree state, fixture manifest, configured bounds, wall time, max active tasks/runners, queue high-water, terminal outcome matrix, cancellation cleanup, and event-loop health latency.
- [ ] 3.2 [Reviewcommon, read-only] Independently rerun all focused backend harnesses and inspect the implementation against `ImplementationContract` IDs below. Verify no new shadow persistence, dual writes, process pool, ambient-proxy use, public-secret leak, local canonical-key formatter, or unrelated workbench identity/UI modification.
- [ ] 3.3 [Critic, read-only] Run adversarial fixture/refutation tests after implementation, including preflight success with isolated proxy failure, cancelled runner startup, deep timeout, queue overload, scheduler restart, and no-admission behavior. Verify `CRITIC_VERDICT: PASSED` before any release decision.
- [ ] 3.4 [Strategy decision] Compare verified funnel benchmark evidence with frozen thresholds. If all thresholds pass, record “retain Python control plane + bounded funnel; Go not authorized.” If a threshold reproducibly fails, open a separate Go-sidecar RFC only; a full Go rewrite remains out of scope. Verify the decision cites the benchmark packet and critic verdict.

## Atomic task proposals for Planner

| Proposal | Assignee | Dependency | Workspace | Exclusive files | ImplementationContract | Acceptance harness |
| --- | --- | --- | --- | --- | --- | --- |
| CSP-FUNNEL-EVIDENCE | recon/probe | none | read-only isolated environment | none | `ic_pending-evidence_CSP-FUNNEL-EVIDENCE` | EvidencePacket v1 with measured baseline and fixture provenance |
| CSP-FUNNEL-FREEZE | strategycode | CSP-FUNNEL-EVIDENCE | planning-only | `openspec/changes/csp-probe-concurrency-funnel/**` | `ic_<policy_hash>_CSP-FUNNEL-FREEZE` | versioned contract resolves all implementation-changing values |
| CSP-FUNNEL-CRITIC-RFC | critic | CSP-FUNNEL-FREEZE | read-only | none | `ic_<policy_hash>_CSP-FUNNEL-CRITIC-RFC` | `CRITIC_VERDICT: PASSED` |
| CSP-FUNNEL-CORE | executor | CSP-FUNNEL-CRITIC-RFC | isolated worktree | `tcp_probe_service.py`, new `services/probe/funnel.py`, new tests | `ic_<policy_hash>_CSP-FUNNEL-CORE` | focused synthetic funnel tests |
| CSP-FUNNEL-ORCHESTRATOR | executor | CSP-FUNNEL-CRITIC-RFC | isolated worktree | `probe/service.py`, focused orchestration tests | `ic_<policy_hash>_CSP-FUNNEL-ORCHESTRATOR` | bounded admission, persistence, cancellation harness |
| CSP-FUNNEL-SCHEDULER | executor | CSP-FUNNEL-CRITIC-RFC | isolated worktree | `scheduler.py`, focused tests | `ic_<policy_hash>_CSP-FUNNEL-SCHEDULER` | local ownership and recovery harness |
| CSP-FUNNEL-API | executor | CSP-FUNNEL-FREEZE | isolated worktree | schema/router/route tests only if telemetry is approved | `ic_<policy_hash>_CSP-FUNNEL-API` | privacy/additive contract test or deliberate skip |
| CSP-FUNNEL-REVIEW | reviewcommon | CORE, ORCHESTRATOR, SCHEDULER, API | read-only | none | `ic_<policy_hash>_CSP-FUNNEL-REVIEW` | independent rerun plus source-boundary audit |
| CSP-FUNNEL-CRITIC-POST | critic | REVIEW | read-only | none | `ic_<policy_hash>_CSP-FUNNEL-CRITIC-POST` | adversarial verdict and benchmark comparison |
| CSP-GO-DECISION | strategycode | CRITIC-POST | planning-only | OpenSpec only | `ic_<policy_hash>_CSP-GO-DECISION` | evidence-gated retain/escalate decision |

The literal policy hash and all non-placeholder `ImplementationContract` IDs are intentionally unresolved until 1.1 and 1.2: creating stable identifiers before their policy inputs are empirically frozen would falsely imply an approved contract.

## File ownership and hot-file policy

`backend/app/services/probe/service.py` and `backend/app/services/scheduler.py` are hot files. Their executor cards MUST be separate worktrees and cannot merge simultaneously; Planner serializes their integration after both have passed focused tests. The existing `shadcn-workbench-and-probe-contract-fix` worktree owns all frontend, node identity, result-map, proxy-chain, generator, subscription, and capability-filter changes; no funnel card may touch them.

## Rollback declaration

This plan has no production side effects. Implementation rollback is a source/test revert of only the funnel diff. No database migration, retention cleanup, persistent preflight result, process daemon, or configuration default change is allowed. Any later configuration/deployment action requires a separate approved release task with measured rollback steps.
