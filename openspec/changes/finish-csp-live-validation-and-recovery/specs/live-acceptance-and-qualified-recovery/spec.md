## ADDED Requirements

### Requirement: Authenticated bounded semantic acceptance

Production acceptance SHALL use legally managed credentials and existing configured subscriptions/nodes only. It SHALL verify response bodies, not only HTTP status, and preserve security constraints and old inventory on refresh failure.

#### Scenario: Subscription is unreachable
- **WHEN** a bounded refresh fails
- **THEN** prior inventory remains intact and the response correctly distinguishes fetch failure from deletion or successful zero-node update

#### Scenario: Real node probe
- **WHEN** a representative existing node is probed within the approved budget
- **THEN** evidence records actual handshake stage, reachability and body/result consistency without claiming every node must be online

### Requirement: Evidence-qualified restoration

Historical node restoration SHALL require sufficient causal evidence, unique identity, currently valid source and exclusion of manual disable/deletion. Ambiguous candidates SHALL remain unchanged and be reported by category.

#### Scenario: Historical heuristic candidate
- **WHEN** a node merely resembles a former refresh failure candidate
- **THEN** it is not restored absent sufficient causal and user-intent evidence

### Requirement: Reviewed production write safety

Every production write SHALL follow independently reviewed plans, verified rollback and immediate hot backup, using exact preconditions and bounded allowlists.

#### Scenario: Stale recovery plan
- **WHEN** production identity or state differs from the reviewed allowlist
- **THEN** restoration aborts without changing nodes

### Requirement: External prerequisites stay explicit

Missing legal credentials or causal audit history SHALL be reported as a scoped external decision, not bypassed or disguised as completion.

#### Scenario: No legal credential available
- **WHEN** no approved credential can be obtained from existing managed sources
- **THEN** protected operations remain unexecuted and the operator receives actionable secure authorization choices
