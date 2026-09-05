## Purpose

Define the observable dark visual hierarchy and interaction quality of the CSP Workbench so dense subscription, node-quality and routing data stays fast to scan, accessible, responsive and functionally unchanged.

## ADDED Requirements

### Requirement: CSP renders a layered precision-control-plane surface system
The CSP Workbench SHALL render its default dark interface using a semantic surface hierarchy with void canvas `#08090a`, panel surface `#0d0f12`, card surface `#121417`, raised/hover surfaces, hairline containment, restrained inset highlights, primary and muted text, focus, accent and semantic status tokens. The system SHALL apply cool indigo only to selected/primary interaction and SHALL reserve green, amber and red for semantic probe or action states. It SHALL NOT present a flat Slate-blue canvas, generic opaque blue buttons, decorative multi-colour gradients, or a second component-local color/elevation vocabulary.

#### Scenario: A dense workbench route renders
- **WHEN** a user opens a CSP workbench route in the default dark theme
- **THEN** the document canvas, major panels, cards, controls and data rows SHALL expose distinguishable semantic elevation while preserving readable primary, secondary and disabled text contrast

#### Scenario: A semantic state changes
- **WHEN** a node probe, streaming/AI capability, latency or destructive action changes state
- **THEN** the affected indicator SHALL communicate that state through the governed semantic status token and accompanying text or accessible label rather than color alone

### Requirement: Node Ledger provides an integrated high-density control surface
The Node Ledger SHALL combine its search, region selection, media/AI capability filters, handshake/latency constraints, active-filter summary, selection count and non-destructive actions into a compact responsive control surface. Its ledger rows and mobile representation SHALL preserve the existing card/table views, multi-dimensional filtering, batch operations, node inspection, controlled probing, export-related navigation and all data values while presenting country badges, streaming/AI evidence and latency as consistently tokenized, quickly scannable visual states.

#### Scenario: An operator filters node quality evidence
- **WHEN** an operator selects one or more region, media/AI or latency filters and enters a query
- **THEN** the visible ledger result set, active-filter feedback, clear/reset behavior and selection state SHALL retain their existing functional semantics without document horizontal overflow

#### Scenario: A ledger row is inspected
- **WHEN** an operator opens a node detail from either table or mobile ledger presentation
- **THEN** the detail surface SHALL retain probe evidence, outbound identity, region, streaming/AI evidence, dialer-chain controls and existing permitted actions in the governed drawer presentation

### Requirement: Governed micro-interactions preserve accessibility and user preference
Interactive CSP controls SHALL provide targeted, tokenized transitions for hover, active, selected, open and close states; focus-visible states SHALL remain clearly perceivable. Shared Modal and Drawer backdrops MAY use a restrained backdrop blur only within the governed overlay primitive, while all other background blur/filter effects SHALL be rejected. When `prefers-reduced-motion: reduce` is active, non-essential opacity, transform and blur transitions SHALL be removed or reduced without preventing state change, keyboard navigation or Escape dismissal.

#### Scenario: A pointer user hovers an interactive control
- **WHEN** a pointer user hovers a button, filter chip, ledger row or drawer/modal trigger
- **THEN** the control SHALL use a targeted visual response that does not change layout dimensions or obscure its current semantic state

#### Scenario: A reduced-motion user opens an overlay
- **WHEN** the user agent reports `prefers-reduced-motion: reduce` and a user opens or closes a governed overlay
- **THEN** the overlay SHALL complete its state transition without sustained transform, opacity or blur animation and SHALL retain its keyboard/focus lifecycle

### Requirement: Visual quality is verified in a rendered browser
The frontend release gate SHALL render the Node Ledger, its filter surface and the governed overlays at 375px, 768px, 1024px and 1440px. The gate SHALL fail for document horizontal overflow, inaccessible focus/keyboard behavior, visual token leakage, ungoverned backdrop blur, missing semantic elevation on covered primitives, modal/drawer geometry regression, or loss of an existing non-SCRIPT Node Ledger/overlay workflow.

#### Scenario: A visual governance regression is introduced
- **WHEN** a covered component introduces an ungoverned color, elevation, backdrop blur, overlay geometry, layout-shifting interaction or a document-overflow condition
- **THEN** static or browser validation SHALL fail with the affected route/component and viewport or source location
