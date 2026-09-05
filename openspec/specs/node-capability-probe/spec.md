# node-capability-probe Specification

## Purpose
Provides comprehensive proxy node health evaluation, accurate geo-identity consensus, streaming and AI service unlocking capability detection, and controlled speed testing for parsed subscription nodes.

## Requirements

### Requirement: Node Outbound Isolation and Protocol Handshake
The system SHALL spin up an isolated runtime instance for each candidate proxy node using a dedicated loopback port to verify protocol handshake and measure transport latency.

#### Scenario: Successful protocol handshake
- **WHEN** a valid proxy node (such as Shadowsocks, VMess, VLESS with Reality, or Trojan) is submitted for probing
- **THEN** the system launches an isolated runtime, verifies connectivity to a standard 204 endpoint via the node egress without inheriting host environment proxies, records latency, and cleanly tears down the runner instance.

#### Scenario: Invalid node credentials or unreachable server
- **WHEN** a node configuration contains invalid TLS parameters or server is unreachable
- **THEN** the system reports a structured failure reason (such as handshake failure or timeout) within the configured timeout limit and reclaims all allocated ports and temporary files.

### Requirement: Egress Identity and Geo-location Consensus
The system SHALL query multiple independent external IP verification providers through the node egress to establish a consensus on the exit IP, ISO country code, and ASN.

#### Scenario: Multi-provider geo consensus
- **WHEN** the node establishes outbound connection
- **THEN** the probe client queries at least two independent IP identity providers through the node's local listener and records the verified country code and outbound IP only when providers reach consensus.

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

### Requirement: Bounded Speed Testing and Bandwidth Estimation
The system SHALL support bounded single-thread download speed tests with strict global concurrency limits, finite payload ceilings, and timeout safeguards to prevent saturating host network bandwidth.

#### Scenario: Controlled download speed benchmark
- **WHEN** speed test is triggered for a reachable node
- **THEN** the system downloads chunks from a designated test endpoint up to a configurable ceiling (e.g., 5MB–10MB), calculates peak and average throughput in Mbps, and strictly respects maximum worker concurrency.

### Requirement: Capability Filtering and Subscription Generation Integration
The system SHALL persist probe results with timestamps and capability tags, allowing subscription generation pipelines to filter or group nodes based on media unlock and speed criteria.

#### Scenario: Generating subscription filtered by capability
- **WHEN** a user requests subscription output filtered by capability (such as `netflix=true` and `latency < 500ms`)
- **THEN** the system outputs only nodes meeting the capability criteria from recent valid probe runs without modifying original raw proxy parameters.
