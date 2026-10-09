# Technical Plan: Inventory Capability

- **Feature Directory**: `specs/001-inventory/`
- **Capability Domain**: `inventory`
- **Status**: CANDIDATE (Spec Kit Native Architecture)

## 1. Architecture Overview

本能力负责 订阅输入、节点传输身份、写时复制参数覆写与资产零丢失保全。在架构分层中，该领域保持与其他五大能力的高度解耦，通过领域模型与数据库持久化事实进行交互。

## 2. Dependencies & Data Flow

- **数据库表**: 依赖相关数据库契约（详见 `conformance-matrix.md` 精确映射）。
- **内部包**: Go `internal/inventory`（或相关解析/控制器包）。
- **上游依赖**: 依赖配置与上下文输入，下游提供规范化读取与导出契约。

## 3. Knowns & Unknowns (已知与未知架构事实)

### Knowns (已验证事实)
- 支持 Shadowsocks, VMess, VLESS, Trojan, Hysteria2, TUIC, WireGuard 协议解析
- 基于协议、服务器、端口及连接参数计算传输哈希 logical_id
- 在 nodes 表明文存储连接信息，node_sources 记录来源映射，overrides 记录人工覆写

### Unknowns & Retained Blockers (保留待决事项)
- Q5 待决问题：订阅刷新成功但部分节点缺失时，来源修剪与隐藏作用域保护策略（保守保留 vs 立即修剪）

## 4. Conformance & Verification Baseline

- **符合度现状**: 详见 `docs/specification/conformance-matrix.md`；
- **历史需求追溯**: 详见 `docs/specification/historical-requirement-register.md`；
- **测试证据原则**: 本迁移计划不虚构新执行的测试，所有历史测试退出码与证据均标明出处与时效性。
