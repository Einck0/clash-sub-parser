## Purpose

Provides comprehensive proxy node health evaluation, accurate geo-identity consensus, streaming and AI service unlocking capability detection, and controlled speed testing for parsed subscription nodes.

## ADDED Requirements

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
The system SHALL provide modular probes to test node accessibility against streaming platforms (YouTube, Netflix, Disney+) and AI service endpoints (ChatGPT) without using private credentials.

#### Scenario: Netflix unlock status classification
- **WHEN** Netflix probe is executed through the candidate node
- **THEN** the system classifies the node capability as Full Unlock (both regional and original catalog available), Original Only (limited catalog), or Blocked/Restricted based on response status and title availability.

#### Scenario: AI and platform rate limit / challenge handling
- **WHEN** a service endpoint returns Cloudflare challenge or HTTP 429 rate limit
- **THEN** the system marks the specific provider as challenged or rate-limited without declaring the node completely dead.

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
