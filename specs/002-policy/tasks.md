# Implementation tasks — CSP complete-delivery convergence

Feature `specs/002-policy`; current run `pi_run_20261016_085304_1567276`, change `002-policy`, package `default`, epoch1. Previous runs/failed tests are historical, not current PASS. Baseline copies of spec/plan/tasks before this amendment: `/tmp/csp-delivery-20261015-230722/planning-baseline/` with SHA evidence in the current session. Duplicate TASK-FIX entries were merged below without dropping their requirements. Checkbox totals are not a product completion percentage.

## Phase 1 — Current scope and safety

- [x] TASK-SPEC-01 Preserve existing rebuild contracts, models, historical requirement registers and 82-entry dirty baseline; resolve Feature deterministically with official prerequisite script. Reconcile latest user instruction: dynamic proxy settings and actual subscription failures are P1; phone access/testing is excluded.
- [x] TASK-SPEC-02 Amend `spec.md` FR-DEL-01..06, `plan.md` and this task list; preserve old sequential source semantics and explicit literal `去掉(...)` ambiguity. Reuse existing domains and cloud release mechanism, no new product extras or framework writes.
- [x] TASK-DIAG-01 Read-only production/scout evidence: schema17/open mode, proxy configured and reachable, enabled subscriptions two fetch successes/two fetch failures, error-redaction suffix bug, cloud publish skipped in latest known dev run, anonymously readable private-named image, unavailable local GitHub API auth. Evidence `/tmp/csp-delivery-20261015-230722/scout/`. No production refresh/write occurred.
- [x] TASK-SPEC-03 Run speckit-analyze across amended artifacts and constitution; fix critical contradictions before implementation. Record scope/coverage and residual ambiguities honestly.

## Phase 2 — Subscription recovery and dynamic proxy (FR-DEL-01..03)

- [ ] TASK-FETCH-01 Implement persisted environment/direct/proxy route mode in existing settings repository with migration19; `GET/PUT /api/v1/settings/subscription-fetch` management auth/CSRF boundary, validation, safe masked responses, audit and atomic saves. Existing subscription override remains highest precedence; no unintended auth/other settings overwrite.
- [ ] TASK-FETCH-02 Wire coherent per-refresh route/policy snapshot for actual manual/scheduled inventory reconciliation. Saving changes next refresh without restart, survives restart; precise configured-proxy trust does not widen target or redirect SSRF policy. Retired proxy trust does not accumulate.
- [ ] TASK-FETCH-03 Add settings UI with clear inherited/direct/custom state, password-safe endpoint edit/retain/replace behavior, save/error/reset semantics and translations. Actual API route selection is exercised, not a mock-only input.
- [ ] TASK-FETCH-04 Fix redaction so query/userinfo/assignment/wrapped errors retain safe cause suffixes and stable fetch failure stages. Tests prove no secret leaks and distinguish actual DNS/proxy/TLS/HTTP/parse/empty/partial cases; do not weaken TLS or target validation.
- [ ] TASK-FETCH-05 Verify temporary real SQLite + controlled HTTP/proxy servers: route changes, retained credentials, invalid-save rollback, restart persistence, concurrent settings/read behavior, subscription override, redirects/private targets, failed/partial/zero-source asset preservation. Obtain actionable bounded diagnosis of the two real failing sources if current evidence remains inadequate; genuine upstream failure is not silently labeled repaired.

- [ ] TASK-FETCH-06 Close existing subscription scheduling gap: lifecycle-owned bounded interval/standard-five-field-cron execution through the real refresh path, validation, disabled-state/config-change/shutdown behavior, no fake success without reconciler, no overlapping manual/scheduled refresh per source; deterministic time-controlled tests. See plan scheduling contract.

## Phase 3 — Existing rebuild closure and asset correctness (FR-DEL-04)

