## MODIFIED Requirements

### Requirement: Streaming Media and AI Unlock Probing

系统 SHALL 通过候选节点的本地回环监听器提供模块化流媒体与 AI 服务探测，不使用私有凭据。对于同一节点的一次媒体阶段，系统 SHALL 复用一个固定到候选节点回环代理、禁用宿主环境代理的 HTTP 会话；所有平台 SHALL 共享一个由已配置媒体超时定义的绝对截止时间。单个平台的超时、运输错误、挑战、限流或不确定响应 SHALL 保留为该平台的结构化证据结果，不得宣布节点整体死亡或丢弃兄弟平台结果。ChatGPT SHALL 保留可区分的 Web 与 iOS App 请求形态观察；Claude SHALL 仅作为区域信号观察，除非未来存在独立、可审计的服务能力契约。只有被能力筛选显式认可的 verified full 或 verified available 结果才可满足现有媒体能力条件。

#### Scenario: Netflix unlock status classification
- **WHEN** Netflix 探测经候选节点执行
- **THEN** 系统 SHALL 先以凭据无关的 Netflix CDN 快速响应确认可解析的完整区域或 HTTP 403 IP 阻断；无法得出结论时 SHALL 回退到非自制剧和自制剧标题证据，并将能力分类为 Full Unlock、Original Only 或 Blocked Restricted，且保留相应证据路径

#### Scenario: AI and platform rate limit / challenge handling
- **WHEN** 某服务端点返回 Cloudflare challenge、HTTP 429、超时、运输错误或无法识别的响应
- **THEN** 系统 SHALL 仅将该提供商标记为 challenged、rate-limited、timeout、transport-error 或 inconclusive，而不宣布节点完全死亡，也不丢弃同一媒体阶段已完成提供商的结果

#### Scenario: Shared-session outbound isolation
- **WHEN** 一个传输健康节点在同次请求中探测多个媒体或 AI 平台
- **THEN** 系统 SHALL 通过同一个固定到该节点本地回环监听器的 HTTP 会话发送全部平台请求，且该会话 SHALL 不继承宿主环境代理

#### Scenario: Tiered ChatGPT outcome
- **WHEN** 同一节点的 ChatGPT Web 观察为确认可达而 iOS App 请求形态未获得确认可达
- **THEN** 系统 SHALL 保留两个子观察并返回非最高级别，且不得将 App 形态作为完整可用

### Requirement: Bounded Speed Testing and Bandwidth Estimation

The system SHALL support bounded single-thread download speed tests with strict global concurrency limits, finite payload ceilings, and timeout safeguards to prevent saturating host network bandwidth. The download implementation SHALL retain its existing byte ceiling and timeout behavior while batch dispatch changes; a request completion SHALL free exactly one batch worker slot, not create an additional unbounded speed-test worker.

#### Scenario: Controlled download speed benchmark
- **WHEN** speed test is triggered for a reachable node
- **THEN** the system downloads chunks from a designated test endpoint up to a configurable ceiling (e.g., 5MB–10MB), calculates peak and average throughput in Mbps, and strictly respects maximum worker concurrency.

#### Scenario: Sliding batch retains speed bound
- **WHEN** a manual sliding batch includes speed tests and one node completes
- **THEN** the next node MAY begin only after that node’s bounded probe workflow has released its batch slot, and active probe workflows SHALL not exceed the effective configured concurrency
