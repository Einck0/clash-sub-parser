## Purpose

定义 CSP 管理工作台中 IP risk 的安全回显、服务端筛选和高风险操作反馈，使操作者能理解决策而不获取完整出口地址或 provider 原始数据。

## ADDED Requirements

### Requirement: 节点风险摘要与未知状态回显
工作台 SHALL 在节点台账和详情中显示当前风险决策、风险等级或 score 区间、provider 标识、观察时间、过期状态和脱敏原因。工作台 MUST 明确区分 `allow`、`review`、`block`、`unknown`、`stale` 和 provider error，且不得将 `unknown`、`stale` 或 error 渲染为低风险或绿色通过。

#### Scenario: 未配置风险 provider
- **WHEN** 节点没有可用的 IP risk 观察，因为没有启用 provider
- **THEN** 工作台显示 `unknown` 与配置缺失原因，不显示低风险 score

### Requirement: 服务端风险筛选与稳定分页
工作台 SHALL 将风险决策、风险等级、provider、过期状态和策略组风险状态作为可恢复 URL query 的服务端筛选条件。节点列表 MUST 在风险筛选后进行固定排序和分页，并返回与该筛选集合一致的 `total`；浏览器不得下载完整台账后再执行风险筛选。

#### Scenario: 筛选被阻断节点
- **WHEN** 操作者在节点台账选择 `risk_decision=block`
- **THEN** 工作台请求对应服务端页并只显示当前筛选集合中的节点及一致 total

### Requirement: 高风险准入和发布提示
工作台 SHALL 在策略组配置和 publication preflight 中显示风险 policy 是否启用、其 revision、未知处理方式和已阻断或待审节点数。对于会使节点被排除或使 publication 被拒绝的操作，工作台 MUST 在提交前显示后果并在收到诊断后保留可读的失败状态；界面不得自动关闭、隐藏或擅自修改 policy 以绕过拦截。

#### Scenario: publication 预检被拒绝
- **WHEN** 发布请求因风险 policy 被拒绝
- **THEN** 工作台显示脱敏节点标识、原因代码和 policy revision，并保持既有 publication 状态不变

### Requirement: 无敏感风险数据的客户端边界
工作台 MUST 不接收、缓存、记录或显示完整出口 IP、provider API key、原始 provider response、cookie 或包含凭据的查询 URL。风险展示 API 仅返回规范化摘要与必要的版本和时间元数据。

#### Scenario: 查看节点风险详情
- **WHEN** 操作者打开节点风险详情
- **THEN** 响应和界面仅包含允许的规范化摘要，不包含完整 IP 或 provider 原始 payload
