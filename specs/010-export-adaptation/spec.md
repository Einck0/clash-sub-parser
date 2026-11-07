# 010 Export adaptation

## Scope and authority
User requests optimization of every CSP export and repair of export errors, considering node-only exports or target-specific strategies and subs-check. Run pi_run_20261108_030008_2419874. Development/push dev/cloud and isolated acceptance authorized; production/prelease not authorized. Prior frontend/probe work and dispatcher 300-second adjustment are not implementation scope. Preserve all 225 pre-existing dirty records.

## Findings, not assumed production incidents
Source at 6f64097: direct/live BuildCurrentArtifact rejects an entire mixed inventory on unsupported target capabilities; preview has a separate compatible branch. FilterCompatibleSnapshot checks only generic credentials, not actual target renderability or chains. Preview's successful facts fast path ignores requested compatibility and returns an unpersisted snapshot digest although publish requires a persisted draft. Mihomo admits SS plugin fields but omits serialization. These are source-confirmed paths; cloud tests and synthetic runtime must establish regression behavior, not claim production reproduction.

## Stories
### US1 (P1) Usable target-specific subscriptions
Given SS plus VLESS and invalid target-specific transport, Surge/QX return only genuinely renderable nodes rather than fail the whole mixed subscription. Sing-box emits its existing typed node configuration; Mihomo keeps full policy/DNS/rules. Explicit strict preview still reports all incompatible nodes. Default non-Mihomo preview and direct/live/check share compatible adaptation. No automatic fallback to DIRECT or altered credentials.
### US2 (P1) Consistent preview and actual delivery
Preview matches direct/live output for identical facts and mode; successful preview returns a real persisted draft that can be published. Included/excluded manifests reflect rendered IDs, not original inventory. Existing immutable publications remain immutable.
### US3 (P1) Faithful Mihomo SS plugins
A supported SS plugin and options survive rendering; unsupported plugin/transport causes actionable rejection instead of silent option loss.

## Requirements
- FR001: One renderability decision per target underlies diagnostics and compatible filtering, including actual target renderer failures, credential completeness, unsafe syntax, duplicate names and unsupported dialer semantics; input snapshots remain unchanged.
- FR002: Direct/live/link-check non-Mihomo exports adapt compatible nodes in original order. Mihomo remains strict full policy, preserving group order, PROCESS-NAME, terminal MATCH, DNS and primary fallback safeguards. Active chains on non-Mihomo retain explicit fail-closed error this iteration; never strip detour to direct.
- FR003: All incompatible nodes yield explicit no-compatible-nodes error, not successful blank subscription or primary fallback bypass. Empty original inventory preserves existing established behavior. Exclusions carry stable IDs and safe reasons without secrets; HTTP direct/live expose aggregate excluded count.
- FR004: Preview validates mode (strict/compatible), defaults per target, uses same adaptation on consistent facts, persists every successful draft, and manifests include exactly rendered nodes and exclusions. Strict remains explicit opt-in for node targets. No repository fallback may bypass consistent-facts failure or chain/risk constraints.
- FR005: SS plugin/options serialize to native Mihomo fields using existing accepted plugin schema; no unknown options silently discarded. Validate with existing cloud Mihomo check and behavior fixtures.
- FR006: No DB schema/data changes, no new converter runtime, no compatibility glue/dual type tracks; stable IDs/source evidence/auth/revocation/CAS/concurrency/ETag/userinfo and risk/global filters preserved. Unknown client capabilities not expanded based on unverified scout statements.

## Success criteria
- SC001: Cloud behavioral fixtures prove mixed-inventory target projection and strict-vs-compatible, target syntax failures, all-excluded, chain failure, immutable input and no secret diagnostics across four targets.
- SC002: Same-SHA prebuilt runtime with fresh synthetic DB proves direct aliases, preview->publish->immutable download, live link/check, auth and deterministic ETag/userinfo; no external probes/fetches or production data.
- SC003: Cloud Mihomo/sing-box schema validations pass; final independent reviewer verifies SHA/jobs/digest/OCI and runtime artifacts. No UI modifications, Critic not applicable under frozen backend contract.
