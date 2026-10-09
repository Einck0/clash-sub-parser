# Implementation Tasks: Probe Capability

- **Feature Directory**: `specs/003-probe/`
- **Capability Domain**: `probe`
- **Status**: CANDIDATE

## 1. Specification Migration & Governance Tasks (Migration-Only)

- [x] **TASK-MIG-01**: 从历史 OpenSpec 规格提炼并建立 Spec Kit 原生主干能力规范 (`specs/003-probe/spec.md`)
- [x] **TASK-MIG-02**: 编制架构方案与已知/未知事实台账 (`specs/003-probe/plan.md`)
- [x] **TASK-MIG-03**: 建立对应功能的历史需求条目映射与证据锚点 (`docs/specification/migration-ledger.json`)
- [x] **TASK-MIG-04**: 保持既有代码符合度审计结果客观不变（不擅自伪造测试通过）

## 2. Business Implementation Tasks (Pending User Clarification & Future Sprints)

> **注意**：以下任务属于业务功能修复或待决问题落地，保持未完成 (`- [ ]`) 状态，绝不带病勾选或降级不实现。
- [ ] **TASK-BIZ-01**: 监控 1 分钟协调器探针在高负载下的系统资源消耗
- [ ] **TASK-BIZ-02**: 完善辅助 Sing-box 探针构造器的协议覆盖
