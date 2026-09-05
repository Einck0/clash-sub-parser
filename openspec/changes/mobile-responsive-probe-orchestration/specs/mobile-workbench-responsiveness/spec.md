## Purpose

Defines the responsive and accessible Workbench behavior required to operate CSP safely on phones, tablets, and desktop displays without losing any management capability.

## ADDED Requirements

### Requirement: Responsive Workbench shell and navigation
The Workbench SHALL provide a desktop sidebar at viewport widths of 768px or greater and an accessible mobile navigation control below 768px. The mobile navigation control and all primary mobile navigation targets MUST have a minimum 44px by 44px hit area. The application shell and route content MUST prevent document-level horizontal scrolling at every supported viewport.

#### Scenario: Narrow-screen navigation
- **WHEN** the viewport is 375px wide and a user activates the navigation control
- **THEN** the system opens a navigation panel without obscuring its close control or causing horizontal document overflow

#### Scenario: Desktop navigation
- **WHEN** the viewport is 1024px wide
- **THEN** the system displays the persistent Workbench sidebar and does not render a duplicate mobile navigation control

### Requirement: Adaptive overlay presentation
All shared drawers and modal workflows SHALL preserve dialog semantics, focus lifecycle, Escape closing, explicit close control, background scroll locking, and safe-area padding. At viewport widths below 640px, a secondary right-side panel SHALL present as a bottom sheet; a left navigation panel SHALL remain an edge drawer. An optional drag-to-dismiss gesture MUST close only after a documented vertical displacement or velocity threshold and MUST NOT convert gestures originating on editable controls into dismissal.

#### Scenario: Mobile bottom sheet
- **WHEN** a user opens a node detail, preview, editor, or confirmation overlay at 375px width
- **THEN** the overlay appears from the bottom, exposes an explicit 44px close target, respects the bottom safe area, and returns focus to its trigger when closed

#### Scenario: Keyboard dismissal
- **WHEN** a dialog or drawer is open and the user presses Escape
- **THEN** the system closes the active overlay and restores focus without scrolling the background document

### Requirement: Mobile-preserving management views
Subscriptions, policy groups, proxy chains, rules, and settings SHALL provide a single-column or safely wrapping mobile presentation while retaining all existing create, edit, refresh, preview, enable, disable, reorder, delete, filter, and save operations. Fields and action controls MUST remain visible and reachable without a horizontal page scroll.

#### Scenario: Mobile settings editor
- **WHEN** a user opens Settings at 375px width
- **THEN** every quality-control configuration field and save action is readable, vertically reachable, and retains its unit or help text

#### Scenario: Mobile rule management
- **WHEN** a user opens Rules at 375px width
- **THEN** the page uses the Workbench semantic theme and exposes category, search, draft, reorder, and save operations without relying on desktop-only multi-column controls

### Requirement: NodeLedger mobile rendering and shared operation state
NodeLedger SHALL use a mobile-appropriate compact list or card renderer below 640px while retaining the same query, selected-node set, current results, probe actions, status, and detail workflow as the desktop renderers. A mobile row SHALL expose node name, protocol, latest quality status or latency, and reachable actions for detail and single-node quality check; additional data MAY appear in the detail sheet.

#### Scenario: Mobile batch selection and probe
- **WHEN** a user selects filtered nodes and starts a batch quality check at 375px width
- **THEN** the selected-node state is preserved, the progress state remains visible, and actions do not overflow the viewport

#### Scenario: Renderer switch consistency
- **WHEN** the same NodeLedger query is viewed at 639px and then 768px
- **THEN** the selected nodes, filters, probe records, and currently available operations remain semantically consistent
