## Purpose

Defines CSP's source-visible shadcn-vue component foundation so its accessible workbench controls share one inspectable token, primitive, interaction, and responsive contract.

## ADDED Requirements

### Requirement: Inspectable shadcn-vue component foundation

The frontend SHALL use the current shadcn-vue component-distribution convention with project-owned component source, a declared component manifest, a shared class-composition utility, semantic CSS variables, Tailwind utilities, Lucide icons, and current Reka UI v2 accessible primitives where behavior needs a primitive.

The frontend MUST NOT depend on a runtime-hosted theme, a remote component loader, opaque generated styling, a second parallel UI kit, deprecated Radix Vue packages, or application-level imports from a legacy alias after the affected primitive has migrated. The component source and its package versions SHALL be locked in the project dependency manifest.

#### Scenario: A developer can inspect every shared primitive locally

- **WHEN** a developer inspects the frontend repository after installation
- **THEN** the component manifest, shared class utility, semantic token definitions, installed dependency versions, and source for each adopted primitive are available locally without a network request at application runtime

#### Scenario: Overlay primitive preserves accessible lifecycle

- **WHEN** an operator opens and closes a dialog, sheet, or drawer with pointer or keyboard
- **THEN** it has dialog semantics, traps and restores focus, closes with Escape, locks background scrolling while open, and exposes no duplicate focusable overlay shell

### Requirement: Semantic industrial workbench presentation

The default dark workbench SHALL use a Void `#08090a` canvas, `#0d0f12` panel, and `#121417` card elevation roles. Color, typography, spacing, radius, border, focus, motion, z-index, destructive, warning, success, and informational values SHALL be semantic tokens rather than per-component color ownership.

A shared primitive SHALL provide visible default, hover, focus-visible, disabled, loading, selected, destructive, and reduced-motion states as applicable. Status color SHALL supplement an explicit textual or numeric state and MUST NOT be the only signal.

#### Scenario: Dense node state remains readable without relying on color

- **WHEN** a Node Ledger row represents untested, unknown, failed, timeout, or healthy status
- **THEN** it contains a distinct text label or numeric value in addition to any color or icon treatment and maintains its semantic state in table, card, and mobile views

#### Scenario: Reduced motion preserves operation

- **WHEN** the browser requests reduced motion
- **THEN** dialogs, sheets, drawers, dropdowns, tooltips, selections, and status transitions remain usable without durable transform, opacity, blur, or animation effects

### Requirement: Responsive control and overlay containment

Across 375px, 768px, 1024px, and 1440px viewport classes, workbench pages and opened overlays SHALL have no document-level horizontal overflow. Actionable mobile controls SHALL provide at least a 44px effective target. The desktop Node Ledger table SHALL preserve its 36px row-density contract, and overlays SHALL remain within their defined accessible geometry.

#### Scenario: Node Ledger remains contained at every declared viewport

- **WHEN** fixture-backed Node Ledger data is rendered at 375px, 768px, 1024px, or 1440px in table, card, and mobile presentations
- **THEN** document scroll width does not exceed client width, the relevant controls remain reachable, and status labels remain visible

#### Scenario: Overlay geometry remains contained on mobile

- **WHEN** an operator opens a shared dialog or right-side drawer at 375px width
- **THEN** its dialog bounds stay within the viewport, its title and close control are reachable, and keyboard close/focus restoration remain functional
