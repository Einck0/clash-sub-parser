## Purpose

Provide one observable design-governance contract for the CSP and Eindash control planes so their visual tokens, dialogs, drawers, responsive overlays and release gates remain consistent, accessible and verifiably within viewport bounds.

## ADDED Requirements

### Requirement: Semantic design tokens are the sole visual source
The system SHALL expose one semantic token vocabulary for canvas, surface, raised surface, inset surface, borders, main and muted text, accent, status colors, focus, spacing, radii, elevation and motion. Domain styles SHALL consume that vocabulary and SHALL NOT retain legacy aliases such as `--brand`, `--ink`, `--surface`, `--bg-0`, `--bg-1` or a parallel global theme. A domain component SHALL NOT create a private visual token system or hard-code an overlay's color, radius, elevation, width or z-index.

#### Scenario: Legacy token regression is rejected
- **WHEN** a source audit encounters a legacy token reference or a second global token root outside the approved theme entry
- **THEN** the audit SHALL fail and identify the source location before the change is accepted

#### Scenario: Existing feature styling is migrated
- **WHEN** a legacy CSP management view or modal is rendered after the migration
- **THEN** it SHALL retain its existing non-SCRIPT workflow while consuming the semantic token vocabulary rather than the removed aliases

### Requirement: Application overlays use the shared modal primitive
The system SHALL provide an application modal primitive accepting only `sm`, `md` or `lg` as public size values. On desktop, those values SHALL resolve to 448px, 640px and 960px maximum content widths respectively, constrained by a 32px viewport gutter. Every modal SHALL use an opaque shared backdrop, shared dialog z-index, 8px radius, a maximum usable height of `calc(100dvh - 64px)`, a separately scrolling body, an accessible title, Escape close, focus containment, background scroll lock and focus restoration. A modal consumer SHALL NOT override those dimensions with component-local CSS or utility width classes.

#### Scenario: A normal modal opens on desktop
- **WHEN** a user opens a modal at a viewport width of 1024px or greater
- **THEN** its computed content width SHALL not exceed its declared scale or the viewport gutter constraint and its body SHALL be the only scrollable content region when content exceeds the maximum height

#### Scenario: A modal is usable by keyboard
- **WHEN** a keyboard user opens a modal and presses Escape or tabs through its controls
- **THEN** initial focus SHALL enter the modal, focus SHALL remain contained until close, Escape SHALL close it, and focus SHALL return to the trigger

### Requirement: Application drawers follow one responsive contract
The system SHALL provide an application drawer primitive with a desktop width of 480px for right-side secondary panels and 320px for left navigation. At viewport widths below 640px, a right-side drawer SHALL render as a bottom sheet with a 16px top radius, maximum height `min(88dvh, 720px)`, safe-area padding, a visible drag affordance and independently scrolling body; a left navigation drawer SHALL remain an edge drawer. All drawer close controls SHALL be keyboard accessible and have a 44px minimum touch target.

#### Scenario: Secondary panel adapts on phone
- **WHEN** a user opens a right-side secondary panel at a 375px viewport
- **THEN** the panel SHALL be a bottom sheet within the viewport, expose its title and close control, and SHALL NOT create document-level horizontal scrolling

#### Scenario: Navigation remains an edge drawer
- **WHEN** a user opens mobile navigation at a 375px viewport
- **THEN** the navigation panel SHALL retain left-edge presentation rather than using the secondary-panel bottom sheet treatment

### Requirement: Legacy overlays migrate without functional regression
Quick Export, Rule Import, Rule Presets, Node Group editing and manual-node editing SHALL use the shared modal or drawer primitive. Their current non-SCRIPT controls, validation, save/cancel outcomes, target selection, rule parsing, node-group editing and five-target export behavior SHALL remain available after migration.

#### Scenario: Quick Export retains target distribution choices
- **WHEN** a user opens Quick Export and selects a supported target
- **THEN** merged and single-subscription choices, the supported target selector, generated URL, client scheme and QR payload SHALL remain available inside the shared overlay

#### Scenario: Rule and node management retains save semantics
- **WHEN** a user opens Rule Import, Rule Presets, Node Group editing or manual-node editing
- **THEN** the original data-entry and apply or save behavior SHALL remain reachable without relying on the removed legacy backdrop classes

### Requirement: Rendered geometry is a release gate
The frontend release gate SHALL run real browser assertions at 375px, 768px, 1024px and 1440px for core routes and each shared-overlay consumer. The assertions SHALL fail on document horizontal overflow, overlay bounds outside the visual viewport, modal size values outside the declared scale, drawer presentation inconsistent with the viewport contract, inaccessible dialog semantics, or a table/container whose intended local overflow leaks to the document.

#### Scenario: A geometry regression is caught
- **WHEN** a modal or drawer exceeds the visual viewport or a document has `scrollWidth` greater than `clientWidth` at a covered viewport
- **THEN** the browser gate SHALL fail with the route, viewport, consumer and measured geometry

#### Scenario: CSP and Eindash share the governance baseline
- **WHEN** Eindash evolves its CPA console overlays after this governance change
- **THEN** its design artifacts SHALL declare the same token ownership, modal scale, desktop drawer width, mobile bottom-sheet behavior and real-browser geometry gate before implementation is approved
