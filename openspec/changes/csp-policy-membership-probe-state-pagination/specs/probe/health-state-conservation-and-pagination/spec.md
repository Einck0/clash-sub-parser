## Purpose

确立探针健康状态权威性判定模型，消除辅助观测对整体健康的虚假回退，规范未检测与不确定状态分类及其互斥守恒指标，并提供全池数据库级健康状态分页查询。

## ADDED Requirements

### Requirement: Baseline-Authoritative Health Evaluation
The system SHALL evaluate overall node health solely based on fresh baseline probe observations matching the current connection revision, and SHALL NOT fall back auxiliary capability availability (such as AI or streaming services) to mark a node as healthy.

#### Scenario: Node with available AI observation but missing baseline probe
- **WHEN** a node has successful AI probe observations but lacks a fresh baseline observation for its current connection revision
- **THEN** the system SHALL classify the node health as undetermined rather than healthy, preventing dangerous routing through unverified nodes

#### Scenario: Node with stale or revision-mismatched baseline observation
- **WHEN** a node connection credentials have been updated or its baseline observation exceeds freshness TTL
- **THEN** the system SHALL classify the node health as undetermined, requiring re-probe before regaining healthy status

### Requirement: Mutually Exclusive Conservation of Pool Health Counts
The system SHALL maintain strict mutually exclusive conservation across all health status counters in `ProbePoolStatus`: `total_count == healthy_count + degraded_count + unavailable_count + undetermined_count + untested_count`, where `available_count == healthy_count + degraded_count`.

#### Scenario: Node pool status aggregation with mixed probe states
- **WHEN** client queries `GET /api/v1/probes/pool`
- **THEN** the returned counters SHALL sum exactly to `total_count`, with `untested_count` counting only nodes with zero observations and `undetermined_count` counting nodes with inconclusive or stale observations

### Requirement: Server-Side Health Category Filtering for Nodes
The system SHALL support filtering nodes by underlying health category (`healthy`, `degraded`, `unhealthy`, `undetermined`, `untested`) at the database repository level across the full query scope.

#### Scenario: Filtering nodes by health status on node ledger
- **WHEN** client requests `/api/v1/nodes?health_status=undetermined&page=1&page_size=50`
- **THEN** the server SHALL query the full scope using database predicates and return accurately paginated items and matching total count without calculating counts on client-side memory
