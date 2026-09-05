## Purpose

Defines deterministic orchestration for manual and scheduled node quality checks, including service-level timeouts, bounded concurrency, configuration snapshots, and safe runtime reconfiguration.

## ADDED Requirements

### Requirement: Probe execution configuration contract
The system SHALL expose and persist `probe_concurrency`, `probe_service_timeout_ms`, `probe_cron_enabled`, and `probe_cron_interval_minutes` in the quality-control settings read and update contract. Their defaults SHALL be 10, 2000, true, and 60 respectively. `probe_concurrency` MUST accept values from 1 through 20, `probe_service_timeout_ms` from 500 through 30000, and `probe_cron_interval_minutes` from 1 through 1440. Existing `probe_timeout_ms` SHALL remain the optional total budget for one node workflow and SHALL NOT be reinterpreted as the service-level timeout.

#### Scenario: Reading default settings
- **WHEN** no persisted quality-control configuration row exists
- **THEN** the settings endpoint creates or returns a singleton configuration with concurrency 10, service timeout 2000ms, scheduled probing enabled, and a 60-minute interval

#### Scenario: Rejecting invalid orchestration settings
- **WHEN** a client submits a concurrency, service timeout, or cron interval outside its permitted range
- **THEN** the settings endpoint rejects the request without changing the stored configuration

### Requirement: Frozen probe-run configuration and bounded node concurrency
The system SHALL resolve one effective configuration snapshot when a manual or scheduled batch starts. It MUST use a semaphore or equivalent bounded pool that never runs more node workflows concurrently than the effective `probe_concurrency`. A client MAY request a lower concurrency for a manual batch but MUST NOT exceed the persisted setting. A configuration update MUST affect later batches without mutating an already running batch.

#### Scenario: Default concurrent batch
- **WHEN** a batch is started without a concurrency override and the persisted concurrency is 10
- **THEN** no more than 10 node workflows run at once and all eligible submitted nodes are eventually processed unless cancelled

#### Scenario: Settings change during a batch
- **WHEN** a 10-concurrent batch is already running and an administrator saves concurrency 4
- **THEN** the running batch retains its original concurrency snapshot and the next batch uses at most 4 concurrent node workflows

### Requirement: Independent service-level deadline classification
For each node workflow, transport handshake, egress identity lookup, each selected media or AI platform probe, controlled download speed probe, and runtime startup readiness SHALL each have an independently enforced deadline equal to `probe_service_timeout_ms`. A service timeout MUST produce a structured timeout classification for that service, release its resources, and allow unrelated services and unrelated node workflows to continue. The configured service-level deadline SHALL NOT terminate an entire server list, entire batch, or unrelated node workflow.

#### Scenario: One provider times out
- **WHEN** the Netflix provider does not complete before the configured 2000ms service deadline while transport succeeds
- **THEN** the Netflix result is classified as timeout, transport and other provider results remain available, and the node workflow does not block the batch pool beyond the timeout

#### Scenario: Runtime startup times out
- **WHEN** the isolated runtime does not become ready before the configured service deadline
- **THEN** the node result is classified as timeout, the subprocess, temporary files, and allocated loopback port are reclaimed, and other queued nodes can proceed

### Requirement: Dynamic scheduled quality checks
The scheduler SHALL inspect the persisted quality-control configuration at each scheduling tick. It SHALL start a scheduled quality-check batch only when `probe_cron_enabled` is true, at least `probe_cron_interval_minutes` have elapsed since the last scheduled batch began, and no scheduled batch is already running. A successful Settings update SHALL take effect no later than the next scheduling tick without service restart.

#### Scenario: Disabling scheduled probing
- **WHEN** an administrator saves `probe_cron_enabled` as false
- **THEN** no new scheduled quality-check batch starts after the next scheduler tick, while a currently running batch is allowed to finish or be cancelled according to its own lifecycle

#### Scenario: Changing the scheduled interval
- **WHEN** an administrator changes the interval from 60 to 15 minutes
- **THEN** the next scheduling tick evaluates 15 minutes against the recorded scheduled-run start time and uses the new interval without requiring a container restart
