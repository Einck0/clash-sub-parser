## Purpose

Provide a high-volume NodeLedger workspace that preserves reliable node operations, rich diagnostics, and complete filtering while keeping thousands of inventory rows responsive.

## ADDED Requirements

### Requirement: Complete ledger inventory and diagnostic presentation
The system SHALL load the effective node ledger and persisted probe results from the existing APIs and present every node with its identity, subscription, protocol, endpoint, effective dialer chain, probe state, latency, speed, egress location, and supported media or AI capabilities when available.

#### Scenario: Ledger with probe records loads
- **WHEN** the NodeLedger workspace loads successfully
- **THEN** each ledger node is associated with its persisted probe record by its stable node name or node key and displays the available diagnostic fields

#### Scenario: Probe records are unavailable
- **WHEN** the probe-results request fails while the ledger request succeeds
- **THEN** the workspace displays the ledger as untested and permits a later refresh rather than discarding the inventory

### Requirement: Multi-dimensional inventory discovery
The system SHALL allow users to combine text, subscription, protocol, probe status, country or region, dialer-chain state, minimum speed, media or AI-unlock, and sort filters. Text matching SHALL cover node identity, endpoint, subscription, protocol, chain, group membership, and available probe egress metadata.

#### Scenario: Combined media and speed filter
- **WHEN** a user selects multiple media or AI capabilities and a minimum speed
- **THEN** only nodes satisfying every selected capability and the threshold remain in the result

#### Scenario: Metric shortcut filter
- **WHEN** a user activates a healthy, fast, or chained metric
- **THEN** the corresponding complete filter state is applied and the metric reports its active state

### Requirement: Virtualized table preserves complete result semantics
The system SHALL use a responsive dense-table presentation for large filtered inventories without changing the result set, selected-node set, sorting, or actions according to which rows are mounted in the viewport. The workspace SHALL retain an accessible card presentation for smaller or alternate views without rendering an unbounded card list.

#### Scenario: Full filtered probing from virtual table
- **WHEN** no explicit selection exists and a user starts a batch probe after applying filters
- **THEN** the probe target set contains every filtered node, including nodes outside the virtual table viewport

#### Scenario: Selected-node probing from virtual table
- **WHEN** a user selects nodes in different virtual scroll positions and starts a batch probe
- **THEN** the target set contains exactly the selected nodes regardless of whether they are currently mounted

### Requirement: Node probe operations remain controllable
The system SHALL allow a user to start a single-node probe, start a batch full-protocol probe for selected nodes or all filtered nodes, show cumulative progress and result counts, stop scheduling subsequent batch chunks, and clear persisted probe records only after explicit confirmation.

#### Scenario: Batch probe stopped between chunks
- **WHEN** a user stops an active batch probe
- **THEN** the current completed results remain visible and no later chunk is scheduled

#### Scenario: Clear diagnostic history
- **WHEN** a user confirms clearing probe data
- **THEN** persisted probe records are cleared and the workspace immediately displays every node as untested

### Requirement: Node inspection and dialer-proxy management
The system SHALL provide a detail surface showing node configuration and diagnostics, allow a user to trigger a single-node probe, and allow the user to create, replace, or clear a node-level dialer-proxy binding using the existing proxy-chain validation API.

#### Scenario: Replace an existing node-level binding
- **WHEN** a user saves a new dialer proxy for a node that has a node-level binding
- **THEN** the previous node-level binding is removed before the new binding is created and the refreshed ledger shows the effective chain

#### Scenario: Clear inherited versus node-level binding
- **WHEN** a node has an inherited dialer chain
- **THEN** the workspace identifies its source and does not offer deletion of the inherited binding as if it were a node-level binding

### Requirement: Resilient and accessible workbench states
The system SHALL expose loading, empty, request-error, disabled-action, confirmation, focus, and responsive-layout states without hiding established functionality. Interactive controls SHALL have accessible labels or discernible text and keyboard-reachable focus treatment.

#### Scenario: Empty filtered result
- **WHEN** a filter combination has no matches
- **THEN** the workspace displays an empty state and a control to reset the active filters
