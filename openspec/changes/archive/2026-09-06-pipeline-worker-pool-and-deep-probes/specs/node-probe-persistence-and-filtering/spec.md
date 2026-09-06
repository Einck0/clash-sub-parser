## MODIFIED Requirements

### Requirement: Periodic Background Node Probing
The system SHALL support periodic automated node probing on a configurable interval. Scheduling runtime status — current state, recent outcome, server clock, and estimated next trigger — SHALL be queryable through a protected endpoint. IP Risk observations MUST NOT participate in node capability filtering, subscription generation, or health statistics.

#### Scenario: Running background probing
- GIVEN `probe_cron_enabled` is true and `probe_cron_interval_minutes` is configured to a positive integer $M$
- WHEN $M$ minutes have elapsed since the previous background probe in the current process
- THEN the background scheduler triggers probing for all active subscription nodes, persists the results, updates the runtime status to reflect the latest run.

#### Scenario: IP Risk observation does not affect capability filtering
- WHEN a node has an IP Risk observation recorded alongside media capability observations
- THEN capability-based filtering, node-group resolution, and subscription generation SHALL evaluate only media capability fields and ignore the risk observation entirely
