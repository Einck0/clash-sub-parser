# Specification: Node Probe Persistence, Periodic Probing, and Filtering

## ADDED Requirements

### Requirement: Node Probe Results Persistence
The system SHALL store the outcome of all node capability and speed probes in a persistent relational table.

#### Scenario: Saving probe outcome
- GIVEN a node capability probe finishes
- WHEN the result contains latency, geo ip, speed, or streaming unlock status
- THEN the system stores or updates the record keyed by `name|type|server:port` in `node_probe_results`.

### Requirement: Periodic Background Node Probing
The system SHALL support periodic automated node probing on a configurable interval.

#### Scenario: Running background probing
- GIVEN `probe_interval_minutes` is configured to a positive integer $M$
- WHEN $M$ minutes have elapsed since the previous background probe
- THEN the background scheduler triggers probing for all active subscription nodes and persists the results.

### Requirement: Capability-Based Filtering in Subscriptions and Node Groups
The system SHALL filter nodes for subscriptions and node groups based on speed thresholds and multi-selected streaming/AI platform unlocks.

#### Scenario: Filtering by speed and unlock targets
- GIVEN a node group configured with `filter_min_speed_mbps = 10.0` and `filter_media_unlock = ['chatgpt', 'gemini']`
- WHEN nodes are resolved for this group
- THEN only nodes with latest probe status 'ok', speed $\ge$ 10.0 Mbps, and confirmed unlocks for both ChatGPT and Gemini are included.
