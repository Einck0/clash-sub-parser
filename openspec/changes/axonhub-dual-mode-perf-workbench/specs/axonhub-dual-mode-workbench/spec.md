## Purpose

Defines the semantic dual-mode surfaces and high-frequency control behavior that let operators scan and act on the existing CSP workbench comfortably without changing any node, probe, routing, or export behavior.

## ADDED Requirements

### Requirement: AxonHub semantic light and dark themes
The workbench SHALL render both `data-theme="light"` and `data-theme="dark"` through one complete semantic token vocabulary for canvas, panel, card, raised, inset, border, primary text, muted text, focus, shadow, accent, and status roles. Light canvas SHALL be `#F8FAFC` with pearl-white `#FFFFFF` content surfaces; dark canvas SHALL be `#0F172A` with Slate surfaces in the `#0F172A` through `#1E293B` range. Dark mode MUST NOT use `#000000`, `#08090A`, or another near-black void as its canvas.

Success and verified states SHALL use Emerald or Teal semantic treatments, informational states SHALL use Teal or Cyan, warning SHALL remain amber, and destructive states SHALL remain red. Textual status and numeric evidence SHALL remain visible beside color treatments. A theme switch SHALL update the current interface without changing its data, route, filter state, selected nodes, or in-progress operation state.

#### Scenario: Operator changes theme while inspecting a filtered ledger
- **WHEN** an operator switches between light and dark themes with Node Ledger filters or a drawer open
- **THEN** the visible surfaces, text, focus treatment, status accents, and overlay adapt to the selected semantic theme while the same filters, selection, drawer target, and probe data remain active

### Requirement: Shared action hierarchy and reachable controls
Every CSP action control SHALL use one of four semantic variants: Primary for the single affirmative operation in a context, Secondary for reversible supporting actions, Ghost for low-emphasis contextual actions, and Danger for destructive operations that already require confirmation. Controls SHALL provide a visible focus indicator, disabled and loading states, and at least a 44 by 44 CSS-pixel target at viewport widths below 640px; controls at or above 640px MAY use a compact visual surface only if their effective pointer target remains at least 40 by 40 CSS pixels.

#### Scenario: Operator triggers and cancels a batch probe on a narrow screen
- **WHEN** the Node Ledger is rendered at 375px width and a batch probe is available or running
- **THEN** its primary start action, danger stop action, loading indicator, and any enabled supporting action are reachable through visible controls with the defined target geometry and keyboard focus feedback

### Requirement: Node Ledger action bar and segmented controls preserve behavior
The Node Ledger SHALL group batch and record-management actions into a labelled Action Bar and present mutually exclusive view or choice controls as accessible segmented controls. A segmented control SHALL expose a programmatic label, an explicit active state, and a keyboard-reachable control for every available choice. This presentation SHALL retain all existing source, protocol, health, chain, country, media, speed, selection, batch-probe, clear-data, table-view, grid-view, mobile-list, drawer, and virtual-scroll behavior.

#### Scenario: Operator changes the ledger view through a segmented control
- **WHEN** an operator selects the table or grid option through pointer or keyboard interaction
- **THEN** exactly one option is exposed as active and the existing ledger view changes without resetting filters, selection, loaded summaries, or drawer detail state

### Requirement: Detail evidence loading remains explicit and non-destructive
When the Node Ledger opens a drawer for a summary-only result, it SHALL render a visible loading state while requesting that node's on-demand detail. If the request fails or the result is unavailable, the drawer SHALL preserve the summary values, explain that diagnostics are unavailable, and offer a retry that does not execute a probe, clear data, modify a chain, or mutate persisted records.

#### Scenario: Detail request is unavailable
- **WHEN** an operator opens a node drawer and the detail endpoint responds with an error
- **THEN** the drawer keeps the node identity and summary values visible, announces that detail evidence is unavailable, and provides a retry action without causing a probe or any other data mutation
