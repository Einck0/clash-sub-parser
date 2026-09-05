## Purpose

Keep CSP's dark workbench visually coherent and ensure its primary export and active settings controls remain immediately operable in ordinary desktop and mobile viewports.

## ADDED Requirements

### Requirement: Workbench uses one coherent color system
The workbench SHALL render application shell, pages, controls, and form surfaces using the shared semantic dark-console color system without a light page section appearing beneath a dark shell.

#### Scenario: Generate and settings visual continuity
- **WHEN** an authenticated user opens Generate or Settings
- **THEN** the page background, cards, controls, and text use compatible semantic surfaces and readable contrast

### Requirement: Primary export controls receive viewport priority
At a 1440×900 CSS pixel viewport, the Generate experience SHALL expose a current export address and its copy or client-import action without requiring vertical scrolling past decorative metric content.

#### Scenario: Desktop export availability
- **WHEN** the Generate route finishes loading at 1440×900
- **THEN** a user can see and activate a current export URL action in the initial viewport

### Requirement: Settings uses non-nested control grouping
Settings SHALL group related controls with spacing, headings, and semantic state rather than rendering each checkbox as an independently bordered card within a bordered parent.

#### Scenario: Authentication settings scanability
- **WHEN** an operator views authentication settings on desktop or mobile
- **THEN** each protected scope and its enabled/disabled dependency is identifiable without relying solely on nested borders or color

### Requirement: Responsive layout has no document-width overflow
At 375, 768, 1024, and 1440 CSS pixel viewports, core AuthGate, Generate, and Settings pages SHALL keep document scroll width no greater than viewport width and retain operable controls inside their viewport or controlled scroll region.

#### Scenario: Multi-viewport geometry gate
- **WHEN** the browser geometry regression suite runs against fixture data
- **THEN** it reports no horizontal document overflow and verifies the declared primary controls for each covered route
