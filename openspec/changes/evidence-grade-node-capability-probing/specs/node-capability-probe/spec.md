## MODIFIED Requirements

### Requirement: Egress Identity and Geo-location Consensus
The system SHALL query multiple independent external IP verification providers through the node egress to establish a consensus on the exit IP, ISO country code, and ASN. Each provider request SHALL have its own configured service-level timeout, and failure or timeout of one provider SHALL not route traffic through the host or prevent other independent probe services from completing. The system SHALL record bounded identity-provider observations and report identity as verified only when at least two independent successful providers agree on the outbound IP and ISO country code; disagreement MUST be reported as conflicted rather than as a confirmed location.

#### Scenario: Multi-provider geo consensus
- **WHEN** the node establishes outbound connection and at least two independent identity providers report the same outbound IP and ISO country code through the node's local listener
- **THEN** the probe records the verified country code and outbound IP with agreement evidence and does not use a host-egress fallback

#### Scenario: Geo provider deadline expires
- **WHEN** one identity provider does not respond before the configured service-level timeout
- **THEN** the system records that provider as timed out or unavailable without treating the host egress as a fallback result

#### Scenario: Identity provider disagreement
- **WHEN** successful identity providers disagree on the observed outbound IP or ISO country code
- **THEN** the probe reports identity confidence as conflicted, does not claim a verified country or ASN consensus, and continues unrelated media and AI provider checks through the candidate node

### Requirement: Streaming Media and AI Unlock Probing
The system SHALL provide modular, credential-free probes to test node accessibility against streaming platforms (YouTube, Netflix, Disney+) and AI service endpoints (ChatGPT) through the candidate node. Each selected platform probe SHALL have an independently enforced configured service-level timeout. Every result SHALL distinguish verified availability, partial catalogue availability, regional restriction, IP reputation block, challenge, rate limit, timeout, transport failure, and inconclusive provider-contract drift; a generic HTTP success SHALL NOT by itself be treated as an unlock. A platform timeout, challenge, rate limit, restriction, or inconclusive result SHALL be represented for that platform and SHALL NOT by itself mark the node transport dead or cancel unrelated selected platform probes.

#### Scenario: Netflix unlock status classification
- **WHEN** Netflix probe is executed through the candidate node
- **THEN** the system classifies the node capability as `full` only when both configured non-original and original catalogue evidence are observed, `originals_only` when only the original catalogue evidence is observed, or a distinct restricted, IP-blocked, challenged, rate-limited, timeout, transport-error, or inconclusive result based on the provider response

#### Scenario: AI and platform rate limit / challenge handling
- **WHEN** a service endpoint returns a configured challenge or HTTP 429 rate-limit signal
- **THEN** the system marks the specific provider as challenged or rate-limited with sanitized evidence and without declaring the node completely dead

#### Scenario: One platform times out
- **WHEN** a selected platform does not complete before the configured service-level timeout
- **THEN** the system records a structured timeout for that platform while retaining any completed transport, geo, speed, and other platform outcomes

#### Scenario: Provider contract is unrecognized
- **WHEN** a platform response matches neither the active provider contract's positive nor negative signals
- **THEN** the system records an inconclusive result with a versioned, sanitized signal summary and SHALL NOT expose it as unlocked to capability filters

### Requirement: Capability Filtering and Subscription Generation Integration
The system SHALL persist probe results with timestamps and capability tags, allowing subscription generation pipelines to filter or group nodes based on media unlock and speed criteria. A media or AI filter SHALL accept only a verified supported capability, including legacy recognized full-unlock values retained from historical records. Partial catalogue, restricted, IP-blocked, challenged, rate-limited, timeout, transport-error, and inconclusive results MUST NOT satisfy a full-unlock filter. Additive evidence SHALL not alter node raw proxy parameters, node identity keys, or historical result rows.

#### Scenario: Generating subscription filtered by capability
- **WHEN** a user requests subscription output filtered by capability (such as `netflix=true` and `latency < 500ms`)
- **THEN** the system outputs only nodes with a recent valid speed result and a verified full Netflix unlock, while preserving original raw proxy parameters

#### Scenario: Historical full-unlock result compatibility
- **WHEN** a historical probe row contains a recognized legacy full-unlock platform value without additive evidence
- **THEN** existing subscription and node-group filtering retains its established full-unlock interpretation until a newer observation replaces it
