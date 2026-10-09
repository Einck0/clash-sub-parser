## Purpose

规范策略组成员来源（显式连接边 vs 动态全池规则）的判定语义与交互说明，实现策略组列表与候选成员选择的服务端分页与异步检索，并将完整拓扑全图与分页列表解耦。

## ADDED Requirements

### Requirement: Deterministic Membership Resolution Semantics
The system SHALL adhere to the invariant membership resolution algebra: explicit node/group edges SHALL be evaluated as candidates filtered by group filter and parent filter propagation; dynamic full-pool evaluation SHALL only execute when no edges exist and a group filter is non-empty.

#### Scenario: Group with explicit edges evaluates only connected candidates
- **WHEN** a policy group contains one or more explicit node edges or child group edges
- **THEN** the resolver SHALL apply the group filter strictly to the connected candidates, and UI SHALL clearly indicate that group filters apply only to explicit targets rather than the global pool

#### Scenario: Dynamic group evaluates global admitted pool
- **WHEN** a policy group contains zero edges and has a non-empty node filter
- **THEN** the resolver SHALL dynamically scan all globally admitted nodes, and UI SHALL inform the user that this group automatically admits matching nodes from the entire pool

### Requirement: Server-Side Pagination and Search for Policy Groups
The system SHALL support server-side pagination (default 50, maximum 100) and case-insensitive keyword search for policy groups via `GET /api/v1/policies/groups`.

#### Scenario: Paginated query returns bounded groups with total count
- **WHEN** client requests `/api/v1/policies/groups?page=1&page_size=50`
- **THEN** the response SHALL return up to 50 policy groups alongside accurate `page`, `page_size`, and `total` metadata

#### Scenario: Keyword search filters policy groups
- **WHEN** client provides a search query `?search=proxy`
- **THEN** the response SHALL filter groups by name and group type, returning only matching items and the matching total count

### Requirement: Asynchronous Candidate Node Selection Without Silent Truncation
The policy editor SHALL provide asynchronous search and paginated retrieval for candidate nodes, preventing silent truncation beyond 100 nodes.

#### Scenario: Selecting nodes from a pool exceeding 100 nodes
- **WHEN** an operator selects nodes to attach as edges in the policy editor
- **THEN** the selector SHALL allow searching and paging through the entire active inventory, ensuring all nodes across the entire pool are discoverable

### Requirement: Topology Graph Decoupled from Paginated Group Cards
The policy management interface SHALL separate the full topology graph representation from the paginated group card list.

#### Scenario: Viewing topology when total groups exceed single page size
- **WHEN** total policy groups exceed single page size (e.g. > 50 groups)
- **THEN** the validation and topology view SHALL reflect the entire interconnected graph without truncating unpaged groups
