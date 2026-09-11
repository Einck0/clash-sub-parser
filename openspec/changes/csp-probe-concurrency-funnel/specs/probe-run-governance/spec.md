## Purpose

Keep scheduled and interactive probe execution bounded, observable, and recoverable while establishing an evidence-based decision gate for whether a separate Go probe engine is needed.

## ADDED Requirements

### Requirement: Periodic probe execution has a single local owner
Within one application process, the system SHALL allow at most one periodic probe run at a time. A skipped overlapping tick SHALL not start a duplicate inventory collection or deep-probe batch. The runtime status projection SHALL distinguish baseline initialization, running, successful completion, failure, and overlap skip without exposing candidate secrets.

#### Scenario: Periodic tick arrives during an active run
- **WHEN** the periodic scheduler fires while a periodic probe run owns execution
- **THEN** the second tick exits without creating another batch and the active run continues unchanged

#### Scenario: Scheduler restarts before the interval elapses
- **WHEN** a new scheduler process receives its first tick
- **THEN** it records a baseline and does not immediately replay a missed full probe run

### Requirement: Interactive and periodic work have independent bounded admission
The system SHALL cap deep-probe admission for interactive requests and periodic work using declared limits. A request MUST NOT create an unbounded coroutine fan-out or exceed its allocated deep-probe capacity. If the system cannot admit work within its declared deadline, it SHALL return a bounded overload or cancellation outcome rather than block unrelated API handlers indefinitely.

#### Scenario: Interactive batch exceeds capacity
- **WHEN** an interactive request supplies more candidates than it can admit before its deadline
- **THEN** admitted candidates run within the limit and remaining candidates receive a terminal non-admission outcome with an actionable aggregate summary

#### Scenario: Periodic run is active while an interactive request arrives
- **WHEN** a periodic run has consumed its allocated deep-probe capacity
- **THEN** the interactive request follows its independent bounded admission policy and does not create duplicate scheduler work or unbounded process creation

### Requirement: Aggregate telemetry is privacy-safe and comparable
The system SHALL expose or log only aggregate funnel telemetry needed to assess behavior: candidate totals, preflight outcome counts, admitted count, deep-probe terminal counts, queue wait or rejection counts, elapsed durations, and configured limits. It MUST NOT include node credentials, full configurations, provider bodies, or unredacted endpoints in telemetry.

#### Scenario: Batch completes
- **WHEN** a batch reaches a terminal state
- **THEN** the summary contains aggregate preflight and deep-probe counts plus elapsed time and configured limits, without credentials or raw node configuration

### Requirement: Go adoption remains evidence-gated
The system SHALL treat the Python preflight-plus-bounded-runner architecture as the only implementation authorized by this change. A full Go rewrite or a Go sidecar MUST NOT be introduced unless a versioned benchmark packet shows that the frozen Python acceptance thresholds fail under the same redacted fixture inventory and a Critic RFC refutation explicitly accepts the proposed replacement boundary.

#### Scenario: Python benchmark meets the frozen gate
- **WHEN** the benchmark harness completes within the declared admission, latency, resource, and correctness thresholds
- **THEN** no Go rewrite or sidecar task is eligible for dispatch

#### Scenario: Python benchmark misses the frozen gate
- **WHEN** the benchmark harness reproducibly misses a frozen threshold
- **THEN** the result is recorded as an evidence packet and a separate Critic-reviewed design decision is required before any Go implementation task can be proposed