- [ ] TASK-FIX-01 Complete existing failed/partial source-association preservation and successful scoped pruning with historical node retention in inventory; multi-subscription failure isolation, actual SQLite regression tests.
- [ ] TASK-FIX-02 Complete pure-node target validate/preview/publish/serve independence from irrelevant group errors; keep global node/filter/risk fail-closed behavior and Mihomo full-graph validation. Typed builtins count correctly without fake outbound substitutions.
- [ ] TASK-BE-01 Enforce policy_group_sources sole authority after one-time migration18. Remove ongoing edge/source mirror and empty/error legacy fallback. Migrate supported edit/revision/rollback/delete actions to sources; obsolete writes reject explicitly without source corruption. Repository operations are transactional and revision-safe.
- [ ] TASK-BE-02 Validate sequential add/remove/re-entry/current-set dedup, all six kinds, mixed ordering, group_ref vs group_nodes, child-then-item conditions, reference and expansion cycles, global/subscription/risk admission without extra healthy-only constraint.
- [ ] TASK-FE-01 Complete existing unified ordered-source editor, add/remove/reorder/candidate pagination and cross-page selection; retain draft on409, show cycle errors, no group-global condition wrapper. Preview actual unsaved draft without DB writes and use common resolve-input/snapshot semantics.
- [x] TASK-MIG-01 Run migration18+19 on private production backup copy; prove group IDs/order/admitted membership/route semantics and immutable publication digests preserved. Do not claim PRAGMA user_version is schema version. Record before/after evidence without credentials.
- [ ] TASK-MIG-02 Finish historical three-round-robin/fourth-cheap-group copy-only proposal with actual archive mapping/order and resolved counts. Preserve original literal `去掉(...)`; do not invent exclusion interpretation or permanently rewrite ambiguous user configuration. Mark any unresolved business interpretation, not false completion.
- [ ] TASK-SCALE-01 Verify >250 nodes/150 groups real SQLite/HTTP save→GET order, conflict rollback, invalid regex/cycle, draft no-write and saved preview/export consistency. Fix relevant existing future-timestamp/health-filter pagination defects only where actual evidence shows confirmed functionality is broken; no unrelated enhancement expansion.

## Phase 4 — Cloud source alignment and final self-tests (FR-DEL-05)

- [ ] TASK-CLOUD-01 Preserve both local commits/dirty work and latest remote additions in an isolated integration work area; reuse existing remote CI/Dockerfile.runtime/compose.prod/cloud-delivery scripts/docs. Remove outdated Python/frontend references from current pipeline only after confirming replacements. No forced push, default-branch overwrite or duplicate release path.
- [ ] TASK-GATE-01 Final merged-state `go build ./...`, native full ordinary/tagged tests including tools, substantive affected-package race checks, web type-check/Vitest/isolated production build and diff checks. Record matched test cases >0, actual commands/exits/log hashes. Fix failing source-authority test instead of weakening constraints. Preserve prior failures and linkage. No premature embedded-asset dirt.
- [ ] TASK-REV-01 Independent reviewer against final merged patch/spec/plan: source authority, migration equivalence including refs/filters, revision/rollback, fetch trust lifecycle/redaction/secrets, asset preservation and export semantics; independently reproduce significant risks and confirm tests are effective. Correct failures and reverify with valid report lineage.
- [ ] TASK-PREVIEW-01 Non-critic prepares healthy isolated ready URL/temporary DB/disabled external schedulers/nonproduction ports and standard instance JSON with shutdown/cleanup ownership, source/asset manifest and real scenarios.
- [ ] TASK-CRITIC-01 Actual black-box interactions and PNG visual analysis of settings route save/error state, ordered-source edit/preview/conflict/pagination and representative existing pages in desktop/mobile four viewports. No phone access; browser mobile viewport is not physical-device proof. Critic cannot prepare/start/modify instance.

## Phase 5 — Authorized release and actual production acceptance (FR-DEL-05..06)

