# Implementation plan: target-aware native exports

## Technical context / decision
Go native compiler + application publication + HTTP, existing Vue UI untouched. Keep existing node-only Sing-box/Surge/QX and full Mihomo. References: subs-check-pro 68394cbe0a7f76a778d1d2fa4b611d4a6dd16045 delegates external conversion to Sub-Store/QuickJS, not an intrinsic four-client compiler. Do not import external runtime or GPL code. Scout claims about client support are not authoritative enough to remove existing TUIC or other protocols: retain existing supported surface, fix demonstrated renderability gaps only.

## Module responsibilities and contracts
### A: compiler target adaptation
Own internal/compiler/compiler.go and new target-adaptation tests only. Existing public ValidateTargetCapabilities and FilterCompatibleSnapshot remain canonical entrypoints; enhance their implementation rather than invent second algorithm. A shared per-node renderability function checks generic fields, protocol capability, identity/name, non-Mihomo DialerProxy, and actual one-node target renderer (Mihomo validation/render, sing-box typed builder, Surge/QX line renderer). Render errors become safe capability diagnostics. Duplicate names detected deterministically (first renderable wins in compatible; later duplicates excluded); strict reports them. Filtering preserves order, clones group members, recomputes derived node identity membership used by compiler where necessary, never mutates input. Mihomo topology rules remain strict. Do not run full render on complete snapshots per node or introduce quadratic snapshot copies. No new protocol capability claims.
### B: application and HTTP consistency
Own internal/application/publication/** and internal/transport/http/{subscription_direct.go,subscription_client.go} plus directly corresponding HTTP tests. External compiler APIs remain same as above. Artifact gains ExcludedNodes []domain.ManifestExcludedNode (JSON field excluded_nodes), successful non-Mihomo snapshots carry safe warning diagnostics for exclusions. Direct/live headers X-Export-Excluded-Count report aggregate count including 304; no secrets in headers. Snapshot returned by BuildCurrentArtifact contains actual rendered nodes. Adapt only after existing risk/global filter resolution and before target capability checks; do not enable primary fallback for incompatible inventory. All-excluded errors are no_compatible_nodes. Non-Mihomo active-chain fail-closed behavior stays.
Default mode is strict for Mihomo, compatible for node targets. Preview explicit strict stays strict and explicit compatible invokes compiler filtering, no arbitrary other strings. Reuse consistent facts + same artifact builder in Preview without repository re-resolution fallback; successful previews must persist a draft and payload refs then return its actual ID, manifest matches output. Explicit injected Snapshot tests use same target adaptation contract. Maintain error/no-content draft semantics where useful but remove superseded dual algorithms, not legacy shims. Frozen existing immutable publication payloads are never rewritten.
### C: Mihomo SS plugins
Own internal/compiler/mihomo.go and dedicated plugin tests only. Existing accepted schema supports plugin/plugin-opts variants; determine native plugin mapping from current validators and authoritative Mihomo docs; serialize supported options preserving JSON scalar values. Unknown/unrenderable plugin/options fail closed. No protocol/cipher expansion. Existing module read exceptions compiler.go, resolver/types.go and domain/node.go. Do not change compiler.go (A owner).

## Scenarios and failures
1. Valid SS + VLESS -> QX/Surge includes SS, safe exclusion VLESS; explicit strict rejects. Sing-box/Mihomo preserve supported VLESS.
2. Valid SS + VMess grpc on QX -> valid SS delivered and target renderer reason for grpc; all grpc -> no_compatible_nodes, no fallback.
3. Any enabled chain on non-Mihomo -> explicit unsupported_target_capability; no direct conversion even if other nodes usable. Mihomo chain topology unchanged.
4. Successful preview -> persisted snapshot ID -> publish -> immutable download byte/digest equality. Live request and preview compatible bytes identical for same facts.
5. Risk/global selection failures, unknown references/cycles, partial Mihomo empty groups remain errors, never papered over by fallback.
6. SS obfs/v2ray plugin -> native fields/options survive; unsupported configuration explicit error, source credentials untouched.

## Dependencies and validation
A/B/C write sets disjoint, frozen existing compiler signatures permit parallel work. Master owns specs/shared Git index, exact-scope dev push. No local tests/build; static diff/gofmt allowed. Existing CI .github/workflows/ci.yml required checks + runtime image. Synthetic fresh DB/no-egress prebuilt same-SHA runtime acceptance and independent reviewer after cloud success. No UI changes/critic, no production access/deploy. Runtime preparer owns exact container/network/volume/relay/secret cleanup.

## Constitution check
Asset preservation: no persisted source edits. Stable IDs retained. Domain/compiler/application/transport separation preserved. No new storage abstraction/runtime or dual types. Living spec FR001-FR006 covered by tasks and validation. PASS at design review, subject to implementation evidence.

## Review decisions
Pending independent final review; findings will be evaluated on evidence, not mechanically accepted.
