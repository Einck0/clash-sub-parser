# Capability: Living Specification Consolidation

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的持续主干能力规范管理体系。系统必须建立唯一的、持续有效的主干能力规范集合，承载 6 大核心业务领域（订阅资产、策略组、探测引擎、发布导出、鉴权安全、控制台界面）的形式化需求契约，提供四级证据来源确权注册表与实机符合度审计矩阵，并为历史变更集提供完整的追溯映射。

## ADDED Requirements

### Requirement: 领域主干能力规范集合管理
系统 SHALL 在 `openspec/specs/` 目录下集中维护持续有效的领域主干能力规范，包括 `inventory`、`policy`、`probe`、`publication`、`auth`、`ui` 规范体系，并适配 RFC 2119 规范语法与 RFC 场景格式。

#### Scenario: 自动化严格语法校验
- **WHEN** 运行 openspec validate --specs --strict 命令
- **THEN** 所有主干能力规范均通过校验且退出码为 0，零语法或格式警告

### Requirement: 证据来源分级与用户原话确权
系统 SHALL 建立严格的四级证据来源分级体系（TIER-1 用户明确原话、TIER-2 历史变更契约、TIER-3 实机实现事实、TIER-4 开发者经验），在规范编制中严格区分客观事实与推测分析。

#### Scenario: 甄别变更提案分析
- **WHEN** 变更提案 proposal.md 中存在问题分析描述
- **THEN** 注册表将其准确归类为 TIER-2 历史契约分析，严禁冒充 TIER-1 用户原话

### Requirement: 历史变更追溯与双层规范契约映射
系统 SHALL 对历史积累的 26 个既有变更集和 63 份分项规范建立领域级追溯映射矩阵，明确历史规范的主干合流领域与尚未消解需求存根，确立『主干规范 + 历史存根』双层契约，确保历史契约不被无意删除。

#### Scenario: 追溯历史分流规则规范
- **WHEN** 检索历史变更 csp-fresh-inventory-full-validation 中的 policy/rules-validation 规范
- **THEN** 追溯矩阵明确指出其合流至 policy 领域的 FR-POL-05 与 publication 领域的 FR-PUB-01，未合并细分条款保留为历史存根

### Requirement: 结构化业务未决问题收敛
对于实机实现中存在的语义断层与未决需求，系统 SHALL 收敛形成不超过 6 组的业务问题清单，提供清晰的分项业务选项，且严禁施工机自行预设默认值。

#### Scenario: 呈现策略组解包未决选项
- **WHEN** 检视 clarification-questions.md 中的策略组解包问题
- **THEN** 清单清晰列出去重规则、递归深度与过滤传播的具体选项供用户决策
