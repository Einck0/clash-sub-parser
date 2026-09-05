## Purpose

Ensure every CSP client-side navigation target has a useful destination or a recoverable, accessible fallback instead of an empty application surface that looks like a failed load.

## ADDED Requirements

### Requirement: Quality-control route remains reachable
The workbench SHALL resolve `/probe` to the existing Node Ledger quality-control experience without changing its node, probe, filter, batch-control, or export behavior.

#### Scenario: Direct quality-control navigation
- **WHEN** an authenticated user opens `/probe` directly
- **THEN** the workbench renders the Node Ledger quality-control experience with a visible page title and its normal loading, data, or error state

#### Scenario: Existing node route remains compatible
- **WHEN** an authenticated user opens `/nodes`
- **THEN** the workbench renders the same Node Ledger experience without redirecting existing bookmarks through an incompatible route

### Requirement: Unmatched client routes recover visibly
The workbench SHALL render an accessible not-found recovery view for an unmatched client-side path rather than an empty router outlet.

#### Scenario: Unknown route
- **WHEN** an authenticated user opens a path not owned by the workbench
- **THEN** the page displays a not-found explanation and a keyboard-accessible action that returns to the Node Ledger

### Requirement: Header metrics distinguish unknown from zero
The workbench header SHALL not present unavailable node/probe data as a factual zero.

#### Scenario: Initial data lifecycle
- **WHEN** the header data source has not completed its initial load
- **THEN** each affected metric displays a loading placeholder or is intentionally hidden

#### Scenario: Loaded empty dataset
- **WHEN** the Node Ledger has completed loading and contains no nodes
- **THEN** the header may display zero and the content area presents the existing actionable empty state
