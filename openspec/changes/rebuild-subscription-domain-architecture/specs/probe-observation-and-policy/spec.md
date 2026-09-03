## Purpose

提供可追溯、有限资源且语义明确的节点探测与策略评估，让真实代理出站、速度、地区和各平台能力能够被安全地用于筛选而不会篡改节点本体。

## ADDED Requirements

### Requirement: Immutable observation records
The system SHALL persist each completed probe as an immutable observation linked to a stable node identifier, probe profile revision, execution time, and terminal outcome. A newer observation SHALL NOT overwrite the historical semantic result of an earlier run.

#### Scenario: Repeated probe retains history
- **WHEN** the same node is probed twice with different results
- **THEN** both observations remain queryable and the system can identify which one is currently eligible for a policy

### Requirement: Real egress semantics
The system SHALL classify handshake success, proxy-routed egress verification, destination reachability, bandwidth measurement, and individual platform capability as distinct results. It SHALL NOT infer platform capability from a generic TCP or HTTP success.

#### Scenario: Platform challenge is not node death
- **WHEN** a platform endpoint returns a challenge or rate-limit response through a successfully verified proxy egress
- **THEN** the observation records the platform-specific challenged or rate-limited result while retaining successful transport and egress evidence

### Requirement: Bounded and cancellable execution
The system SHALL enforce configured global and per-request concurrency, byte, duration, port, and cleanup bounds for probe jobs. A cancelled or timed-out job SHALL reclaim runner resources and record a terminal non-success outcome.

#### Scenario: Batch respects concurrency bound
- **WHEN** an operator submits more nodes than the configured worker limit
- **THEN** no more than the configured number of probe runners execute concurrently and queued work remains observable

### Requirement: Explicit observation eligibility
The system SHALL evaluate capability policies against observations using an explicit maximum age, profile compatibility rule, and result-status rule. Missing, stale, failed, or incompatible observations SHALL have distinct explanations.

#### Scenario: Stale speed result is rejected
- **WHEN** a group requires a minimum speed and the newest successful speed observation exceeds the configured maximum age
- **THEN** the node is excluded with a stale-observation explanation rather than treated as meeting the threshold

### Requirement: Probe data privacy
The system SHALL redact node credentials and source secrets from job status, observations, logs, and diagnostic responses.

#### Scenario: Failed probe diagnostic
- **WHEN** a probe fails due to authentication or handshake configuration
- **THEN** the returned error conveys the failure category without echoing private node parameters