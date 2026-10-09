# Technical Plan: Publication Capability

- **Feature Directory**: `specs/004-publication/`
- **Capability Domain**: `publication`
- **Status**: CANDIDATE (Spec Kit Native Architecture)

## 1. Architecture Overview

本能力负责 原生 Mihomo 完整拓扑发布、非 Mihomo 纯节点导出与免密共享路由。在架构分层中，该领域保持与其他五大能力的高度解耦，通过领域模型与数据库持久化事实进行交互。

## 2. Dependencies & Data Flow

- **数据库表**: 依赖相关数据库契约（详见 `conformance-matrix.md` 精确映射）。
- **内部包**: Go `internal/publication`（或相关解析/控制器包）。
- **上游依赖**: 依赖配置与上下文输入，下游提供规范化读取与导出契约。

## 3. Knowns & Unknowns (已知与未知架构事实)

### Knowns (已验证事实)
- Mihomo 导出输出完整 proxy-providers、proxy-groups、rules 结构
- 非 Mihomo 导出（Clash 传统、Base64 等）输出纯节点列表
- 支持专属 Token 与系统主令牌双重访问控制，支持历史 /p/{id} 兼容路由

### Unknowns & Retained Blockers (保留待决事项)
- Q3 待决问题：非 Mihomo 导出在遇到引用了空策略组的规则时，空组过滤与行为一致性处理

## 4. Conformance & Verification Baseline

- **符合度现状**: 详见 `docs/specification/conformance-matrix.md`；
- **历史需求追溯**: 详见 `docs/specification/historical-requirement-register.md`；
- **测试证据原则**: 本迁移计划不虚构新执行的测试，所有历史测试退出码与证据均标明出处与时效性。