- [ ] TASK-RELEASE-01 After applicable gates, synchronize embedded assets and verify final generated delta; create scoped Git commit(s) via git_commit and non-force push the verified integration branch. Record Git SHA/patch identity; do not commit secret data or test-generated maintenance litter.
- [ ] TASK-RELEASE-02 Run existing GitHub checks/build/publish for exact current commit, record run URLs and non-skipped job results, OCI revision/image digest. Private visibility must be verified via authenticated gate and anonymous denial; existing anonymously readable package and missing API auth are explicit blockers until actually resolved. Do not read Docker credentials as API tokens, silently make images public, or claim cloud success for old SHA.
- [ ] TASK-IMAGE-01 Official immutable-image preflight/pull and isolated image acceptance; source/embedded assets/image revision consistency. Do not use mutable latest or substitute a local build to disguise blocked cloud publication.
- [ ] TASK-PROD-01 Fresh SQLite hot backup with integrity/hash plus safe business preimages/current-image rollback point. Deploy only CSP through official existing path after gates and scope authorization; preserve DB volume/ports/proxy/auth. Verify migration/version/assets/health and prove source-SHA→image-digest→container mapping; rollback if necessary without casually overwriting current DB.
- [ ] TASK-PROD-02 Execute concrete operations from production-coverage-register against actual domain/API/downloads, recording expected/actual business results, cleanup and retests. Check every enabled subscription refresh stage/count, actual settings effect/persistence, inventory/history, ordered policies/preview, probe truth, all publication targets/download/revocation and auth behavior without toggling production auth. Preserve users' assets, clean only task-owned objects, restore temporary settings. Unsupported/stale coverage entries are corrected and explicitly dispositioned, never silently skipped or treated as pass.
- [ ] TASK-DELIVER-01 Shut down/clean task previews and task-owned sensitive exports only, record retained processes and unresolved upstream/ambiguity/capability/coverage limits. Update `.exp.md` only with reusable verified lessons. Final Chinese plain-text QQ report: completed/not completed first, modifications, independent/local/cloud/browser/production verification, Git SHA, Actions URLs, digest and deployed identity, backup/rollback/cleanup and explicit uncompleted scope. No internal attachments or fake whole-task PASS.

## Dependency and ownership notes

Phase2 and Phase3 are separate functional scopes, but share inventory/domain/HTTP entry points; serialize shared-source implementation rather than race two writers. Cloud-only file preparation can be isolated after the source manifest is stable. Final combined-state self-test precedes independent review and critic; source/asset changes after gates require proportional re-verification. Runtime work_package remains whole_task for all launches; task IDs do not fabricate additional frozen packages. External manual testing aggregate budget is in plan.md; scout has already consumed <=4,470,628 bytes /97s. Any hard credential/external upstream/owner-visibility barrier is reported precisely while authorized independent local work continues.

## Current evidence update (2026-10-16, local delivery only)

- Subscription routing implemented and exercised with real temporary HTTP/proxy/SQLite; stage diagnosis of S02/S06 is TLS handshake timeout. S03/S08 source-copy success is not production-new-version acceptance. TASK-FETCH-05 remains open for complete final combined coverage/real-upstream recovery; no success fabricated.
- Latest full tagged Go suite: 1538 passed, 1 skipped, process exit0; tagged build0. Web final347/34files, typecheck/build0; affected-package race evidence in release-freeze/ready-instance and safety fix logs. Earlier failures/timeouts remain preserved. Graphify unavailable (127), no graph exists; not a graph-maintenance success.
- Backend/cloud independent review PASS `rep_rev_whole_task_1fd3ec05-27ce-46ee-a9b4-94912902a0e1`; final frontend delta PASS `rep_rev_whole_task_98732c18-71d2-4628-ab31-aa6eb58d354b`.
- Final critic PASS `rep_crit_whole_task_5d81f21a-42c7-43a9-913a-7db628a7e04f`, verified 17 native PNG read calls in actual session. Earlier critic report claimed visual/publication coverage without supporting execution; not accepted as evidence. Final critic explicitly excludes external upstreams/all-kernel/production/device verification.
- Final preview59865 source403files and3assets matched latest source/dist; preceding previews33677/48231 are task-owned and require shutdown. Synthetic DB only, both schedulers disabled. Preparing commit embeds only these exact assets, not a new build variant.
- Real production-copy migration17→19 proved29groups, ordered memberships, routes and immutable publications preserved. Schema20 snapshot table was subsequently introduced; final17→20 copy equivalence still to be explicitly receipted, TASK-MIG-01 stays open. Historical cold-archive proposals remain incomplete and ambiguous group unchanged.
- `auto_test` exposed but not implemented remains an explicit functional gap. GHCR private-named package was anonymously readable and local GitHub API auth unavailable; owner has been notified (delivery239921b3cca24300abc7370fbc1510a8). No new image, production deploy or complete production register has passed.

Checkboxes above refer to specific implementation/local review scopes, never whole-project completion. Latest upstream production state is not replaced by a local isolated acceptance.

