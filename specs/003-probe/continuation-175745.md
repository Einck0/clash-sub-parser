# 2026-11-07 continuation — current run 175745

Current contract run `pi_run_20261107_175745_2035987`, change `003-probe`, package `default`, epoch1 supersedes old run identity, not old failure history. Previous process ended exit-9; EOF HTTP500 retries are evidenced but final signal cause unconfirmed. Preserve all worktree edits. No production deployment; dev push/cloud authorized; no host/local Docker tests/build.

## Latest user outcome

Continue all probe repairs and researched methods. User now explicitly requires **untested nodes are unknown**, not failure. Add default main-subscription export when rule selection cannot produce normal export. This amendment takes precedence over old `untested` node-health statements, not fine-grained skipped-attempt evidence.

## Unknown node state: single canonical path

No valid current-revision baseline (never tested, revised connection, future/invalid baseline, auxiliary-only, skipped/no completed connectivity result) => health_status `unknown`, label未知, latency absent. Baseline success =>healthy; actual baseline network failure=>unhealthy. Queue/probing remain orthogonal transient indicators. Per-attempt skipped still exposes why no result; unknown node does not invent failure/success. No DB backfill or dual emitted status. Replace internal HealthCategoryUntested with HealthCategoryUnknown (valueunknown), validators/filter choices use healthy/unhealthy/unknown; update consumers/tests, not compatibility branches. Existing unknown BaselineHealth already canonical. Legacy pool JSON schema retained without duplicate counts: unknown nodes count in undetermined_count, untested_count stays0. Total conservation unchanged. No automatic publication admission weakening from health naming.

### Ownership

Health owner writes `internal/domain/probe_health.go`, `probe_health_test.go`, `node_filter.go`, `node_filter_test.go`; `internal/application/inventory/service.go`, existing inventory read/reconcile tests; `internal/application/probe/service.go` and relevant pool tests in runner_test.go/periodic_test.go (no runner.go); `internal/repository/sqlite/node_list_all_test.go`; `internal/transport/http/nodes.go` and node route tests. Read-only domain/probe.go, repository/sqlite/nodes.go. UI owner writes entire actual affected `web/src/features/nodes/`, `web/src/features/probes/`, policy `{policyTypes.ts,StructuredFilterEditor.vue,policy.test.ts}`; no Go writes. Backend probe repair owner only runner.go/detectors/profiles plus tests not assigned to health owner; no domain health/UI/publication.

## Main subscription fallback: narrow explicit algorithm

Normal public live export remains current authenticated link or target route -> consistent facts -> existing resolution/preflight -> target compiler. Frozen publication artifacts unchanged. A fallback is allowed only for rule-selection exhaustion, not a generic catch-all.

1. Load enabled primary subscription ID and its current active eligible node IDs transactionally into ConsistentFacts (`PrimarySubscriptionID string`, `PrimaryNodeIDs []string`). Load node sources as needed for this selection even when no filters requested; source membership joins use existing actual schema. Include only main active eligible nodes, exclude notices/protocol constraints as normal export does; never raw-fetch subscription URL or export arbitrary raw YAML. Primary disabled/missing/empty cannot certify usable fallback.
2. Normal export attempted first. Trigger only when selected nodes empty (`filtered_nodes_empty`) or every effective routed user group is empty (`empty_routed_group` / `required_nonempty` caused by zero eligible membership) and no independent blocker. An active revision with no effective rules/groups is also rule-selection exhaustion if it otherwise cannot compile normal routing. A working explicit DIRECT/REJECT rule remains intentional and is not overridden. One empty routed group alongside usable routed groups is partial configuration error, not 'all rules ineffective'; do not discard functioning rules to bypass it.
3. On eligible exhaustion, use only primary node intersection with facts.Nodes; bypass rule/member/global selection filters for this explicitly requested fallback, but retain node active/enabled/notices/protocol security, explicit risk block/review/unknown policy, chain safety validation, DNS validation and target compiler strict capability. If original blockers include risk, graph cycle/missing reference, bad DNS, invalid protocol/credentials, internal storage, cancellation, auth/revocation, do not fallback. No-active-revision is an explicit configuration prerequisite failure for this increment, not synthesized or swallowed.
4. Reuse existing snapshot/compiler single path. Fallback creates deterministic minimal select group `主订阅兜底` + MATCH to it for Mihomo (via resolver/compiler model), primary-only nodes for other compiler targets. Preserve configured DNS, applicable chain bindings and primary userinfo. Do not mutate saved groups/rules/filter or primary subscription. Re-evaluate risk/strict compile on fallback. No raw upstream relay or redirect/token leak.
5. Surface safe `primary_subscription_fallback` warning diagnostic in returned snapshot/link check/preview so callers can tell fallback happened. If primary absent/disabled/empty, keep original meaningful rule-empty error with explicit fallback-unavailable diagnostic; never HTTP200 empty config. Content/snapshot digest deterministic for same input.
6. Same result for `BuildCurrentArtifact`, GenerateTargetArtifact and fixed GenerateLiveArtifact; preview/check uses same builder and flags fallback. Existing separate frozen publish/download immutable behavior unchanged. Do not retrofit all legacy publication snapshot APIs unless actual public live callers require it; list any unsupported entry explicitly.

