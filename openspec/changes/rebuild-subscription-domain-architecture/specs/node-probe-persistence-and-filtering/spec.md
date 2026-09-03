## MODIFIED Requirements

### Requirement: Node Probe Results Persistence
The system SHALL store each node probe outcome as an immutable observation keyed by canonical node identity, probe profile revision, and execution identity. The system SHALL derive a latest eligible result for a policy without overwriting earlier observations or using display name as the primary key.

#### Scenario: Saving probe outcome
- **GIVEN** a node capability probe finishes
- **WHEN** the result contains transport, egress, speed, or platform outcomes
- **THEN** the system stores an observation linked to the stable node identifier and makes it queryable with its timestamp, profile, and terminal status

### Requirement: Periodic Background Node Probing
The system SHALL schedule periodic probing as observable bounded jobs over the active canonical inventory. It SHALL avoid overlapping equivalent scheduled jobs and SHALL report skipped, queued, cancelled, and completed outcomes.

#### Scenario: Running background probing
- **GIVEN** probe_interval_minutes is configured to a positive integer M
- **WHEN** M minutes have elapsed and no equivalent job is active
- **THEN** the scheduler creates one bounded inventory job, persists its job status and observations, and does not start a duplicate overlap

### Requirement: Capability-Based Filtering in Subscriptions and Node Groups
The system SHALL filter nodes for subscriptions and node groups through explicit policy evaluation over observations with declared profile compatibility and maximum age. Missing, stale, failed, or incompatible observations SHALL not satisfy a positive capability requirement.

#### Scenario: Filtering by speed and unlock targets
- **GIVEN** a node group configured with a 10.0 Mbps minimum and ChatGPT and Gemini capability requirements
- **WHEN** nodes are resolved for that group
- **THEN** only nodes with a recent compatible successful observation meeting every criterion are included and all exclusions have a structured explanation