Final asset/migration delta: `/tmp/csp-delivery-20261015-230722/finalize0700/verification-index.json` SHA7658e4692e0053ec8bd307753493e1d47a56e534e7cc279c7e295504e26939c0. Final production-copy17→20 conservation passed integrity/FK and all29group/publication checks. Exact3critic-reviewed assets synchronized and embedded build/test passed; all3task previews stopped and ports closed. Authorised storage-state export no longer exists. No production migration/deploy or cloud image publication occurred. TASK-RELEASE-01 remains partially open until commit and non-force push are actually receipted.

## Interrupted-run continuation tasks (current contract)

Historical open checkboxes above are not silently marked passed by reports; reconcile final evidence by explicit source/phase, never completion percentage.

- [x] TASK-RES-01 Verify current SHA, unchanged tracked worktree/76 preserved maintenance files, no known task preview worker, old production healthy/schema17, CI38793153064 sameSHA success/artifact and publish skipped. New per-source controlled fetch evidence, current subscription race13pass; retain prior FAILED143 and earlier race timeout, no sender invented. Evidence `subscription/deliverables.json` and `release0700/deliverables.json` under current evidence root.
- [x] TASK-RES-02 Implement FR-RES-01 in cmd/csp shutdown path/tests and FR-RES-02 Dockerfile/version regression/docs; substantive targeted native/race/build version tests. No UI/asset changes.
- [ ] TASK-RES-03 Bounded strict-TLS direct/path comparison for two failed sources in isolated copy, no production writes/proxy changes/secrets; report stage and actual successes/failures without changing upstream identity.
- [x] TASK-RES-04 Independent final delta review including old CI metadata fix, DB-lifetime behavior, security/asset-preservation evidence eligibility, exact code/asset manifests, no weakened tests.
- [x] TASK-RES-05 Prepare final healthy synthetic isolated instance with both coordinators disabled, standard instance/source/cleanup identity; final critic actual settings/policy flows + PNG read, explicitly not physical-phone/production acceptance. Clean only owned preview.
- [ ] TASK-RES-06 Scoped git_commit after review/critic, non-force push reviewed branch, record exactSHA cloud build result. No unreviewed merge or default-branch overwrite.
- [ ] TASK-RES-07 Official private-image/digest gate and remote-ancestry integration, production online backup/rollout/94-operation coverage only when authenticated owner preconditions met. Until then report BLOCKED, not PASS. User notified delivery609c21a7875c4e3b8e1888393125f9be.
- [ ] TASK-RES-08 Update project experience and final Chinese QQ delivery, preserve auto_test/ambiguous recovery/TLS/production limitations and verified cleanup. CPA out of scope.

### Live visibility amendment tasks

Latest “没必要私有啊” supersedes private-image portion of TASK-RES-07 and older TASK-RELEASE/PROD private gates, not quality/backup/production safety. Earlier QQ delivery609c21... is historical, now corrected.

- [x] TASK-RES-PUB-01 Update existing `.github/workflows/ci.yml`, native cloud tests and operational docs for public-allowed exact-SHA release. Keep least privilege, build-once artifact, explicit release triggers, checksum/ELF/version/OCI digest verification and actual registry push/read; stop calling private visibility/API/OWNER_PAT gate. Preserve legacy private-gate script only as unused historical tool if tests still intentionally cover it; never execute its now-revoked visibility policy.
- [ ] TASK-RES-PUB-02 Independently review final combined shutdown/metadata/public-release delta, commit via tool, normal push dedicated release ref after all applicable gates, obtain actual cloud checks/publish/digest/run artifacts. Actual registry write/read proves permission, not Git dry-run/scopes.
- [ ] TASK-RES-PUB-03 Verify exact cloud image in isolated synthetic instance then scoped production hot backup/authorized official image-only deployment and actual business acceptance/cleanup. User private-policy change does not expose business data or bypass this step.

## Second interrupted-run recovery (2026-10-16 08:53)

Current frozen run `pi_run_20261016_085304_1567276`, change002-policy/default/epoch1; prior run pi_run_20261016_074108_1448746 FAILED143 retained. Last reviewer provider HTTP500 has no verdict; exact process termination sender/cause unverified. Same models retained, no gateway changes. Worktree/11-codefile delta and synthetic preview46623 retained; main brain assumes verified cleanup ownership from prior run (PID1548536/instance marker). Prior local selftest report4c756583 eligible only after manifest/source match; cmd native33/race33/cloud23/CI26/vet/build all0. New reviewer must produce current-contract report; old failed provider call is not a gate PASS. Current tasks continue without restarting implementation or restoring cancelled node-count work. Public-image override remains authoritative; no private gate returns. CPA/phone out of scope.

