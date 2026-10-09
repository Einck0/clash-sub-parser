# Implementation Tasks: Auth Capability

- **Feature Directory**: `specs/005-auth/`
- **Capability Domain**: `auth`
- **Status**: CANDIDATE

## 1. Specification Migration & Governance Tasks (Migration-Only)

- [x] **TASK-MIG-01**: 从历史 OpenSpec 规格提炼并建立 Spec Kit 原生主干能力规范 (`specs/005-auth/spec.md`)
- [x] **TASK-MIG-02**: 编制架构方案与已知/未知事实台账 (`specs/005-auth/plan.md`)
- [x] **TASK-MIG-03**: 建立对应功能的历史需求条目映射与证据锚点 (`docs/specification/migration-ledger.json`)
- [x] **TASK-MIG-04**: 保持既有代码符合度审计结果客观不变（不擅自伪造测试通过）

## 2. Business Implementation Tasks (Pending User Clarification & Future Sprints)

> **注意**：以下任务属于业务功能修复或待决问题落地，保持未完成 (`- [ ]`) 状态，绝不带病勾选或降级不实现。
- [ ] **TASK-BIZ-01**: 控制台新增明文存储安全风险提示 Banner
- [ ] **TASK-BIZ-02**: 支持多管理员独立凭据审计（可选待办）
