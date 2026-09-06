## Purpose

为 AI 平台提供不依赖账户凭据、经待测节点真实出站的分级观察，使区域信号、Web 可达性与 App 请求形态不会被混同为完整产品可用性。

## ADDED Requirements

### Requirement: Claude regional-signal observation

系统 SHALL 以待测节点出站请求 Claude 的公开、无账户区域信号端点，并将结果表示为 Claude `region_signal` 观察。该观察 MUST 报告可解析的两位地区信号或结构化的 challenge、rate limit、timeout、transport error、inconclusive 结果；它 MUST NOT 因地区信号、静态黑名单或 HTTP 成功而声明 Claude Web、API、账户注册、付费或完整产品解锁。平台配置仅在显式启用 `claude` 时执行此观察。

#### Scenario: Parseable regional signal
- **WHEN** Claude 区域信号端点经待测节点返回可解析的两位地区代码
- **THEN** 系统 SHALL 返回 `verified` 的 `region_signal` 观察及该地区代码，并使用不表示“完整解锁”的 verdict 和 label

#### Scenario: Service capability remains unverified
- **WHEN** Claude 区域信号端点返回可解析地区代码
- **THEN** 该结果 SHALL NOT 使能力筛选、订阅生成或 NodeLedger 的“完整解锁”判断通过

#### Scenario: Endpoint challenge or contract drift
- **WHEN** Claude 区域信号端点返回挑战、429、超时、传输错误或不符合已知格式的成功响应
- **THEN** 系统 SHALL 返回相应的结构化非成功或不确定观察，而不得推断地区、可用性或封禁

### Requirement: ChatGPT Web and App tier observation

系统 SHALL 对 ChatGPT 保留并行可审计的 Web 观察与 iOS App 请求形态观察。Web 或 App 的正向结论 MUST 各自建立在该请求形态的已知成功响应上；App 正向结论 SHALL 表示“App 请求形态已观察到可达”，而不是已登录账户可使用的产品保证。每个子观察 SHALL 在同一节点共享隔离媒体会话和媒体阶段截止时间，并在对外结果中保留各自状态、证据版本及脱敏证据。

#### Scenario: Web available and App available
- **WHEN** ChatGPT 的 Web 观察和 iOS App 请求形态均返回各自已知的正向响应
- **THEN** 系统 SHALL 返回一个明确表明 Web 与 App 两项均已观察到的 ChatGPT 结果，并仅在两项均为确认正向时显示最高级别

#### Scenario: Web available but App inconclusive
- **WHEN** Web 观察返回已知正向响应而 App 请求形态超时、挑战、限流或契约漂移
- **THEN** 系统 SHALL 保留 Web 正向证据和 App 非正向证据，将总结果降为非最高级别，并不得把 App 能力标为可用

#### Scenario: App challenge or rate limit
- **WHEN** App 请求形态返回 HTTP 403、Cloudflare challenge 或 HTTP 429
- **THEN** 系统 SHALL 将 App 子观察分别标为 challenged 或 rate_limited，且不得因 Web 或旧缓存成功而覆盖该次 App 结论

### Requirement: AI observation isolation and evidence boundary

Claude 与 ChatGPT 的所有观察 SHALL 通过待测节点的专属本地回环代理发送，禁用宿主环境代理，并只暴露允许的脱敏证据字段。结果 MUST NOT 包含原始响应 body、请求 headers、cookie、authorization、token、节点凭据或代理 URL。

#### Scenario: Observation detail is queried
- **WHEN** 操作者读取节点的 AI 平台观察详情
- **THEN** 响应 SHALL 仅含已允许的 HTTP 状态、最终主机、重定向类别、信号、耗时和错误分类等脱敏证据
