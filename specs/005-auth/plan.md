# Technical Plan: Auth Capability

- **Feature Directory**: `specs/005-auth/`
- **Capability Domain**: `auth`
- **Status**: CANDIDATE (Spec Kit Native Architecture)

## 1. Architecture Overview

本能力负责 控制台会话认证、系统主令牌配置与明文敏感信息存储模型。在架构分层中，该领域保持与其他五大能力的高度解耦，通过领域模型与数据库持久化事实进行交互。

## 2. Dependencies & Data Flow

- **数据库表**: 依赖相关数据库契约（详见 `conformance-matrix.md` 精确映射）。
- **内部包**: Go `internal/auth`（或相关解析/控制器包）。
- **上游依赖**: 依赖配置与上下文输入，下游提供规范化读取与导出契约。

## 3. Knowns & Unknowns (已知与未知架构事实)

### Knowns (已验证事实)
- 管理端采用 HttpOnly Session Cookie 鉴权，支持会话超时与登出失效
- 遵照用户原话，节点凭据采用明文直接持久化在 nodes 表，不引入多余加密库
- 管理端与导出链接具备独立鉴权开关

### Unknowns & Retained Blockers (保留待决事项)
- 历史加密相关规范（AES-GCM/Vault）确认为历史废弃冲突，保持存档

## 4. Conformance & Verification Baseline

- **符合度现状**: 详见 `docs/specification/conformance-matrix.md`；
- **历史需求追溯**: 详见 `docs/specification/historical-requirement-register.md`；
- **测试证据原则**: 本迁移计划不虚构新执行的测试，所有历史测试退出码与证据均标明出处与时效性。