### Dev → prelease amendment

- [x] TASK-DEV-01 Update current CI/docs/tests for dev cloud test/build and only-after-success GitHub prerelease `prelease-<tested-shortSHA>` with exact same binary/checksum/image digest; no server heavy compile, no production auto-switch. Independent review of changed orchestration required, unchanged UI critic9PNG may be reused after hash match.
- [ ] TASK-DEV-02 Commit scoped reviewed changes, normal isolated merge current origin/dev preserving both histories; independently review any conflicts/new functional delta; commit merge via tool then normal non-force push dev. No replacing dirty worktree or committing76maintenance.
- [ ] TASK-DEV-03 Obtain exact devSHA Actions test/build/image push/pull/OCI outcome and GitHub prerelease correspondence. Failed steps remain failures, no promotion until passed. No production deployment inferred by this instruction.

## Current evidence update (2026-10-16 dev-prelease)

CI synthetic 33 tests pass exit0 (PYTHONPATH=.github/scripts confirmed); cloud-delivery 22+1=23 tests; Go cmd/csp native+race pass. promote_prelease.py AST valid, all subprocess.run with shell=False, no token/secret print, immutable-reuse/orphan/digest-mismatch/fail-closed all unit-covered. prelease job: dev push only, contents:write isolated, prelease-* tag no v* loop. Image package csp-runtime-private is historical name authorized as public (user: "没必要私有啊"); publish remains explicit trigger. Independent reviewers: e53c PASS (shutdown+build metadata), b4a1 PASS (dev→prelease pipeline). Critic PASS 280ef (9 native PNGs read). No commit/push/release/production executed. Preview PID1548536 port46623 cleanup remains main brain owner.

## Current Engineering Tasks (2026-10-16 Run pi_run_20261016_104303_1679144)

- [x] TASK-ENH-01 (Proxy DNS Fix): In `internal/fetch/fetch.go`, do not override `hopReq.URL.Host` with `pinnedAddr` when `activeProxyURL != nil`; preserve original domain in `hopReq.URL.Host` so proxy performs remote DNS resolution. Align `internal/fetch/route_diagnostic_test.go` wire semantics assertion.
- [x] TASK-ENH-02 (Subscription Prefix): Add single authoritative top-level `prefix` to `Subscription` domain model; add migration `000021_subscription_prefix.sql`; implement prefix prepending on node extraction in `internal/application/inventory/service.go` and retroactive prefix renaming for existing nodes in `internal/application/inventory/` and `internal/application/subscription/`.
- [x] TASK-ENH-03 (Manual Refresh for Disabled Subs): Allow manual refresh on disabled subscriptions in `internal/application/inventory/service.go` and `internal/application/subscription/service.go` while keeping cron scheduler restricted to enabled subscriptions; update affected unit tests.
- [x] TASK-ENH-04 (WARP Node & Manual Protection; restoration audit exception retained): Restore `WARP-83d9` (`node_9f2515f807595e984f6081ac58d244bd`) into SQLite; update `enabledSubscriptionsInventorySubquery` in `internal/repository/sqlite/node_predicates.go` to include manual/static nodes (true manual nodes without subscription-origin evidence, not all no-live-source nodes); protect manual nodes from deactivation in `deactivateOrphansSQL`.
- [x] TASK-ENH-05 (Frontend Subscription Polish): In `SubscriptionConfigDrawer.vue`, add node prefix input field; in `SubscriptionsView.vue`, add visible refresh button/status with "正在刷新" spinner and disabled button state during active refresh; support manual refresh on disabled subscriptions.
- [x] TASK-ENH-06 (Local Verification & Self-Test): Use lightweight targeted verification locally and full Go/web/build checks on dev CI, isolated fixtures only.
- [ ] TASK-ENH-07 (Review & Promotion): Independent review, critic verification, Git commit via tool, push to `dev`, and verify GitHub Actions build and promotion to `prelease-<shortSHA>`.

