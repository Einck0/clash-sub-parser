## MODIFIED Requirements

### Requirement: Node Probe Results Persistence
The system SHALL store the outcome of all node capability and speed probes in a persistent relational table. Media and AI outcomes SHALL retain the stable platform key and legacy availability semantics while permitting additive normalized status, verdict, evidence version, checked timestamp, confidence, and bounded sanitized evidence. Persistence MUST NOT delete, rewrite, or re-key historical `node_probe_results` records solely to introduce additive evidence.

#### Scenario: Saving probe outcome
- **GIVEN** a node capability probe finishes
- **WHEN** the result contains latency, geo IP, speed, streaming unlock status, or additive evidence
- **THEN** the system stores or updates the record keyed by `name|type|server:port` in `node_probe_results` without deleting unrelated historical rows or exposing secrets in the persisted evidence

#### Scenario: Reading historical and evidence-grade results
- **GIVEN** existing records contain legacy media values and newer records contain additive evidence fields
- **WHEN** a caller reads node probe results
- **THEN** each known platform key remains readable with its existing availability semantics and evidence-aware callers can consume the additive fields without an API version switch

### Requirement: Capability-Based Filtering in Subscriptions and Node Groups
The system SHALL filter nodes for subscriptions and node groups based on speed thresholds and multi-selected streaming/AI platform unlocks. A selected media or AI platform SHALL pass only when its latest result is a verified supported capability or a recognized historical full-unlock value; partial, restricted, IP-blocked, challenged, rate-limited, timeout, transport-error, and inconclusive results MUST NOT pass the full-unlock condition.

#### Scenario: Filtering by speed and unlock targets
- **GIVEN** a node group configured with `filter_min_speed_mbps = 10.0` and `filter_media_unlock = ['chatgpt', 'gemini']`
- **WHEN** nodes are resolved for this group
- **THEN** only nodes with latest probe status `ok`, speed greater than or equal to 10.0 Mbps, and verified unlocks for both ChatGPT and Gemini are included

#### Scenario: Partial or inconclusive platform result
- **GIVEN** a node group requires Netflix
- **WHEN** a node's latest Netflix result is `originals_only`, restricted, challenged, rate-limited, timeout, transport-error, or inconclusive
- **THEN** that node is excluded from the Netflix-required group while its other probe data and original proxy parameters remain unchanged
