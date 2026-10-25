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


## 原始载荷整改增量设计（2026-10-25）

冻结方案、模块边界、公共接口、决策依据及验证见 [payload-retirement-20261025.md](payload-retirement-20261025.md)。复用现有 schema/ID 与结构化节点事实，body_blob 固定空 BLOB；历史批量清除走独立离线维护 CLI，不在启动迁移自动执行。Inventory、SQLite、CLI 三模块独立并行，主脑汇聚云测与隔离证据。

Constitution check：失败刷新资产保全、逻辑身份、凭据与领域分层均不变；无新增密文/中间件；活规范已同步。技术栈沿用 Go/SQLite；所有编译与测试在现有 dev Actions，宿主只能静态检查和运行云镜像。无未决业务需求，生产执行仍须新授权。
