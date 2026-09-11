## Why

CSP’s current deep-probe path starts an isolated sing-box process for every candidate node before proving that the endpoint is even TCP-reachable. The in-repository implementation has a process-per-non-direct-node lifecycle and a batch semaphore capped at 20, while the periodic scheduler awaits the entire batch in its application event loop. This makes dead-node-heavy inventories consume the same scarce process slots and service budget as potentially usable nodes.

The existing node-key/workbench change is planning-valid but still has unchecked execution, review, and visual-gate work. The concurrency change must add only an independently owned probe funnel and must not reopen its frozen identity or UI contracts.

## What Changes

- Add a two-stage, bounded probe funnel: an explicit direct endpoint TCP preflight stage followed by the existing isolated egress/deep-probe stage only for candidates admitted by the preflight policy.
- **BREAKING (probe execution semantics):** batch and scheduled probe summaries will distinguish `preflight_timeout` and `preflight_error` from deep-probe `timeout` and `fail`; an endpoint that fails preflight is not represented as a validated proxy failure or an unlocked capability.
- Add an explicit scheduler run-ownership and admission contract: one application process may execute one periodic probe run at a time; interactive batches have a separately bounded deep-probe admission budget and cannot share an unbounded task fan-out with the periodic run.
- Add resource and outcome telemetry that contains only aggregate counts, elapsed times, and queue/admission metrics. It must not log node credentials, full node configurations, or provider response bodies.
- Define a benchmark harness and decision gate for the Go question. The default target is Python control plane plus a Python preflight and bounded existing sing-box runner; neither a full Go rewrite nor a Go sidecar is authorized by this change. A later proposal may be opened only if the benchmark gate fails and an independently reviewed Go sidecar RFC is approved.
- Preserve all existing egress rules: preflight is a host TCP liveness hint, not a proxy capability assertion; all identity, media, AI, and speed checks remain explicit through the candidate node’s isolated runner with environment-proxy inheritance disabled.
- Keep the pre-existing `shadcn-workbench-and-probe-contract-fix` change separate. This change may add only backend/API telemetry fields and its own focused harnesses; it does not modify Node Ledger presentation or shadcn primitive contracts.

## Capabilities

### New Capabilities

- `probe-admission-funnel`: Bounded endpoint preflight, deep-probe admission, outcome taxonomy, and egress-isolation requirements for node probing.
- `probe-run-governance`: Periodic and interactive run ownership, cancellation, metrics, failure behavior, and benchmark decision gate.

### Modified Capabilities

- None.

## Impact

- Expected implementation surface is limited to `backend/app/services/tcp_probe_service.py`, `backend/app/services/probe/service.py`, `backend/app/services/scheduler.py`, owned probe schemas/routes only if aggregate telemetry is exposed, and focused synthetic tests.
- No database migration, persistence schema change, dependency installation, service restart, production configuration change, live probe, or deployment is authorized by this plan.
- Existing `NodeProbeResult` remains the sole persisted result authority; the funnel must not add a shadow result table, dual-write path, or a second scheduler.
- High-risk contract: it spans scheduler, shared probe I/O, persistence semantics, and public summaries. It remains DRAFT until the evidence and Critic RFC gates in `tasks.md` are passed.
