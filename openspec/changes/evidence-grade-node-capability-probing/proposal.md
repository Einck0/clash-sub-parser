## Why

CSP already verifies a candidate node through an isolated sing-box loopback proxy, but its media and AI results are still mostly single-response heuristics. A generic `200`, redirect, or edge/CDN response can be mistaken for availability, while challenge, rate-limit, and provider-contract drift are not preserved as decision evidence. The NodeLedger UI consequently cannot distinguish a genuinely verified unlock from an inconclusive response.

The refactor must raise judgment quality without discarding CSP's real protocol coverage, historical probe records, filtering, export targets, or the completed service-timeout and scheduling work.

## What Changes

- Introduce a declarative, credential-free provider catalogue whose probes expose an explicit request contract, accepted and rejected response signals, evidence fields, and a stable result taxonomy.
- Replace provider-specific ad-hoc dictionaries with evidence-grade results that separately model verified availability, partial catalogue access, region restriction, IP reputation block, challenge, rate limit, timeout, transport failure, and inconclusive contract drift.
- Verify exit identity using independent direct-through-node providers, record provider agreement/disagreement, and never convert an unavailable identity provider into host-egress data.
- Add a separate CDN-routing observation capability, beginning with YouTube mapping, and display it as routing evidence rather than asserting it is the legal or physical location of the proxy exit.
- Preserve legacy `media` keys and unlock semantics for existing NodeLedger, subscription, node-group, config-export, and five-target subscription workflows; add evidence fields additively.
- Upgrade NodeLedger and node-preview diagnostics to explain provider verdict, detected region, observed HTTP/redirect/evidence code, confidence, and timestamp without creating a second mobile UI. The established AppModal/AppDrawer scale, 4px/8px rhythm, 44px mobile targets, and no-private-geometry rule remain mandatory.
- Add fixture-driven provider contract tests, host-egress leak tests, result-schema compatibility tests, stale-contract quarantine tests, and 375/768/1024/1440px real-DOM review gates.

## Capabilities

### New Capabilities

- `evidence-grade-provider-probing`: Credential-free streaming, AI, identity, and CDN-routing probes with explicit verdicts, bounded evidence, and contract-drift quarantine.
- `probe-evidence-explanation`: A responsive diagnostic presentation that exposes the proof behind a probe result and keeps availability filters backward compatible.

### Modified Capabilities

- `node-capability-probe`: Replace opaque platform response heuristics with evidence-grade, per-provider classifications while retaining isolated real egress, service deadlines, and current platform keys.
- `node-probe-persistence-and-filtering`: Preserve historical probe data and extend persisted/filtered probe results with additive evidence and confidence semantics.

## Impact

- Backend: probe providers/service/runner result composition, probe schemas, `NodeProbeResult.media` JSON persistence, migration readiness tests, and provider fixtures. No external service credential, browser automation, or new runtime dependency is permitted.
- Frontend: NodeLedger domain filter helpers, preview badges, LedgerDrawer diagnostics, API types, and existing responsive test harnesses. Existing cards/table views, selection, bulk actions, exports, and all non-SCRIPT features remain.
- Operations: outgoing requests remain explicitly pinned to the tested node's loopback proxy with `trust_env=False`; per-service timeout, semaphore, and cleanup guarantees remain. Provider catalogue changes require versioned fixtures and review before activation.
