## Purpose

Defines an explanation-first and responsive presentation of node-quality evidence so operators can inspect why a platform was classified while retained capability filters continue to select only verified unlock outcomes.

## ADDED Requirements

### Requirement: Probe evidence explanation in all existing node-quality surfaces
The system SHALL present the normalized provider status, verdict, detected region when available, checked timestamp, confidence, and a concise sanitized reason in NodeLedger details and existing node-preview diagnostics. A compact badge MAY abbreviate the result, but it MUST expose the same explanation through an accessible label, tooltip, or detail surface. The presentation SHALL reuse the existing responsive renderer and shared overlay primitives rather than introduce a separate mobile workflow.

#### Scenario: Operator inspects an inconclusive result
- **WHEN** an operator opens a node diagnostic containing an `inconclusive` provider result
- **THEN** the view identifies the provider as inconclusive, displays its contract/evidence version and sanitized signal summary, and does not display an unlock-success badge

#### Scenario: Narrow viewport remains actionable
- **WHEN** the NodeLedger and node detail surface render at 375px, 768px, 1024px, and 1440px widths
- **THEN** the provider explanation remains reachable without document-level horizontal overflow, all interactive controls retain at least 44px targets on narrow viewports, and overlay dimensions use the shared modal/drawer scale

### Requirement: Verified-only compatibility for capability filters
Existing media and AI filters in NodeLedger, subscriptions, and node groups SHALL continue to select a platform only when its normalized result represents a verified supported capability. `partial`, `restricted`, `ip_blocked`, `challenged`, `rate_limited`, `timeout`, `transport_error`, and `inconclusive` results MUST NOT satisfy a full-unlock filter. Legacy persisted result values representing a known full unlock SHALL retain their prior filtering result until replaced by a newer observation.

#### Scenario: Partial Netflix result is filtered out of full-unlock requirement
- **WHEN** a subscription or node group requires Netflix and the latest result is `status: partial` with `verdict: originals_only`
- **THEN** that node is excluded from the required Netflix-unlock result set

#### Scenario: Historical full result remains usable
- **WHEN** a historical probe record contains the existing recognized Netflix full-unlock representation and has no additive evidence fields
- **THEN** existing UI, export, and capability filters continue to interpret it as full unlock without migration or destructive rewrite of the record

### Requirement: Additive persistence and API compatibility
The system SHALL persist additive evidence within the existing media result payload without changing node identity keys or deleting historical probe rows. Existing consumers of platform keys and established status values SHALL continue to receive compatible values; consumers that understand the new fields SHALL receive the explanation fields with the same probe result. No endpoint shall require an existing caller to supply a new probe evidence field.

#### Scenario: Existing client reads a historical and a new result
- **WHEN** an existing client reads probe results containing both an older media payload and an evidence-grade payload
- **THEN** it can still render each known platform key and its pre-existing availability semantics, while an updated client can render the additive evidence without an API version switch
