# Implementation Tasks: UI Capability

- **Feature Directory**: `specs/006-ui/`
- **Capability Domain**: `ui`
- **Status**: CANDIDATE

## 1. Specification Migration & Governance Tasks (Migration-Only)

- [x] **TASK-MIG-01**: 从历史 OpenSpec 规格提炼并建立 Spec Kit 原生主干能力规范 (`specs/006-ui/spec.md`)
- [x] **TASK-MIG-02**: 编制架构方案与已知/未知事实台账 (`specs/006-ui/plan.md`)
- [x] **TASK-MIG-03**: 建立对应功能的历史需求条目映射与证据锚点 (`docs/specification/migration-ledger.json`)
- [x] **TASK-MIG-04**: 保持既有代码符合度审计结果客观不变（不擅自伪造测试通过）

## 2. Business Implementation Tasks (Pending User Clarification & Future Sprints)

> **注意**：以下任务属于业务功能修复或待决问题落地，保持未完成 (`- [ ]`) 状态，绝不带病勾选或降级不实现。
- [ ] **TASK-BIZ-01**: 根据 Q6 裁决完善健康状态筛选的多选组合与分页重置交互
- [ ] **TASK-BIZ-02**: 优化千万级节点数据表格的虚拟滚动渲染性能