### Module contract and ownership

Publication owner writes `internal/domain/facts.go`; `internal/repository/sqlite/consistent_facts.go` and narrowly named primary fallback tests; `internal/application/publication/` tests/service; transport subscription client tests if needed (no auth semantics changes). Read-only `internal/resolver/`, `internal/compiler/`, `internal/domain/{policy.go,node.go,subscription.go,ports.go}`, `internal/repository/sqlite/{subscriptions.go,node_sources.go,nodes.go}`, `internal/transport/http/{subscription_client.go,subscription_export.go,subscription.go}` as existing. No probe/UI writes, no schema migration. Cross-module interface frozen: new facts fields above only; existing public builder signatures unchanged. Need other shared field/path => master decision first.

## Scenarios

- No observation / auxiliary-only / baseline skipped / old revision =>unknown API+badge+filter+pool count, never failed. Valid baseline available/error unchanged; current slow positive latency only.
- Complete Gemini body late marker >256KiB accepted, late conflict/overflow rejected; Antigravity network_entry availability never auth/model success; country disagreement warning with both sources. Existing researched methods evidence remains in probe-prior-art-20261107.md.
- Global filter removes all candidates, main has active n1,n2 and non-main n3 =>fallback exports n1,n2 only; saved filter unchanged; primary userinfo and deterministic digests preserved.
- All routed groups empty =>same primary fallback; working route=>normal output unchanged; partially empty/error topology=>original error; DIRECT-only deliberate config unchanged.
- No primary/disabled/empty=>original failure + safe unavailable detail; risk block/storage/auth/revoked/invalid DNS/unsupported target protocol/graph cycle=>no fallback success. Live token check precedes any fallback.

## Tasks and analyze

- [ ] CONT01 Complete inherited probe code/tests under actual current diff; focused static check only locally.
- [ ] CONT02 Canonical unknown node health across domain/read model/filter/pool and tests.
- [ ] CONT03 Exact UI states/unknown health/filter and researched evidence scope; tests.
- [ ] CONT04 Main-only fallback facts/resolution/diagnostics + narrow positive/negative tests.
- [ ] CONT05 Explicit-file dev commit+push, existing Actions same-SHA required checks/build, isolate prebuilt image (digest/revision/freshDB/loopback); executor evidence and independent reviewer.
- [ ] CONT06 Applicable real UI critic after review; own resource cleanup; authoritative current aggregate query and task-only final commit/report. No production.

Spec Kit analyze: existing feature selected; rules map CONT01–06 with disjoint module ownership except explicitly sequenced pool tests (probe owner hands them off before health owner). Publication amendment linked from004 feature, no new project or destructive setup. Constitution asset/identity/clear domains intact. All scopes retain incomplete evidence honestly. Legacy probe-state `skipped` semantics applies to attempts, latest unknown applies to node health; no contradiction. Rule fallback is narrow whitelist, risk/security failures are excluded; no general exception fallback. Cloud only and independent evidence remain mandatory. No newly invented graph success (graphify absent127).
