# Implementation Tasks: Policy Capability

- **Feature Directory**: `specs/002-policy/`
- **Capability Domain**: `policy`
- **Status**: CANDIDATE

## 1. Specification Migration & Governance Tasks (Migration-Only)

- [x] **TASK-MIG-01**: 从历史 OpenSpec 规格提炼并建立 Spec Kit 原生主干能力规范 (`specs/002-policy/spec.md`)
- [x] **TASK-MIG-02**: 编制架构方案与已知/未知事实台账 (`specs/002-policy/plan.md`)
- [x] **TASK-MIG-03**: 建立对应功能的历史需求条目映射与证据锚点 (`docs/specification/migration-ledger.json`)
- [x] **TASK-MIG-04**: 保持既有代码符合度审计结果客观不变（不擅自伪造测试通过）

## 2. Business Implementation Tasks (Pending User Clarification & Future Sprints)

> **注意**：以下任务属于业务功能修复或待决问题落地，保持未完成 (`- [ ]`) 状态，绝不带病勾选或降级不实现。
- [ ] **TASK-BIZ-01**: 实现 FR-POL-02 策略组解包叶节点 (Unpack Leaf Nodes) 展开引擎与 DAG 循环检测
- [ ] **TASK-BIZ-02**: 重构 internal/resolver/graph.go 修复 FR-POL-04 混合交错保序
- [ ] **TASK-BIZ-03**: 根据 Q4 裁决实施历史空策略组数据迁移脚本
