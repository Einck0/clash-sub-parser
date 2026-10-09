# Policy Capability Specification

## Purpose

本规范定义 Clash 订阅解析器（clash-sub-parser）的策略组与图解析器核心能力。系统必须支持灵活的代理分流拓扑，提供两种核心的成员引用模式——直接将子策略组作为节点引用（Direct Group Reference）与解包获取其中的叶节点（Unpack / Flatten Leaf Nodes），支持全池动态匹配与无截断解析，并在策略组为空时提供受控的 PASS 兜底机制与防越权隔离。

## User Scenarios & Priority

### User Story 1 - 策略组双模式嵌套引用 (Priority: P1)
- **GIVEN** 管理员创建了一个顶级策略组 G1 和一个地区子组 G2
- **WHEN** 管理员在 G1 中选择引用 G2
- **THEN** 系统必须支持直接以组名引用 G2（Mihomo 原生组嵌套），亦必须支持解包模式将 G2 内的最终叶节点直接平铺展开至 G1

### User Story 2 - 全池动态条件无截断匹配 (Priority: P1)
- **GIVEN** 系统导入了超过 200 个有效节点
- **WHEN** 管理员配置了一个按国家代码或正则匹配的动态策略组
- **THEN** 图解析器全量遍历所有活动节点，返回所有匹配项，严禁施加 50 节点的隐藏硬分页截断

### User Story 3 - 空策略组安全兜底 (Priority: P2)
- **GIVEN** 某个策略组由于筛选条件过严或上游节点下线导致解析成员为 0
- **WHEN** 该组启用了 `empty_fallback_pass` 且导出为 Mihomo 配置
- **THEN** 编译器输出 `proxies: [PASS]` 与 `empty-fallback: PASS`，确保客户端基础流量不断网；未启用的空组输出 `REJECT`

---

## Requirements

### Requirement: 直接将其他策略组作为节点引用 [FR-POL-01]
系统 SHALL 允许策略组显式包含子策略组。在面向支持策略组嵌套的客户端（如 Mihomo）时，编译器 MUST 将子策略组作为成员（`MemberKindGroup`）直接渲染至 `proxies` 列表中。

#### Scenario: Mihomo 组嵌套渲染
- **WHEN** 顶级策略组引用了子策略组 SubGroupA 且模式为直接引用
- **THEN** Mihomo 导出配置中顶级组的 proxies 成员直接包含 "SubGroupA"

### Requirement: 解包获得子策略组叶节点 [FR-POL-02]
系统 SHALL 支持策略组解包模式（Unpack / Flatten）。在此模式下，图解析器 MUST 递归遍历子策略组的所有底层叶节点（Leaf Nodes），并将展开后的具体代理节点直接注入父策略组的成员集合中。

#### Scenario: 解包展开叶节点
- **WHEN** 策略组 ParentGroup 以解包模式引用了包含 Node1 和 Node2 的 ChildGroup
- **THEN** ParentGroup 的成员列表中直接包含 Node1 与 Node2，而不包含 ChildGroup 组名

### Requirement: 成员模式数据契约与前端编辑 [FR-POL-03] [NEEDS CLARIFICATION]
系统 SHALL 在策略组边定义（`group_edges`）及 API 契约中支持模式标识（如 `mode: direct_ref | unpack_nodes`），并在管理端工作台中提供直观的单选/多选交互。关于解包模式下的去重规则、父级过滤边界与异常节点约束，系统根据用户在 Q1 中的决策落实。

#### Scenario: 前端配置解包模式
- **WHEN** 管理员在策略组编辑抽屉中添加子组并勾选“展开为节点”
- **THEN** API 提交包含解包模式标记的数据载荷，服务端正确持久化并校验

### Requirement: 成员来源混合交错顺序保持 [FR-POL-04] [NEEDS CLARIFICATION]
系统 SHALL 允许策略组同时包含显式节点、子策略组及动态规则。用户编排的节点与子策略组之间的相对交错顺序（Interleaved Ordering）在当前实现中存在遍历分块缺陷，系统根据用户在 Q2 中确认的业务保序需求执行图解析器重构或规范固化。

#### Scenario: 显式指定交错顺序
- **WHEN** 用户以交错顺序添加了 [NodeA, ChildGroupB, NodeC]
- **THEN** 图解析器依据排序策略生成导出成员序列，并记录确定的排序诊断

### Requirement: 空策略组 PASS 兜底与防越权隔离 [FR-POL-05]
系统 SHALL 支持 `empty_fallback_pass` 布尔配置。当有效空组标记此选项时，Mihomo 编译器 MUST 输出 `proxies: [PASS]` 与 `empty-fallback: PASS`。对于未配置兜底的空组，系统 MUST 输出 `REJECT`，防止非授权穿透。

#### Scenario: 空组未授权时拦截
- **WHEN** 策略组有效成员为 0 且未配置 empty_fallback_pass
- **THEN** 编译器生成包含 REJECT 的出站组，阻止该策略组建立非预期连接

### Requirement: 全池动态筛选与无截断全量解析 [FR-POL-06]
系统 SHALL 支持全池动态策略组，依据协议、地区代码、正则及健康状态对全局活动节点进行匹配。解析器 MUST 使用 `ListAll` 全量检索候选池，严禁使用固定分页限制截断最终成员。

#### Scenario: 动态匹配超过 200 个节点
- **WHEN** 动态策略组匹配规则命中 250 个活跃节点
- **THEN** 解析器返回包含全部 250 个节点的完整成员列表

### Requirement: 历史遗留策略组数据迁移恢复 [FR-POL-07] [NEEDS CLARIFICATION]
系统对包含遗留 `group_nodes` 字段且成员边为 0 的历史轮询策略组，严禁以 `empty_fallback_pass` 作为等价修复方案，亦不得以普通组引用冒充展开原意。系统 SHALL 依据用户在澄清问题集 Q3 中确定的恢复范围与时机（待解包能力实现后按原配置单独批准恢复或明确重新配置），执行确定性的迁移与修复。

#### Scenario: 历史轮询组迁移处理
- **WHEN** 系统加载包含旧版 group_nodes 数据格式的历史策略组配置
- **THEN** 系统根据既定迁移策略转换为新契约或应用空组安全标记

---

## Success Criteria

### Measurable Outcomes
- **[SC-POL-01]** Mihomo 直接组引用在编译器输出中的映射准确率达到 100%。
- **[SC-POL-02]** 解包叶节点模式下（待落地实现）展开叶节点集合与子组底层节点一致率达到 100%。
- **[SC-POL-03]** 动态筛选在 250+ 节点压力用例下无截断丢失，解析完整率 100%。
- **[SC-POL-04]** 启用了 PASS 兜底的空策略组在 Mihomo 客户端上不断网、未配置组防越权有效率 100%。
