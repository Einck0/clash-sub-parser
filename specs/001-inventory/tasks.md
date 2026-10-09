# Implementation Tasks: Inventory Capability

- **Feature Directory**: `specs/001-inventory/`
- **Capability Domain**: `inventory`
- **Status**: CANDIDATE

## 1. Specification Migration & Governance Tasks (Migration-Only)

- [x] **TASK-MIG-01**: 从历史 OpenSpec 规格提炼并建立 Spec Kit 原生主干能力规范 (`specs/001-inventory/spec.md`)
- [x] **TASK-MIG-02**: 编制架构方案与已知/未知事实台账 (`specs/001-inventory/plan.md`)
- [x] **TASK-MIG-03**: 建立对应功能的历史需求条目映射与证据锚点 (`docs/specification/migration-ledger.json`)
- [x] **TASK-MIG-04**: 保持既有代码符合度审计结果客观不变（不擅自伪造测试通过）

## 2. Business Implementation Tasks (Pending User Clarification & Future Sprints)

> **注意**：以下任务属于业务功能修复或待决问题落地，保持未完成 (`- [ ]`) 状态，绝不带病勾选或降级不实现。
- [ ] **TASK-BIZ-01**: 根据用户对 Q5 的决策落实订阅刷新来源修剪与隐藏保护逻辑
- [ ] **TASK-BIZ-02**: 优化大规模（1000+）节点导入时的批处理事务吞吐量
