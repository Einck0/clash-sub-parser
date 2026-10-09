## Why

当前 `clash-sub-parser`（CSP）项目在演进中积累了 26 个历史 OpenSpec 变更集和 63 份分项规格，但根目录持续有效的主干能力规范（`openspec/specs/`）长期为空，导致全系统缺乏单一的权威规范事实源（Single Source of Truth）。历史提案中的问题分析有时被误当成用户原话，且部分业务行为（如策略组解包展开、成员混合交错顺序、历史空组数据恢复、非 Mihomo 目标空组处理等）存在实现缺口或未决歧义。

用户在本轮会话中明确指示：
> “按你的建议来，重写当前的SPC，再根据规格看当前是否符合，模糊整合询问我，然后再落地”

因此，本变更集作为**纯文档与规范特性包（Documentation Only）**，负责整合并重写全系统 Living Specification，完成客观代码符合度审计，编制待决业务问题集供用户裁决，为后续业务代码落地筑牢契约根基。

## What Changes

1. **建立 6 大业务领域主干能力规范 (Living Specs)**：
   - 在 `openspec/specs/` 下建立 `inventory`、`policy`、`probe`、`publication`、`auth`、`ui` 规范体系；
   - 吸收 GitHub Spec-Kit 结构化范式（优先级场景 User Scenarios、稳定需求编号 FR-ID、可度量标准 SC-ID），适配 OpenSpec 严格语法并通过 `openspec validate --specs --strict` 校验。
2. **导入官方模板参考存根与架构决策记录**：
   - 在 `.specify/templates/` 存放 Spec-Kit 官方模板副本及 provenance 哈希记录，声明保留 repo-local `openspec` CLI 作为执行框架；
   - 在 `docs/decisions/proposed/architecture/` 编写 CSP 架构决策记录，客观记录 `proposed` 状态与被否决方案（`.agents/notes/README.md` 仅保留跳转索引）；
3. **输出多维度工程审计文档与待决问题清单**：
   - `docs/specification/source-register.md`：建立四级来源分级体系，严格甄别用户原话；
   - `docs/specification/conformance-matrix.md`：建立 37 项功能需求的五态符合度审计矩阵，附带精准实机代码行与测试退出码；
   - `docs/specification/clarification-questions.md`：汇总 6 组核心业务待决问题，列出分项选项，绝不自行预设默认值；
   - `docs/specification/upstream-provenance-and-coverage.md`：逐一映射 26 个既有变更集与 63 份历史规格，确保契约零遗失；
   - `docs/specification/maintenance-guide.md`：建立长期规范维护与演进指南。

## Capabilities

### New Capabilities

- `living-spec-consolidation`: 建立项目持续主干能力规范管理体系，包含 6 大业务领域 Living Spec、四级来源确权、实机符合度审计矩阵及跨变更覆盖追溯能力。

### Modified Capabilities

- 无。本变更集保留全部既有 26 个变更集，不作归档或删除。

## Impact

- **代码与运行时**：零修改。本阶段不改动任何 Go 后端代码、Vue 前端代码、SQL 迁移脚本或 Docker 部署配置，不触碰正在健康运行的生产容器。
- **文档与治理**：新增 `openspec/specs/` 主干规范、`.specify/templates/` 存根、`docs/decisions/` 决策记录及 `docs/specification/` 系列审计文档。
- **流程状态**：本变更完成文档编制与本地自测后，流转独立审查，业务代码落地需待用户对 6 组问题裁决后另行实施。
