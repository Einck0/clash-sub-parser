# Tasks

## Active continuation — global empty-group PASS (pi_run_20261113_001940_3770721)
Current authority: `spec.md` US4/FR007/SC001–SC002; ordered source wording, static evidence and matrix GP-A01–GP-A12: [global-empty-pass-20261113.md](global-empty-pass-20261113.md). Strategy is the sole planning author. Historical checked tasks below prove only their recorded earlier scope, never this continuation's completion. Existing storage work remains separately tracked and unchanged. No production authorization.

- [x] GP001 [US4] Strategy reconcile latest original quotation and ordered corrections; supersede primary fallback and per-group-control clauses in `specs/010-export-adaptation/{spec,plan,tasks}.md` and add narrow applicability notes to `specs/002-policy/{contracts,confirmed-fixes-contract,prior-requirements-register,production-coverage-register}.md`, `specs/001-inventory/spec.md` HR-084–HR-086 and `specs/007-specification-governance/plan-contracts.md` DOC-0182; preserve historical text/evidence.
- [x] GP002 [US4] Strategy record baseline static consumer/default/target/error facts and acceptance matrix in `specs/010-export-adaptation/global-empty-pass-20261113.md`. Source inspection only, NOT test or runtime acceptance.
- [ ] GP003 [US4] Advanced execution compares final code and existing tests against GP-A01–GP-A12, supplies concrete test outline to low-tier executor, and coordinator freezes exact disjoint write ownership. Read `migrations/000031_global_empty_pass.sql`, `internal/repository/sqlite/node_filters.go`, `internal/application/policy/service.go`, resolver/compiler/publication consumers and settings/group UI; do not duplicate correct implementation or overwrite parallel `service_fallback_test.go` edits. Depends GP002.
- [ ] GP004 [US4] Assigned implementer/low-tier test author close only proven migration/persistence/global-over-legacy/draft/UI gaps (GP-A01–GP-A03, GP-A12) in the paths/tests assigned by GP003; include omitted update vs explicit false, failed repository reads/saves, existing/new groups and no group control. Tests cloud-only. Depends GP003.
- [ ] GP005 [US4] Assigned implementer/low-tier test author close only proven native-PASS/normal-export/error/target/consumer gaps (GP-A04–GP-A11) in resolver/compiler/publication and HTTP tests assigned by GP003. Preserve strictness, no primary replacement and independent blockers; protect existing parallel test edits. Can run in parallel with GP004 only on disjoint files. Tests cloud-only. Depends GP003.
- [ ] GP006 [SC001] Coordinator reviews task-owned explicit paths/index, commits and pushes dev, then records the exact SHA and existing Actions `test-and-build` + `publish-runtime` jobs and failure logs. Repair/re-push genuine failures; dev release skip is expected. No local compilation/testing. Depends GP004/GP005 or explicit evidence existing code already covers those cases; no artificial requirement for new code.
- [ ] GP007 [SC002] Coordinator obtains same-SHA prebuilt image digest/OCI revision; assigned acceptance owner exercises GP-A01–GP-A12 in isolated synthetic DB/UI/routes, no production mounts or real upstream traffic, records commands/results and owns isolation cleanup. Native continuation needs behavioral evidence, not just a PASS substring. Depends GP006.
- [ ] GP008 [SC004] Distinct independent reviewer inspects final diff, latest source wording and raw exact-SHA Actions/image/runtime/UI evidence; coordinator evaluates findings and records remaining failures/limits and delivery status. Strategy revises planning only if a new authorized decision changes it. No production deployment. Depends GP007.

### Current traceability
US4/FR007 -> GP001–GP005 -> GP-A01–GP-A12. FR001–FR006 regression preservation -> GP005/GP006/GP-A06–GP-A11. SC001 -> GP006; SC002 -> GP007; SC004 -> GP008. FR008–FR011/SC003 are not reimplemented or declared complete by this continuation. All behavioral/cloud/UI gates remain unchecked here until real same-source evidence exists.

## Original export iteration (completed; historical)
- [x] T001 Investigate native compiler, service paths and pinned subs-check reference.
- [x] T002 Freeze target projection/strict contracts.
- [x] T003 [P] [US1] Canonical per-node renderability/projection and behavior tests.
- [x] T004 [P] [US1,US2] Shared facts artifact pipeline, persisted preview drafts, exclusion diagnostics/count headers and tests.
- [x] T005 [P] [US3] Native SS plugins with pinned-source validation and tests.
- [x] T006 Exact-scope dev pushes and same-SHA cloud checks/runtime passed.
- [x] T007 Same-SHA synthetic isolated backend acceptance and cleanup.
- [x] T008 Independent source/cloud/image/runtime review PASS.
- [x] T009 Record delivery/evidence and terminal report.

