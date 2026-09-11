## Purpose

Provide a safe, observable admission funnel that discards invalid or unreachable endpoint candidates before allocating the isolated per-node proxy runtime used for capability validation.

## ADDED Requirements

### Requirement: Preflight is explicitly bounded and semantically limited
The system SHALL validate a candidate’s server and port before deep probing with a direct TCP connection subject to a configured, bounded timeout and bounded concurrency. A preflight success SHALL mean only that the probe host opened a TCP connection to the configured endpoint; it MUST NOT be reported as proxy reachability, egress identity, geographic location, media or AI access, speed, or capability qualification.

#### Scenario: Endpoint admits a deep probe
- **WHEN** a syntactically valid candidate opens a TCP connection within the configured preflight timeout
- **THEN** the candidate is admitted once to the bounded deep-probe queue and no capability result is created until the isolated proxy probe completes

#### Scenario: Endpoint does not admit a deep probe
- **WHEN** a valid candidate times out or has a transport error during preflight
- **THEN** no isolated proxy runtime is allocated for that candidate and the returned batch outcome is respectively `preflight_timeout` or `preflight_error`

#### Scenario: Candidate cannot form a safe target
- **WHEN** a candidate has no usable server or has a port outside the valid TCP range
- **THEN** it is reported as `preflight_invalid` without opening a socket or allocating a proxy runtime

### Requirement: Deep probes retain isolated egress semantics
The system SHALL perform identity, media, AI, transport-through-proxy, and speed checks only through the candidate’s dedicated explicit proxy path. Probe HTTP clients MUST disable ambient environment-proxy inheritance, and a direct preflight outcome MUST NOT be substituted for any deep-probe result.

#### Scenario: Preflight and deep-probe sources differ
- **WHEN** a candidate passes direct preflight but its isolated proxy transport check fails
- **THEN** the final candidate outcome is the deep-probe failure and no successful capability result is inferred from preflight

#### Scenario: Ambient proxy variables are present
- **WHEN** a deep probe runs on a host with proxy environment variables
- **THEN** its provider requests use only the candidate’s explicit runner endpoint and do not inherit the host proxy configuration

### Requirement: Admission and result ownership remain singular
The system SHALL retain the existing probe-result store as the only persisted probe-result authority. Preflight observations SHALL be transient batch outcomes or explicitly scoped aggregate telemetry; the system MUST NOT introduce a second result table, shadow ledger, dual-write result flow, or client-side fallback that reclassifies a preflight result as a deep-probe result.

#### Scenario: Batch includes rejected and admitted candidates
- **WHEN** a batch contains both preflight-rejected and deep-probe-admitted candidates
- **THEN** each candidate has exactly one terminal batch outcome and only completed deep-probe results are eligible for the existing persisted capability result path

### Requirement: Funneling has bounded work and cancellation
The system SHALL bound candidate preflight work and deep-probe work independently. Cancelling a request or exhausting its declared batch deadline SHALL stop waiting for unstarted work, cancel active tasks safely, release runner resources, and return outcomes for completed or terminally cancelled candidates without leaking proxy processes or loopback ports.

#### Scenario: Batch deadline expires
- **WHEN** the configured batch deadline expires while candidates are queued or running
- **THEN** unstarted candidates are reported as cancelled or not-admitted, active runner contexts are exited, and the next batch can acquire the released capacity
