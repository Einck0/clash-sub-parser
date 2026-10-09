# Agent Note: Living Specification 架构整合与规范分层

- Status: proposed (Phase: Documents & Specification Consolidation)
- Date: 2026-10-09
- Author: executor
- Scope: clash-sub-parser specification architecture and governance harmonization

## 背景与上下文

`clash-sub-parser`（CSP）项目在历史迭代中积累了 26 个 OpenSpec 变更集（Change）与 63 份分项规格，但根目录 `openspec/specs/` 一直为空，缺乏统一的持续有效主干能力契约（Living Specification）。各变更集之间的规格分散，部分历史假设、临时修复与实际业务代码之间存在偏差。

用户在本轮会话中明确指示：
> “按你的建议来，重写当前的SPC，再根据规格看当前是否符合，模糊整合询问我，然后再落地”

本决策记录旨在梳理本次规范整合的架构设计、设计依据及否决的备选方案。

## 决策内容

1. **采用 SpecKit 规范结构融入 repo-local OpenSpec 契约**：
   - 在项目内建立 6 大核心业务领域的主干能力规范（`inventory`、`policy`、`probe`、`publication`、`auth`、`ui`），存放于 `openspec/specs/<domain>/spec.md`。
   - 吸收 GitHub Spec-Kit 核心结构（优先级用户场景 User Story P1~P3、稳定功能需求编号 FR-ID、可度量成功标准 SC-ID），并严格适配 OpenSpec 机器可校验的 Markdown 结构（`## Requirements`、`### Requirement: ...`、`#### Scenario: ...`）。
   - 保留官方 OpenSpec CLI 校验能力，执行 `openspec validate --specs --strict` 确保零语法错误。

2. **保留 OpenSpec 作为全局执行治理框架，参考模板仅作局部副本**：
   - 在 `.specify/templates/` 保存 Spec-Kit 官方模板副本（锁定 commit `93989802b1401c99cc7de6e4ed76572f9cb662e8`）及 provenance 哈希记录。
   - 明确声明因工程既有 `pi-governance` 契约要求，保留 repo-local `openspec` CLI 作为变更驱动引擎，不安装或替换为外部 SpecKit 运行时。

3. **双模式策略组核心能力冻结与模糊语义显式标记**：
   - 将用户明确要求的“直接将其他策略组作为节点引用”（Direct Group Reference）与“解包获得其中节点”（Leaf Node Unpack/Flatten）确立为双核心功能需求。
   - 针对当前代码尚未实现的解包逻辑、混合交错顺序丢失、历史空组数据恢复、非 Mihomo 目标空组语义等未决事项，在规范中明确标注为 `[NEEDS CLARIFICATION]`，并通过结构化问题清单汇总呈报用户，严禁施工机擅自脑补默认逻辑。

4. **严格的文档生命周期与交付边界**：
   - 当前阶段为纯文档特性包阶段（Documentation Feature Package），涵盖规格重写、符合度审计矩阵构建、问题集编制与 OpenSpec Change 规划。
   - 本 Note 严格归入 `proposed/architecture/` 目录，文档状态为 `proposed`（规划待批），绝不归入 `implemented/` 虚假宣称业务代码已完工。

## 否决的备选方案 (Rejected Alternatives)

1. **否决方案 1：用 SpecKit 运行时 CLI 彻底替换 OpenSpec 框架**
   - *方案描述*：直接在环境中安装 SpecKit 工具链，废弃 `openspec/` 目录。
   - *否决依据*：本工程处于 Pi 治理体系下，多智能体协同、变更门禁与审查机制均深度绑定 repo-local `openspec` CLI。单方面替换全局执行框架将导致既有 26 个变更集及自动化门禁失效。因此保留 OpenSpec CLI，仅在规格组织结构层面吸收 SpecKit 的优秀实践。
   - *客观归属*：保留 OpenSpec CLI 属于项目工程治理要求，非用户指令限制。

2. **否决方案 2：将代码中既有的 `AllNodeLogicalIDs` 宣称为“叶节点解包已实现”**
   - *方案描述*：利用 `internal/resolver/graph.go` 中的 `AllNodeLogicalIDs` 方法，声称策略组解包展开功能已符合规格。
   - *否决依据*：实机代码审查（`audit-crosscheck`）证实，`AllNodeLogicalIDs` 仅递归收集节点 ID 用于空组判断与元数据统计，并未参与 `rg.Members` 的构造或编译器出站代理的展开。若将其宣称为已实现，属于虚假宣称，掩盖真实的功能缺口。

3. **否决方案 3：智能体自行预设未决业务逻辑的默认值并直接编码**
   - *方案描述*：对策略组解包去重规则、父级过滤传播、历史空组恢复策略及非 Mihomo 空组行为等未明确细节自行挑选“业界通行做法”并直接修改业务代码。
   - *否决依据*：用户明确要求“模糊整合询问我，然后再落地”。擅自预设默认值违反用户授权与实事求是原则。

4. **否决方案 4：将本决策记录置于 `.agents/notes/implemented/`**
   - *方案描述*：直接将本文件标记为已交付。
   - *否决依据*：依据 DeepSeek Agent Notes 生命周期规范，`implemented/` 仅用于代码与运行时已完全交付的事实；当前仅完成文档体系与审计矩阵编制，业务代码尚未修改，必须诚实记录为 `proposed`。

## 引用与相关工件

- 主干能力规范：`openspec/specs/`
- 来源等级注册表：`docs/specification/source-register.md`
- 规格符合度矩阵：`docs/specification/conformance-matrix.md`
- 业务澄清问题集：`docs/specification/clarification-questions.md`
- 变更跟踪集：`openspec/changes/csp-living-spec-consolidation/`
