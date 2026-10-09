## Purpose

定义 IP 风险证据如何被版本化 policy 解释，并在策略组解析和发布前以确定性、可解释且默认安全的方式控制节点准入。

## ADDED Requirements

### Requirement: 版本化风险 policy 与显式未知处理
系统 SHALL 允许授权管理者创建、审阅、激活和停用版本化 IP risk policy。每个 policy MUST 声明 score 阈值、最低置信度、网络或匿名化类别条件、证据最大年龄和 `unknown` 处理方式。`unknown` 处理 MUST 是 `allow`、`review` 或 `block` 中之一，且默认值 MUST 为 `review`；未激活 policy 不得改变节点选择或发布语义。

#### Scenario: 缺少当前风险证据
- **WHEN** 已激活 policy 遇到没有未过期风险观察的节点
- **THEN** 系统应用该 policy 的显式 `unknown` 结果并记录可解释原因，而不把节点标为低风险

### Requirement: 确定性风险决策与解释
系统 SHALL 将当前有效风险观察及已激活 policy 解释为 `allow`、`review`、`block` 或 `unknown`。相同节点、观察集合、policy revision 和解析时点 MUST 产生相同决策、原因代码和决策摘要。解释 MUST 显示使用的 policy/provider 版本、观察时间和非秘密原因代码，但不得暴露完整 IP、provider 密钥或原始证据。

#### Scenario: 高分匿名出口
- **WHEN** 当前观察的风险分数、置信度和匿名化类别满足已激活 policy 的 block 条件
- **THEN** 系统返回 `block` 及可解释的规则原因代码

### Requirement: 策略组准入隔离
系统 SHALL 在策略图解析时把已启用 risk policy 的决策作为节点 admission 输入。`block` 节点 MUST 不进入受该 policy 保护的解析快照；`review` 节点 MUST 由 policy 的显式 group 行为决定纳入隔离组或排除；`allow` 节点按既有图语义继续解析。风险 policy MUST 不修改节点主记录、既有 capability verdict 或其他策略组中未引用该 policy 的成员资格。

#### Scenario: 高风险节点进入受保护策略组
- **WHEN** 解析一个绑定了已启用 policy 的策略组且某成员决策为 `block`
- **THEN** 解析快照排除该成员并给出其 logical ID 和脱敏原因代码

### Requirement: 发布前风险拦截
系统 SHALL 在创建 publication 前对目标解析快照重新计算已绑定且已启用 risk policy 的决策。若任一成员为 `block`，或 policy 定义 `review` 不可发布，系统 MUST 拒绝创建 publication 并返回可定位但已脱敏的 preflight diagnostics。系统 MUST 不通过静默删除节点、替换其他发布物或降级目标格式来绕过拦截。

#### Scenario: 导出前发现被阻断节点
- **WHEN** 管理者发布的快照包含一个当前决策为 `block` 的节点
- **THEN** 系统拒绝 publication，保留既有发布物不变，并返回风险 preflight 诊断

### Requirement: 决策快照可审计
系统 SHALL 将用于 policy admission 和 publication preflight 的 policy revision、观察 ID 或 digest、决策摘要及解析快照 digest 写入脱敏审计记录。审计记录 MUST 不包含完整 IP、原始 provider 数据或 secret reference 值。

#### Scenario: 审计一次风险阻断
- **WHEN** publication 被风险 preflight 拒绝
- **THEN** 审计记录可关联请求、policy revision、快照 digest 和阻断原因代码，且不含敏感原文
