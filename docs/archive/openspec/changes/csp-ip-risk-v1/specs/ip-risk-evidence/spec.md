## Purpose

定义独立于既有平台可用性判断的出口 IP 风险证据合同，使风险数据只经被测节点的网络路径取得、可追溯地过期且不泄露完整 IP 或第三方凭据。

## ADDED Requirements

### Requirement: 受控的出口 IP 风险探测
系统 SHALL 将 IP 风险查询建模为独立的 `ip_risk` 探测类型。每次查询 MUST 在已建立的被测节点探测通道内取得出口身份并经同一节点通道访问已配置 provider；宿主环境代理、直连连接或另一节点的出口不得作为该节点的风险证据。系统 MUST 禁止通过浏览器或 HTML 抓取 provider 网页端。

#### Scenario: 宿主代理与被测节点出口不同
- **WHEN** 宿主环境代理可以访问 provider 而被测节点探测通道失败
- **THEN** 系统记录该节点的 `error` 或 `unknown` 风险观察，且不会用宿主代理结果替代

### Requirement: Provider 启用、凭据与配额边界
系统 SHALL 仅调用已由部署端显式启用且具有有效 secret reference 的官方 API provider。provider 凭据 MUST 不出现在 API 响应、审计、日志、证据摘要、导出物或前端状态。系统 MUST 对每个 provider 实施配置化的最大并发、请求速率、每日预算和 deadline；预算耗尽、缺少凭据、429 或 provider 不可用 MUST 产生可区分的 `unknown` 或 `error` 观察，且不得重试到无界。

#### Scenario: Provider 预算耗尽
- **WHEN** 某个已启用 provider 的请求将超过其配置的预算
- **THEN** 系统不发送该请求，记录明确的预算原因，并将节点风险状态保留为 `unknown` 而非低风险

### Requirement: 不可变、最小化的风险观察
系统 SHALL 为每个成功、受限或失败的 IP risk 查询创建不可变观察，包含节点 `logical_id`、provider 标识及 schema 版本、观察时间、TTL、规范化风险 score 或缺失原因、置信度、匿名化和网络类别、判定状态、SHA-256 evidence digest 与有限的脱敏摘要。系统 MUST 不持久化完整出口 IP、完整第三方响应、API key、cookie、请求 URL query 或网页 body。

#### Scenario: 成功查询被持久化
- **WHEN** 已启用 provider 返回符合其版本化响应合同的风险数据
- **THEN** 系统保存可关联节点和 provider 版本的不可变观察及摘要，不保存完整 IP 或原始 payload

### Requirement: 风险证据新鲜度与缓存隔离
系统 SHALL 将风险观察限定为其 provider、出口身份摘要和 TTL。缓存命中 MUST 仅用于具有同一 node `logical_id`、同一 provider/schema version、未过期且出口身份摘要未变化的查询。过期、出口身份变化、provider 合同不匹配或无法验证出口身份的结果 MUST 不被视为当前有效风险证据。

#### Scenario: 出口变更后的查询
- **WHEN** 节点新的出口身份摘要与最近风险观察的摘要不同
- **THEN** 系统不复用旧观察，并在新查询完成前向风险 policy 提供 `unknown`

### Requirement: 多 provider 归一化不伪造确定性
系统 SHALL 将 provider 的原始风险语义归一化为明确的 score 范围、置信度、网络类别和匿名化特征，并保留 provider 标识与版本。没有可比映射、provider 间冲突或置信度低于 policy 最低值时 MUST 标为 `unknown` 或交由 policy 的显式冲突策略；系统不得将缺失或冲突自动折算为低风险。

#### Scenario: Provider 数据不完整
- **WHEN** provider 响应不包含 policy 所需的 score 或网络类别
- **THEN** 系统记录合同缺失并产出 `unknown`，不推导 `allow`
