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
- Q5 中部分成功的来源成员策略已由用户后续明确：应用本次成功更新集合，完全失败保留上次已应用集合。提取有结果但管理员过滤后全空时，是应用空成员还是保留上次成员仍待业务确认；包括部分解析失败且成功提取项均被过滤的情况。详见 [spec.md](spec.md) 末尾。

## 4. Conformance & Verification Baseline

- **符合度现状**: 详见 `docs/specification/conformance-matrix.md`；
- **历史需求追溯**: 详见 `docs/specification/historical-requirement-register.md`；
- **测试证据原则**: 本迁移计划不虚构新执行的测试，所有历史测试退出码与证据均标明出处与时效性。


## 原始载荷整改历史及当前目标

[2026-10-25 方案](payload-retirement-20261025.md) 是历史证据。用户后续要求每次拉的配置只保留最新一份；独立单槽保存最新成功或部分成功原文、只保留当前代 payload 与条目是现有技术契约，详见 [010 原文契约](../010-export-adaptation/original-config-20261110.md) 和 [010 FR008](../010-export-adaptation/spec.md)，不将这些具体生命周期细节归为用户原话。过滤全空处理仍待决。这些目标不是云测通过或生产迁移的证明；生产执行仍须本轮授权。
