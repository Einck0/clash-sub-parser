## MODIFIED Requirements

### Requirement: Streaming Media and AI Unlock Probing
系统 SHALL 通过候选节点的本地回环监听器提供模块化流媒体与 AI 服务探测，不使用私有凭据。对于同一节点的一次媒体阶段，系统 SHALL 复用一个固定到候选节点回环代理、禁用宿主环境代理的 HTTP 会话；所有平台 SHALL 共享一个由已配置媒体超时定义的绝对截止时间。单个平台的超时、运输错误、挑战、限流或不确定响应 SHALL 保留为该平台的结构化证据结果，不得宣布节点整体死亡或丢弃兄弟平台结果。

#### Scenario: Netflix unlock status classification
- **WHEN** Netflix 探测经候选节点执行
- **THEN** 系统 SHALL 先以凭据无关的 Netflix CDN 快速响应确认可解析的完整区域或 HTTP 403 IP 阻断；无法得出结论时 SHALL 回退到非自制剧和自制剧标题证据，并将能力分类为 Full Unlock、Original Only 或 Blocked Restricted，且保留相应证据路径

#### Scenario: AI and platform rate limit / challenge handling
- **WHEN** 某服务端点返回 Cloudflare challenge、HTTP 429、超时、运输错误或无法识别的响应
- **THEN** 系统 SHALL 仅将该提供商标记为 challenged、rate-limited、timeout、transport-error 或 inconclusive，而不宣布节点完全死亡，也不丢弃同一媒体阶段已完成提供商的结果

#### Scenario: Shared-session outbound isolation
- **WHEN** 一个传输健康节点在同次请求中探测多个媒体或 AI 平台
- **THEN** 系统 SHALL 通过同一个固定到该节点本地回环监听器的 HTTP 会话发送全部平台请求，且该会话 SHALL 不继承宿主环境代理
