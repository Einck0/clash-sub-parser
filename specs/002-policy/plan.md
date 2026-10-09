# Technical Plan: Policy Capability

- **Feature Directory**: `specs/002-policy/`
- **Capability Domain**: `policy`
- **Status**: CANDIDATE (Spec Kit Native Architecture)

## 1. Architecture Overview

本能力负责 策略组拓扑、双模式成员嵌套引用、动态全池匹配与空组安全兜底。在架构分层中，该领域保持与其他五大能力的高度解耦，通过领域模型与数据库持久化事实进行交互。

## 2. Dependencies & Data Flow

- **数据库表**: 依赖相关数据库契约（详见 `conformance-matrix.md` 精确映射）。
- **内部包**: Go `internal/policy`（或相关解析/控制器包）。
- **上游依赖**: 依赖配置与上下文输入，下游提供规范化读取与导出契约。

## 3. Knowns & Unknowns (已知与未知架构事实)

### Knowns (已验证事实)
- 图解析器支持显式节点、按国家/正则动态规则筛选以及直接组引用 (Direct Group Reference)
- 空组支持 empty_fallback_pass: PASS 与 REJECT 安全兜底

### Unknowns & Retained Blockers (保留待决事项)
- FR-POL-02 解包叶节点 (Unpack / Flatten Leaf Nodes) 代码实现完全缺失 (代码不符合)
- FR-POL-04 混合交错顺序 (Mixed Interleaved Order) 解析器硬编码先显式节点后子组，破坏用户编排顺序
- Q1 待决问题：解包模式叶节点去重与递归循环保护策略
- Q2 待决问题：混合顺序严格交错保序要求
- Q4 待决问题：历史遗留空组（0 显式节点 0 规则）数据迁移恢复规则

## 4. Conformance & Verification Baseline

- **符合度现状**: 详见 `docs/specification/conformance-matrix.md`；
- **历史需求追溯**: 详见 `docs/specification/historical-requirement-register.md`；
- **测试证据原则**: 本迁移计划不虚构新执行的测试，所有历史测试退出码与证据均标明出处与时效性。