Historical product a166cb5c8c9af575b5707fad5b708422e313996a, Actions42421228451 checks128549876691/runtime128550658273 SUCCESS, prelease128550837228 SKIPPED. Image sha256:77f26749f0502b2c6bfd2a1772d1fc9f2119962fc2fbff10dbf1b77fef426dfd, OCI same commit; binary7dc73bcaae496ebc08596889445b2d8009cb3ec423b6db209e32e0f6edaa22f5. Independent reviewer rep_rev_whole_task_c450bb42-fe8f-4316-8365-ac16eb62a7d9 PASS. Synthetic SS/pluginSS/VLESS: Mihomo3/0, sing-box2/1, Surge/QX1/2. Direct aliases, strict mode, preview/publish/immutable, live/check, all-excluded422, chain errors, auth/ETag/userinfo verified. Preview39974 and owned resources cleaned. Proprietary app import not verified.

Historical failures retained: b9912ed invented credential fields;31e57f6 omitted config declarations;5f9fc6e invented Node.ConfigJSON;e77ed1d obsolete tests/Reality key;b592c5d invented Publication.Manifest;c7d8f39 HTTP fixtures.5962faf cloud/runtime initially passed but master found silent plugin downgrade; fixed a166cb5 and reran. Original225dirty preserved. Separately authorized actual rollout and remaining empty 香港422/readiness cost are in release-20261108.md; original completion is not proof of current repairs.

## Historical continuation: run pi_run_20261108_111123_2774365 (empty-selection recovery superseded)
- [x] T010 Inspect scoped fallback/storage/readiness source; reconcile latest repair authority and preserve baseline225dirty (FR007-FR011).
- [x] T011 Freeze selection-only primary fallback, durable override ledger, reference-safe snapshot/fetch bounds and fast/deep readiness split in spec.md/plan.md; analyze coverage below.
- [x] T012 [P] [US4] Publication eligible-failure classification/recovery and regression tests in internal/application/publication/service.go and package tests (FR007).
- [x] T013 [P] [US5] Migration000028, durable override repository and reconcile projection, safe lifecycle pruning/wiring/predicates and behavior tests in inventory and SQLite files owned by moduleB (FR008-FR010).
- [x] T014 [P] [US6] Fast operational readiness separate from deep FK diagnosis; callback and tests (FR011).
- [x] T015 Audit merged models/SQL/imports/fixtures and protected worktree; exact-scope dev commit/push, existing same-SHA checks/runtime (SC001/SC003).
- [x] T016 Same-SHA prebuilt fresh synthetic backend acceptance: empty-group direct/live/preview fallback/safety, repeated refresh retention and real readiness; owned isolation cleanup (SC002/SC003).
- [x] T017 Independent reviewer checks source, cloud/image/runtime chain and invariants; evaluate findings and resolve genuine blockers (SC004).
- [x] T018 Record verified results/limitations, inspect live dispatch aggregate, commit documentation and send truthful terminal report. No production cleanup/VACUUM/deployment (SC004).

## Historical Spec Kit analyze: 2026-11-08 continuation
Explicit feature010-export-adaptation, no reinitialization/overwrite scripts. Root constitution/AGENTS cloud rules preserved. FR001-FR006 -> historicalT003-T008 and current regressionT015-T017; FR007 -> T012/T016; FR008-FR010 -> T013/T015-T016; FR011 -> T014/T016; SC001 -> T015; SC002/SC003 -> T015-T016; SC004 -> T017-T018. US4-US6 mapped to disjoint parallel module owners; no uncovered requirement. Original no-schema/partial-empty-error text superseded explicitly in spec/plan, not silently inherited. Count limits have reference exceptions and physical reclamation separate from logical retention. No active unresolved design requirement; actual test/gate status remains pending, not PASS.

