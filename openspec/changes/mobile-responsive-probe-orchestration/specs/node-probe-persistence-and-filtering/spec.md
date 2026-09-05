## MODIFIED Requirements

### Requirement: Node Probe Results Persistence
The system SHALL store the outcome of all node capability and speed probes in a persistent relational table. A completed node result SHALL preserve structured node-level and platform-level timeout classifications alongside latency, geo IP, speed, and streaming unlock outcomes, keyed by `name|type|server:port` in `node_probe_results`.

#### Scenario: Saving probe outcome
- **WHEN** a node capability probe finishes
- **THEN** the system stores or updates the record keyed by `name|type|server:port` in `node_probe_results` with latency, geo, speed, streaming, and timeout outcomes that completed during the workflow

### Requirement: Periodic Background Node Probing
The system SHALL support periodic automated node probing with a persisted explicit enable flag and interval. `probe_cron_enabled` SHALL default to true and `probe_cron_interval_minutes` SHALL default to 60. A scheduler tick SHALL read the latest stored configuration before deciding whether to start a new scheduled run. `probe_interval_minutes` SHALL remain compatible as a legacy manual-interval field and MUST NOT be treated as the enable flag or interval of the new scheduled quality-check contract.

#### Scenario: Running background probing
- **GIVEN** `probe_cron_enabled` is true and `probe_cron_interval_minutes` is configured to a positive integer M
- **WHEN** M minutes have elapsed since the previous scheduled probe run began and no scheduled probe run is active
- **THEN** the background scheduler triggers probing for all active subscription nodes and persists the results

#### Scenario: Disabling background probing
- **GIVEN** `probe_cron_enabled` is false
- **WHEN** the scheduler performs its next interval check
- **THEN** it starts no new background probe run regardless of the configured interval

#### Scenario: Applying schedule changes without restart
- **GIVEN** the service is running with one scheduled quality-check configuration
- **WHEN** a Settings update successfully changes the cron enable flag or interval
- **THEN** the next scheduler check uses the updated values without a service or container restart

### Requirement: Capability-Based Filtering in Subscriptions and Node Groups
The system SHALL filter nodes for subscriptions and node groups based on speed thresholds and multi-selected streaming/AI platform unlocks.

#### Scenario: Filtering by speed and unlock targets
- **GIVEN** a node group configured with `filter_min_speed_mbps = 10.0` and `filter_media_unlock = ['chatgpt', 'gemini']`
- **WHEN** nodes are resolved for this group
- **THEN** only nodes with latest probe status 'ok', speed greater than or equal to 10.0 Mbps, and confirmed unlocks for both ChatGPT and Gemini are included
