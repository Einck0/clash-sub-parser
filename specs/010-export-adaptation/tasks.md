# Tasks

- [x] T001 Investigate native compiler, actual service paths and pinned subs-check reference; source-confirmed findings and boundaries recorded in spec.md/plan.md.
- [x] T002 Freeze target projection/strict contracts, preserve chain fail-closed and Mihomo policy; cross-artifact analysis below.
- [ ] T003 [P] [US1] Canonical per-node renderability and compatible projection with behavioral tests in internal/compiler/compiler.go and target_adaptation_test.go (FR001).
- [ ] T004 [P] [US1,US2] Unified publication artifact/preview adaptation, actual persisted drafts, excluded-node diagnostics/count headers and corresponding service/HTTP tests in internal/application/publication/** and subscription_direct/client.go (FR002-FR004).
- [ ] T005 [P] [US3] Faithful native SS plugins and plugin validation fixtures in internal/compiler/mihomo.go and dedicated tests (FR005).
- [ ] T006 Review complete scoped diff/fixture contracts, commit/push dev and verify same-SHA CI checks/runtime, repair actual failures without local compilation (FR006, SC001/SC003).
- [ ] T007 Run same-SHA prebuilt synthetic isolated backend acceptance for target matrix/aliases/preview-publish/links/auth/ETag, then clean exact owned resources (SC002).
- [ ] T008 Independent reviewer verifies final source/cloud/image/runtime evidence and all safety invariants (SC003).
- [ ] T009 Record real results, protect existing dirty changes, final authoritative aggregate and QQ delivery. No production rollout.

## Spec Kit analyze (2026-11-08, design)
Explicit feature directory 010-export-adaptation; no pointer overwrite. FR001 -> T003; FR002-FR004 -> T004; FR005 -> T005; FR006 -> T006/T008. SC001 -> T003-T006, SC002 -> T007, SC003 -> T006/T008. No uncovered requirements or ambiguous production authorization. Contract remains existing two compiler public entrypoints, disjoint ownership. Runtime proof not yet obtained; source findings not misrepresented as measured production failures. No critical/high inconsistency identified. Unknown actual client proprietary runtime validation remains limitation (Surge/QX unavailable on Linux); target grammar fixtures and authoritative docs, not false app-import claims.
