# Technical Plan: UI Capability

- **Feature Directory**: `specs/006-ui/`
- **Capability Domain**: `ui`
- **Status**: CANDIDATE (Spec Kit Native Architecture)

## 1. Architecture Overview

本能力负责 紧凑布局响应式管理面板、状态联动服务端分页与抽屉式工作台。在架构分层中，该领域保持与其他五大能力的高度解耦，通过领域模型与数据库持久化事实进行交互。

## 2. Dependencies & Data Flow

- **数据库表**: 依赖相关数据库契约（详见 `conformance-matrix.md` 精确映射）。
- **内部包**: Go `internal/ui`（或相关解析/控制器包）。
- **上游依赖**: 依赖配置与上下文输入，下游提供规范化读取与导出契约。

## 3. Knowns & Unknowns (已知与未知架构事实)

### Knowns (已验证事实)
- 采用现代流式布局适配桌面与移动端视口，杜绝无来源硬编码尺寸
- 策略组与节点覆写采用抽屉式（Drawer）详情工作台，支持中英双语国际化 (zh-CN / en-US)

### Unknowns & Retained Blockers (保留待决事项)
- Q6 待决问题：节点健康状态组合筛选（存活+超时+未探测）与服务端全量分页联动行为

## 4. Conformance & Verification Baseline

- **符合度现状**: 详见 `docs/specification/conformance-matrix.md`；
- **历史需求追溯**: 详见 `docs/specification/historical-requirement-register.md`；
- **测试证据原则**: 本迁移计划不虚构新执行的测试，所有历史测试退出码与证据均标明出处与时效性。
