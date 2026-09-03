## Purpose

将节点选择、代理组解析和目标格式渲染编译为同一个确定性结果，使预览、YAML、Script.js 和订阅输出不再各自解释配置。

## ADDED Requirements

### Requirement: Single canonical resolution result
The system SHALL resolve a configuration revision into one canonical ordered node and group graph before rendering any user-visible output. Every output target SHALL consume that same resolution result.

#### Scenario: Preview and generated YAML agree
- **WHEN** an operator previews a configuration and generates YAML without changing configuration or policy inputs
- **THEN** both outputs contain the same ordered eligible node set and resolved group membership

### Requirement: Explicit membership expressions
The system SHALL store proxy-group membership as ordered, explicit expressions with declared inclusion, exclusion, reference, and policy semantics. Derived mirrors and mutable database IDs SHALL NOT be independent sources of group-membership truth.

#### Scenario: Group reference resolves after identifier migration
- **WHEN** a configuration bundle is imported into a database whose surrogate row IDs differ from the exporting database
- **THEN** equivalent group references resolve to the intended logical groups and produce the same membership result

### Requirement: Deterministic compilation
For an identical configuration revision, inventory generation, policy revision, compiler version, and requested target, the system SHALL produce byte-stable semantic output and a reproducible compilation fingerprint.

#### Scenario: Repeat compilation
- **WHEN** the same target is compiled twice with unchanged inputs
- **THEN** the returned compilation fingerprint and normalized result are identical

### Requirement: Structured diagnostics
The system SHALL emit structured diagnostics for unresolved references, cycles, unsupported node formats, empty required groups, and excluded nodes. Diagnostics SHALL identify safe logical references without revealing secrets.

#### Scenario: Invalid group reference
- **WHEN** a group expression references a missing group
- **THEN** compilation fails or returns invalid status according to the requested mode and includes a diagnostic with the expression location and reason

### Requirement: Output policy is explicit
The system SHALL record which inventory generation and policy revision were used for each generated artifact or preview, and SHALL distinguish a policy exclusion from a parser or transport failure.

#### Scenario: Capability policy excludes node
- **WHEN** a node is excluded because its latest eligible observation does not satisfy a media or speed policy
- **THEN** diagnostics identify the policy exclusion and do not label the node as an invalid proxy