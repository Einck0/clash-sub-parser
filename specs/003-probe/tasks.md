# Implementation Tasks: Probe Capability

- **Feature Directory**: `specs/003-probe/`
- **Capability Domain**: `probe`
- **Status**: CANDIDATE (historical capability-wide backlog retained)

## 1. Specification Migration & Governance Tasks (Migration-Only)

- [x] **TASK-MIG-02**: 编制架构方案与已知/未知事实台账 (`specs/003-probe/plan.md`)
- [x] **TASK-MIG-03**: 建立对应功能的历史需求条目映射与证据锚点 (`docs/specification/migration-ledger.json`)
- [x] **TASK-MIG-04**: 保持既有代码符合度审计结果客观不变（不擅自伪造测试通过）

## 2. Business Implementation Tasks (Pending User Clarification & Future Sprints)

> 以下历史任务不属于本轮已验收范围，保留未完成状态。
- [ ] **TASK-BIZ-01**: 监控 1 分钟协调器探针在高负载下的系统资源消耗
- [ ] **TASK-BIZ-02**: 完善辅助 Sing-box 探针构造器的协议覆盖

## 3. Resumed optimization — 2026-11-06

Contract/scenarios: [probe-optimization-20261106.md](probe-optimization-20261106.md). Implementation source SHA `b2e719fac742aede33057d02f41c3ba1a2fbd393`, cloud run [42188668376](https://github.com/Einck0/clash-sub-parser/actions/runs/42188668376), mandatory jobs success and dev release skipped. Isolated runtime image digest `sha256:542910886193b6367fc47323ca193d8cc18c64b13e5f18b6bd53a2b5197c612e`, OCI revision matches source. Final documentation-only commit is separately rechecked by the same workflow before terminal delivery.

- [x] **OPT-01** Move stage admission before scheduler submission with cancellation-safe permits and admission/cancellation/submission-failure regression tests in `internal/application/probe/`.
- [x] **OPT-02** Enforce per-run caps and fair manual dispatch in `internal/probe/queue/`, preserving deduplication and sole-run borrowing.
- [x] **OPT-03** Tighten Gemini supported-region evidence, typed failures, redirect/gate/region/body bounds and tests in `internal/probe/platform/`.
- [x] **OPT-04** Add independent anonymous Antigravity reachable/auth-unverified detector and negative-path tests in `internal/probe/platform/`.
- [x] **OPT-05** Integrate separate antigravity AI map result with selection, session and budget reuse; explicit Gemini-only regression in `internal/application/probe/`.
- [x] **OPT-06** Task-scoped dev commits pushed; same-SHA cloud required checks/build passed; prebuilt fresh-DB runtime version, health/readiness, authenticated empty probe APIs and unauthenticated 401 verified. Own container/volume removed. Executor report `a98350d4-4f38-4639-8f6b-cb893af9d6d7` supersedes pending-cloud reports. No local compile/tests.
- [x] **OPT-07** Independent reviewer `rep_rev_whole_task_1b73f156-62c2-4b3b-96aa-0d8e8c0cf6b8` PASS, no blocking findings. Reviewer recommendation to maintain dated official supported-region snapshot is accepted as future maintenance, not a new service/configuration requirement. No production deployment.

## 4. Validation history and limitations

Initial source commit `c1a5f8f7409cad084735d9ed27f3984ea5c48050` failed cloud run `42187663184`: duplicate test idempotency key, pre-Antigravity request-count assertions, and transport fixture incorrectly expecting unknown. Test-only correction `b2e719f` repaired them without production compatibility code; failure history is preserved.

Deterministic scenarios cover stage limit 1 allowing unrelated work, cancellation while waiting, failed submission drain, per-run manual caps and client reuse. No live-provider accuracy percentage or throughput improvement percentage is claimed. Antigravity result is network-entry evidence only, not account/product eligibility; no authenticated model-call experiment was authorized or performed. No frontend badge redesign, production rollout, or unrelated worktree cleanup. `graphify update .` exited 127 because graphify is not installed; no graph success is claimed. Spec Kit prerequisite/analyze used existing probe feature with explicit `SPECIFY_FEATURE_DIRECTORY=specs/003-probe`, no template regeneration. Master verified root `.exp.md` exists and was read; helper statements that no module-local experience file was found do not imply the root file is absent.
