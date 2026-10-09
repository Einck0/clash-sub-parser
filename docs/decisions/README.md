# CSP 架构决策记录 (Architecture Decision Records)

本目录为 `clash-sub-parser` 项目专属的技术决策记录（Architecture Decision Records / Agent Notes）唯一权威事实源（Canonical Single Source of Truth）。

> **规范声明**：
> 1. 本目录借鉴 DeepSeek Agent Notes 的生命周期分类思想（`proposed/`、`implemented/`、`rejected/`、`archived/`），但**本项目未安装外部 DeepSeek 工具链或运行时框架**，完全依赖 Git 与 Markdown 原生维护；
> 2. 项目的主干能力规范由 `openspec/specs/` 承载，变更工作流由 repo-local `openspec` CLI 驱动；本目录仅记录“架构为什么如此设计”以及“否决了哪些备选方案”。

## 目录结构与生命周期规范

路径编码规范：`{lifecycle}/{class}/yyyy-mm-dd-topic-title.zh.md`

- **生命周期 (Lifecycle)**：
  - `proposed/`：已提议或已接受但**尚未在业务代码中完全交付落地**的提案记录（当前文档阶段的新增决策均置于此）。
  - `implemented/`：业务代码已完整交付且处于生产运行态的决策记录。**严禁在仅完成文档编写而业务代码未上线时将记录置于 implemented**。
  - `rejected/`：经评审后被否决的提案，保留其拒绝理由以避免未来重复犯错。
  - `archived/`：已落地且不再指导日常开发的历史决策。

- **分类 (Classification)**：
  - `architecture`：核心架构与领域模型设计。
  - `feature`：关键业务特性与能力设计。
  - `process`：工程流程与规范演进。
  - `bug-fix`：重大缺陷与架构对齐修复。

## 治理与一事实归属准则

1. **唯一真实事实源**：项目的形式化规格基线由 `openspec/specs/` 承载，可执行工作流由 `openspec/changes/` 承载；本目录仅记录“为什么如此设计”以及“否决了哪些备选方案”。
2. **状态客观性**：文档状态严格区分 `proposed` / `accepted` 与 `implemented`，绝不虚假宣称未落地的代码已实现。
