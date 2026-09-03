## MODIFIED Requirements

### Requirement: Node Outbound Isolation and Protocol Handshake
The system SHALL execute a candidate node probe through an isolated, bounded loopback runtime and SHALL persist a terminal observation linked to the canonical node identity and probe profile revision. The runtime SHALL NOT inherit host environment proxies and SHALL reclaim temporary files and ports on all terminal paths.

#### Scenario: Successful protocol handshake
- **WHEN** a valid supported proxy node is submitted for probing
- **THEN** the system verifies a 204 endpoint through the node egress, records a successful transport observation with measured latency, and cleans up the isolated runtime

#### Scenario: Invalid node credentials or unreachable server
- **WHEN** a node configuration contains invalid TLS parameters or its server is unreachable
- **THEN** the system records a structured failed observation within configured bounds and reclaims all allocated resources without exposing private node parameters

### Requirement: Egress Identity and Geo-location Consensus
The system SHALL query multiple independent external IP verification providers through the verified node egress and SHALL store consensus, disagreement, and provider-failure states distinctly for the observation.

#### Scenario: Multi-provider geo consensus
- **WHEN** the node establishes outbound connection and independent providers agree
- **THEN** the observation records verified exit IP, ISO country code, and ASN with a consensus status

### Requirement: Streaming Media and AI Unlock Probing
The system SHALL provide modular platform probes through the verified node egress and SHALL classify each platform independently without private credentials. A platform-specific challenge, rate limit, or ambiguous response SHALL NOT be represented as a transport failure.

#### Scenario: Netflix unlock status classification
- **WHEN** a Netflix probe is executed through a candidate node
- **THEN** the observation classifies the Netflix capability as Full Unlock, Original Only, or Blocked or Restricted from the applicable response evidence without changing the node transport result

#### Scenario: AI and platform rate limit / challenge handling
- **WHEN** a service endpoint returns Cloudflare challenge or HTTP 429 through a successfully verified proxy
- **THEN** the observation marks only that provider as challenged or rate-limited while retaining the successful egress result

### Requirement: Bounded Speed Testing and Bandwidth Estimation
The system SHALL support bounded speed tests with configured global concurrency, per-job byte ceiling, duration ceiling, and cancellation cleanup. Speed results SHALL record their profile and observation time for policy-age evaluation.

#### Scenario: Controlled download speed benchmark
- **WHEN** speed test is triggered for a reachable node
- **THEN** the system measures throughput within the configured byte and duration ceilings and records the result without exceeding the configured worker concurrency

### Requirement: Capability Filtering and Subscription Generation Integration
The system SHALL evaluate generation filters against explicit policy rules and currently eligible observations, without modifying original proxy parameters. It SHALL explain exclusions caused by stale, missing, failed, or insufficient observations.

#### Scenario: Generating subscription filtered by capability
- **WHEN** a user requests output filtered by a capability policy requiring recent ChatGPT and Gemini availability and a speed threshold
- **THEN** output includes only nodes satisfying every policy criterion and diagnostics distinguish each excluded node's reason