## Storage continuation pi_run_20261110_174812_1075841 (partial, NOT verified delivery)
- [x] S001 Read managed original wording/safety correction and all three design artifacts; confirm clean isolated worktree at 8877588439895a858773fc12574f44a6ae6f2b2d and preserve primary tree/index.
- [x] S002 Supersede current FR008/SC003 target requirements without rewriting historical results.
- [x] S003 Implement explicit read-only inventory (schema/content fingerprints, protected table digests, separate latest-attempt/raw references, unresolved survivor/consumer blockers) and private CLI plan. Reject unimplemented apply/verify without opening a DB. This is an inventory, not a sealed executable migration plan.
- [x] S004 Add explicit transaction-owned version lease preparation/acquisition/release primitives: FK RESTRICT, owner fencing, revision-zero support, atomic snapshot, no expiry stealing or automatic GC. No production consumer is yet wired.
- [ ] S005 Root assigns unused migration number and additional precise test write scope (current_partial_consumers_test.go, partial_shared_test.go); do not modify those outside current authority.
- [ ] S006 Prepare evidence/provenance/observations/current-state schema and startup legacy no-write guard; fresh initialization only after new reconcile invariants are functional.
- [ ] S007 Atomic single-generation reconcile, latest-attempt/raw/applied fencing, honest history and partial-membership replacement; unchanged failure policy, all affected consumer regressions.
- [ ] S008 Frozen facts -> snapshot -> manifest provenance, immutable origins and single-transaction draft+refs; update pin consumer and metadata-only historic access.
- [ ] S009 Wire actual recover_history leases, prove probe snapshot/pagination boundaries, enable GC only after consumer closure.
- [ ] S010 Sealed scratch-only apply/verify, immutable backup/inode/alias checks, asset accounting and interruption/capacity/rollback evidence; never transform original DB or backup.
- [ ] S011 Root explicit-path dev commit/push -> existing same-SHA cloud checks/runtime -> isolated prebuilt acceptance -> independent reviewer. No self-test PASS from static checks.
- [ ] S012 Separately authorized measured production conversion/cutover; no production operation performed by this implementation assignment.

## Storage continuation pi_run_20261110_231040_1298183 (partial; no cloud PASS)
- [x] C001 Read complete original wording/current rules; retain original dirty main tree and all43 partial worktree paths. Independently investigate old run: 429 native retry exhaustion -> SIGTERM143; no stale process or demonstrated conversion/deploy effect. Index remains empty.
- [x] C002 Author conservative continuation scope at `/tmp/csp-compaction-safe-1298183/continuation-design.md`; distinguish user deployment intent from actual protected execution gates.
- [x] C003 Harden read-only verifier to one snapshot with applied/attempt/source-generation consistency checks; add synthetic mismatch regressions (not run).
- [x] C004 Add explicit-decision transaction-only schema30 scratch finalizer; reject fingerprint/source/applied/membership conflicts, preserve pin/publication evidence, protected assets and revision0; add rollback/single-generation tests (not run). CLI apply is still rejected; no file/backup orchestration or GC.
- [x] C005 Independent source review identified conflicting applied state could be overwritten by raw proof; add pre-proof applied-state rejection and regression branches; independent reconsideration confirms source finding corrected, overall reviewer BLOCKED (756efcad-a357-443a-9513-d1e4faf51423, reverifies6c63c863-60c9-423c-bebf-f1ba10a682e0). No behavior PASS.
- [x] C009 Continuation compares four primary dirty code overlaps against original baseline and isolated implementation: original-config admission/atomic persistence/read/source invalidation and count/redaction fixes preserved or strengthened; no unique primary business hunk omitted. Related primary tests preserved in isolated baseline with fixture/auth corrections. Source inspection found stale original-config regression expectations for seeded refresh/two-generation retention and accumulating failed fetches; align with strict one generation and latest-attempt summary, retain raw/node/attempt assertions. Scope is now46 paths (adds original_config_test.go), not45; updated explicit path/hash manifest and secret-scan evidence. No local behavior test or reviewer PASS claimed.
- [ ] C006 Root explicit-file integration/development commit/push dev and existing same-SHA CI/runtime; worker cannot commit or push. Old8877588 CI does not cover current dirty code. No local compile/test fallback.
- [ ] C007 Implement safe filesystem identity/backup/exclusivity/capacity manifest, source27 reconciliation and CLI apply/VACUUM; close all version consumers then lease-safe bounded version GC. Conservative primitive currently rejects unproved legacy facts instead of guessing.
- [ ] C008 Same-SHA prebuilt isolation, protected asset/rollback/capacity validation and complete independent review; only then applicable protected production backup/conversion/cutover and actual before/after metrics. No production operation/notification by this child.

## Historical verified delivery (earlier repair, not current PASS/storage acceptance)
Product b71159afc3d11824a4bd3946e40139c9595526a9, Actions42455516482 checks/runtime SUCCESS, promotion skipped. Same-SHA image e832a180a202823449becf21d54d906cb9cfd0c793c8e9c4cdad748e68403240, OCI exact. Eight synthetic backend scenario groups plus12 supplemental refreshes (18 total), steady-state6->6, each1 insert/1 prune, all reference exceptions accounted, FK0. Independent engineering and supplemental reviewer PASS. Owned preview resources closed,225 original dirty records exact match. Detailed decisions/failures/evidence and production-not-deployed limit in repair-20261108.md. Terminal notification only after live aggregate confirmation.