### Review failure and targeted repair (same feature/run)
- [x] TASK-ENH-R01 Fix atomic subscription/prefix persistence, remove duplicate config authority, fail/rollback injections and stale-save tests. Trace FR-POL-11; review P0-01/P1-01.
- [x] TASK-ENH-R02 Implement deterministic shared prefix labels without source raw-name/identity/connection revision or user-override corruption; import/save/remove/clear tests. Trace FR-POL-11; P1-02.
- [x] TASK-ENH-R03 Exclude refresh-pruned subscription assets from manual branch; preserve real manual WARP/scope/delete/shared semantics and assert policy/export. Trace FR-POL-14; P0-02.
- [x] TASK-ENH-R04 Rename proxy wire tests and document remote resolver trust, add new UI regression tests/i18n/responsive flow; no phone. Trace FR-POL-10/12; P2-01/02.
- [ ] TASK-ENH-R05 Independent re-review names rejected report, scoped commit and dev CI exact-SHA tests/build/prerelease, artifact-based isolated preview critic, preserve 76 baseline maintenance files. Capture WARP operation audit without further production writes; never seed credentials.

Current status: implementation/targeted self-test and independent re-review pass (IDs in plan). TASK-ENH-06 covers lightweight current source evidence; full cloud tests/build remain TASK-ENH-07/R05, not yet passed. WARP active/node/head/version read-back successful, write-before-backup process exception is unresolved and not a safe-production-rollout PASS. No further prod writes permitted in this continuation.

## Current branch governance tasks — run pi_run_20261016_134459_1816216

- [x] TASK-GOV-01 Read-only current ref/PR/CI/protection/worktree/old-gate evidence, protect80untracked+3ignored files; scout evidence directory linked in plan. FR-GOV01..05.
- [ ] TASK-GOV-02 Update `.github/workflows/ci.yml`, `.github/dependabot.yml`, existing synthetic/cloud tests and `docs/operations/csp-cloud-delivery.md` to dev-only build/publish, current Dependabot targetdev, exact-SHA main promotion/no retired-ref triggers/no path-filter check deadlock. FR-GOV01/02/04.
- [ ] TASK-GOV-03 Executor lightweight Python/config self-check, independent source/release-safety reviewer, scoped commit tool + normal devpush, exact-SHA full cloud checks/image/prerelease; no host compilation. FR-GOV02/05.
- [ ] TASK-GOV-04 Verify reviewed dev cloudSHA and current ancestry; promote main normal FF exactSHA without rebuilding/production switch. Snapshot all8obsolete refs into private bundle; review exact mutation eligibility. FR-GOV02/03.
- [ ] TASK-GOV-05 Authorized minimal main/dev branch protections, verify delete/force disabled/check semantics; only then enable auto-delete merged branches. If current PAT denied, preserve403 and report Administration write blocker (no alternate credentials). FR-GOV04/05.
- [ ] TASK-GOV-06 Close oldPR19..23 with reason; recheck exact refs/ancestry/active use then delete8obsolete remote refs with oldSHA lease; preserve all localbranches/worktree/ignored/untracked hashes. FR-GOV03/05.
- [ ] TASK-GOV-07 Independent final post-state audit+required gates, task ledger honest partial status, final QQ delivery receipts. No changedUI→criticN/A. FR-GOV01..05.

### Governance review remediation
- [ ] TASK-GOV-R01 Rejected reviewer `rep_rev_default_c3bad93c-8f5b-42c0-af50-aeffa43af882`: gate test-and-build against nondev workflow_dispatch, add positive/negative semantic tests in both existing suites; use bounded AST boolean evaluation instead of arbitrary eval (empty eval globals are NOT a sandbox). Independent re-review required before commit.

### Governance continuation binding (2026-10-16 15:02)
Current run `pi_run_20261016_150206_1862343`, change `002-policy`, package `default`, epoch1. Continue FR-GOV01..05 and TASK-GOV/R01 only; previous run identity/rejection remain history, not current gate evidence. Previous repair resume returned no result; current diff still lacks job guard and contains eval. No commit/remote mutation yet. Fresh executor and independent reviewer provide current-contract evidence. Source scope unchanged, no production/local Go or web compile. All80maintenance/oldworktree preserved. Permissions blocker remains to verify with actual authorized write, no credential change.
