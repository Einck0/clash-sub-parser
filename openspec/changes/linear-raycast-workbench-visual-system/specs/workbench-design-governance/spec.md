## MODIFIED Requirements

### Requirement: Semantic design tokens are the sole visual source
The system SHALL expose one semantic token vocabulary for void canvas, panel surface, card surface, raised surface, inset surface, hairline borders, main and muted text, accent, status colors, focus, spacing, radii, elevation, overlay blur and motion. In the CSP default dark theme, void canvas SHALL resolve to `#08090a`, panel surface to `#0d0f12` and card surface to `#121417`; elevation SHALL be communicated through controlled luminance steps, restrained inset highlights and hairline containment rather than opaque Slate-blue fills or generic drop shadows. Domain styles SHALL consume that vocabulary and SHALL NOT retain legacy aliases such as `--brand`, `--ink`, `--surface`, `--bg-0`, `--bg-1` or a parallel global theme. A domain component SHALL NOT create a private visual token system or hard-code an overlay's color, radius, elevation, width or z-index. Backdrop blur SHALL be available only through the shared application overlay primitive and SHALL NOT be used as page chrome or a component-local effect.

#### Scenario: Legacy token regression is rejected
- **WHEN** a source audit encounters a legacy token reference, a second global token root, an opaque Slate-blue visual fallback, or component-local backdrop blur outside the approved theme/overlay entry
- **THEN** the audit SHALL fail and identify the source location before the change is accepted

#### Scenario: Existing feature styling is migrated
- **WHEN** a legacy CSP management view or modal is rendered after the migration
- **THEN** it SHALL retain its existing non-SCRIPT workflow while consuming the semantic token vocabulary and visible elevation hierarchy rather than the removed aliases
