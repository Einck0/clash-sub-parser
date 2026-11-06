# Implementation Tasks: Probe Capability

- **Feature Directory**: `specs/003-probe/`
- **Capability Domain**: `probe`
- **Status**: CANDIDATE

## 1. Specification Migration & Governance Tasks (Migration-Only)

- [x] **TASK-MIG-02**: 编制架构方案与已知/未知事实台账 (`specs/003-probe/plan.md`)
- [x] **TASK-MIG-03**: 建立对应功能的历史需求条目映射与证据锚点 (`docs/specification/migration-ledger.json`)
- [x] **TASK-MIG-04**: 保持既有代码符合度审计结果客观不变（不擅自伪造测试通过）

## 2. Business Implementation Tasks (Pending User Clarification & Future Sprints)

> **注意**：以下任务属于业务功能修复或待决问题落地，保持未完成 (`- [ ]`) 状态，绝不带病勾选或降级不实现。
- [ ] **TASK-BIZ-01**: 监控 1 分钟协调器探针在高负载下的系统资源消耗
- [ ] **TASK-BIZ-02**: 完善辅助 Sing-box 探针构造器的协议覆盖


## 3. Resumed optimization — 2026-11-06

Contract and scenarios: [probe-optimization-20261106.md](probe-optimization-20261106.md). Historical pending tasks above remain separate and unchecked.

- [ ] **OPT-01** Move stage admission before scheduler submission with bounded cancellation-safe permits; add concurrency/failure/callback regressions in `internal/application/probe/`.
- [ ] **OPT-02** [P] Enforce per-run caps and fair manual dispatch in `internal/probe/queue/`, preserving deduplication and sole-run borrowing. Same execution owner as OPT-01.
- [ ] **OPT-03** [P] Tighten Gemini evidence, safe typed failures, redirect/region/body bounds and tests in `internal/probe/platform/`.
- [ ] **OPT-04** [P] Add independent anonymous Antigravity reachable/auth-unverified detector and negative-path tests in `internal/probe/platform/`. Same platform owner as OPT-03.
- [ ] **OPT-05** Integrate separate antigravity AI platform map result with selection, session and budget reuse; regression in `internal/application/probe/`. Execution owner, frozen CheckAntigravity API.
- [ ] **OPT-06** Master checks exact allowed diff, commits/pushes dev; existing Actions mandatory jobs + same-SHA OCI image and isolated fresh-DB runtime acceptance; executor validates cloud evidence. No local compilation/tests.
- [ ] **OPT-07** Independent reviewer validates final same-SHA source/evidence and risks; resolve substantive findings, then master closes task/QQ. No production deployment.
