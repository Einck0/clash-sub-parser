## Purpose

为订阅、手工节点、节点台账、编译输出和探测提供同一份可追溯的节点库存，使刷新不会因名称变化或 JSON 字段镜像而丢失身份与来源。

## ADDED Requirements

### Requirement: Stable node identity and provenance
The system SHALL assign every accepted node an opaque stable identifier and SHALL retain its source kind, source configuration reference, source generation, normalized payload fingerprint, and lifecycle state. Node display names SHALL NOT be used as the persistence key.

#### Scenario: Refresh retains unchanged node identity
- **WHEN** a source refresh yields a node with the same normalized protocol payload but a renamed display label
- **THEN** the inventory retains the existing stable identifier, records the new label and source generation, and preserves associated observations

#### Scenario: Same name from different sources
- **WHEN** two enabled sources contain nodes with the same display name but different normalized payloads
- **THEN** the inventory stores two independently addressable nodes with their respective provenance

### Requirement: Refresh generation is atomic
The system SHALL publish a source refresh as one generation only after parsing, validation, normalization, and reconciliation complete. A failed refresh SHALL retain the last published generation as the active source state.

#### Scenario: Malformed refresh does not empty inventory
- **WHEN** an existing source refresh returns malformed or unsupported content
- **THEN** the previous active generation remains selectable for compilation and the source records a structured failed outcome

### Requirement: Manual-node lifecycle
The system SHALL represent manually managed nodes separately from fetched source generations while exposing them through the same inventory and compilation interfaces.

#### Scenario: Manual node survives source removal
- **WHEN** a user removes or disables the source that supplied other nodes
- **THEN** a manually managed node remains in the inventory and remains eligible for a policy that selects it

### Requirement: Sensitive source data isolation
The system SHALL NOT expose source URLs, credentials, or raw authorization material in node inventory listings, diagnostics, exports, logs, or probe observations.

#### Scenario: Operator views the node ledger
- **WHEN** an operator requests an inventory or diagnostic view
- **THEN** the response includes safe provenance labels and identifiers without disclosing a source secret