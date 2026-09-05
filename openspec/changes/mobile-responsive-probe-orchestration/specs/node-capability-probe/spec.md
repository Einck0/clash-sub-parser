## MODIFIED Requirements

### Requirement: Node Outbound Isolation and Protocol Handshake
The system SHALL spin up an isolated runtime instance for each candidate proxy node using a dedicated loopback port to verify protocol handshake and measure transport latency. Runtime startup readiness and transport handshake SHALL each be constrained by the configured service-level timeout. The system SHALL classify a startup or handshake deadline expiry as `timeout`, reclaim the allocated port, subprocess, and temporary files, and MUST NOT inherit host environment proxies.

#### Scenario: Successful protocol handshake
- **WHEN** a valid proxy node (such as Shadowsocks, VMess, VLESS with Reality, or Trojan) is submitted for probing
- **THEN** the system launches an isolated runtime, verifies connectivity to a standard 204 endpoint via the node egress without inheriting host environment proxies, records latency, and cleanly tears down the runner instance

#### Scenario: Invalid node credentials or unreachable server
- **WHEN** a node configuration contains invalid TLS parameters or server is unreachable
- **THEN** the system reports a structured failure reason within the configured service timeout limit and reclaims all allocated ports and temporary files

#### Scenario: Runtime startup deadline expires
- **WHEN** the isolated runtime has not accepted connections before the configured service-level timeout
- **THEN** the system records a timeout result, terminates the runtime, removes temporary resources, and releases capacity for another node workflow

### Requirement: Egress Identity and Geo-location Consensus
The system SHALL query multiple independent external IP verification providers through the node egress to establish a consensus on the exit IP, ISO country code, and ASN. Each provider request SHALL have its own configured service-level timeout, and failure or timeout of one provider SHALL not route traffic through the host or prevent other independent probe services from completing.

#### Scenario: Multi-provider geo consensus
- **WHEN** the node establishes outbound connection
- **THEN** the probe client queries at least two independent IP identity providers through the node's local listener and records the verified country code and outbound IP only when providers reach consensus

#### Scenario: Geo provider deadline expires
- **WHEN** one identity provider does not respond before the configured service-level timeout
- **THEN** the system records that provider as timed out or unavailable without treating the host egress as a fallback result

### Requirement: Streaming Media and AI Unlock Probing
The system SHALL provide modular probes to test node accessibility against streaming platforms (YouTube, Netflix, Disney+) and AI service endpoints (ChatGPT) without using private credentials. Each selected platform probe SHALL have an independently enforced configured service-level timeout. A platform timeout, challenge, or rate limit SHALL be represented for that platform and SHALL NOT by itself mark the node transport dead or cancel unrelated selected platform probes.

#### Scenario: Netflix unlock status classification
- **WHEN** Netflix probe is executed through the candidate node
- **THEN** the system classifies the node capability as Full Unlock (both regional and original catalog available), Original Only (limited catalog), or Blocked/Restricted based on response status and title availability

#### Scenario: AI and platform rate limit / challenge handling
- **WHEN** a service endpoint returns Cloudflare challenge or HTTP 429 rate limit
- **THEN** the system marks the specific provider as challenged or rate-limited without declaring the node completely dead

#### Scenario: One platform times out
- **WHEN** a selected platform does not complete before the configured service-level timeout
- **THEN** the system records a structured timeout for that platform while retaining any completed transport, geo, speed, and other platform outcomes

### Requirement: Bounded Speed Testing and Bandwidth Estimation
The system SHALL support bounded single-thread download speed tests with strict global concurrency limits, finite payload ceilings, and per-service timeout safeguards to prevent saturating host network bandwidth. The speed test request and stream SHALL stop at the configured service-level timeout even if the byte ceiling has not been reached.

#### Scenario: Controlled download speed benchmark
- **WHEN** speed test is triggered for a reachable node
- **THEN** the system downloads chunks from a designated test endpoint up to a configurable ceiling, calculates throughput in Mbps, and strictly respects maximum worker concurrency

#### Scenario: Speed service deadline expires
- **WHEN** a speed test has not completed before the configured service-level timeout
- **THEN** the system stops the stream, returns a structured timeout or partial-timeout outcome with measured bytes where available, and releases the node workflow capacity
