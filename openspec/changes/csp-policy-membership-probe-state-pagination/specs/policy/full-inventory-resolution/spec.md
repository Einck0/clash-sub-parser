## Purpose

保障 CSP 策略树计算与订阅发布导出能够完整检索活跃节点池，消除分页默认限制（50/100）引起的截断缺陷，确保大规模节点池（250+ 节点）准确定位与导出。

## ADDED Requirements

### Requirement: Full Inventory Resolution for Validation and Publication
The system SHALL retrieve the complete active node set within enabled subscriptions without pagination truncation during policy validation and publication artifact compilation.

#### Scenario: Validation resolves all active nodes beyond default page size
- **WHEN** the active node inventory contains 90 or more nodes (such as Taiwan nodes at index 70-72 and Canada nodes at index 78-80, or a test fixture with 250 nodes)
- **THEN** policy validation and resolver snapshot provider SHALL load all 90+ nodes without applying the 50/100 limit, resolving matching groups without premature empty group diagnostics

#### Scenario: Publication export includes all active nodes beyond default page size
- **WHEN** client requests subscription publication export for Mihomo or other target formats
- **THEN** the publication pipeline SHALL compile artifacts against the complete admitted node inventory, ensuring all matching nodes past index 50 appear in the compiled configuration

### Requirement: Preservation of Admission Scope and Global Filters
The system SHALL preserve subscription scope, active-only constraints, notice exclusions, and IP risk policy admissions when fetching unpaginated node inventory.

#### Scenario: Excluded notices and inactive nodes remain filtered
- **WHEN** unpaginated node retrieval is executed for publication or policy validation
- **THEN** the system SHALL strictly filter out notice-type nodes, inactive nodes, and nodes outside enabled subscriptions, passing only fully admitted candidates to the resolver
