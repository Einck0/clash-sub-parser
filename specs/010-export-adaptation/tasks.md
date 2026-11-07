# Tasks

- [x] T001 Investigate native compiler, actual service paths and pinned subs-check reference; findings in spec.md/plan.md/research.md.
- [x] T002 Freeze target projection/strict contracts, preserve chains/Mihomo policy; cross-artifact analysis below.
- [x] T003 [P] [US1] Canonical per-node renderability/projection and behavior tests in internal/compiler/compiler.go and target_adaptation_test.go (FR001).
- [x] T004 [P] [US1,US2] Shared facts artifact pipeline, persisted preview drafts, excluded-node diagnostics/count headers and service/HTTP tests (FR002-FR004).
- [x] T005 [P] [US3] Native SS plugins with pinned-source validation and regression tests in mihomo.go/mihomo_plugin_test.go (FR005).
- [x] T006 Exact-scope dev commits/pushes and same-SHA cloud checks/runtime passed; failures retained (FR006, SC001/SC003).
- [x] T007 Same-SHA prebuilt fresh synthetic isolated backend acceptance, followed by owned-resource cleanup (SC002).
- [x] T008 Independent source/cloud/image/runtime review PASS (SC003).
- [x] T009 Record delivery/evidence; final aggregate PASS, no production rollout authorized. Terminal notification follows final status check.

## Spec Kit analyze
Explicit feature directory 010-export-adaptation; original pointer baseline preserved. FR001 -> T003; FR002-FR004 -> T004; FR005 -> T005; FR006 -> T006/T008. SC001 -> T003-T006, SC002 -> T007, SC003 -> T006/T008. No uncovered requirement or ambiguous deployment authorization. No UI changes, frozen backend Critic not applicable (not a claim Critic ran). Proprietary Surge/QX app-import testing unavailable; grammar fixtures and synthetic HTTP results do not imply native-app verification.

## Verified delivery (2026-11-08)
Product commit a166cb5c8c9af575b5707fad5b708422e313996a, Actions42421228451: checks128549876691 SUCCESS, runtime128550658273 SUCCESS, prelease128550837228 SKIPPED as required on dev. Image sha256:77f26749f0502b2c6bfd2a1772d1fc9f2119962fc2fbff10dbf1b77fef426dfd, OCI revision matches commit; binary sha256:7dc73bcaae496ebc08596889445b2d8009cb3ec423b6db209e32e0f6edaa22f5. No host or local Docker project compilation/tests.

Synthetic plain SS + plugin SS + VLESS: Mihomo includes3/excludes0 preserving plugin/DNS/470 ordered rules with terminal MATCH; sing-box includes2/excludes1; Surge/QX include1/exclude2. Unrepresentable plugins are rejected, never silently plain SS. Verified aliases, strict rejection, default compatible preview real draft -> publish -> immutable bytes, live/check agreement, all-excluded422, chain rejection/non-downgrade, auth401/200, ETag304/count headers and primary userinfo. This is synthetic evidence, not production data or proprietary-app import.

Independent reviewer rep_rev_whole_task_c450bb42-fe8f-4316-8365-ac16eb62a7d9 PASS, no active failures. Detailed local private evidence is retained outside Git. Cleanup removed exact owned preview container/network/volume/relay and synthetic credentials,39974 refused; no production deployment.

## Corrections and review decisions
Cloud failure history b9912ed (invented credential fields),31e57f6 (omitted private config declarations),5f9fc6e (invented test Node.ConfigJSON),e77ed1d (stale strict tests/invalid Reality key),b592c5d (invented Publication.Manifest),c7d8f39 (HTTP fixtures) retained. These were construction/coordination defects, not cloud infrastructure faults. Format-only interim PASS reports are not delivery evidence; final cloud and real runtime evidence govern.
5962faf initially cloud-green and synthetic suite passed, but master found SS plugin semantics silently lost on non-Mihomo. Accepted as blocking, fixed a166cb5, reran cloud and same-SHA runtime; prior broad success is superseded, not erased. Reviewer final PASS accepted based on source/cloud/runtime evidence. No stylistic rewrite or unsupported protocol expansion added.
