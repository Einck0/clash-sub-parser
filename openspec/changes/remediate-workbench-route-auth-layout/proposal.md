## Why

CSP exposes a dead `/probe` navigation target while the Vue router has no matching route or catch-all fallback, producing an empty white workspace. Its authenticated entry and settings/export surfaces also violate the established dark-console, destructive-action, and 1440×900 geometry contracts, creating a lockout path when a generated Token is saved without a recoverable copy.

## What Changes

- Register the existing Node Ledger quality-control experience as the canonical `/probe` route alias and add an accessible, non-empty catch-all route for unknown client-side paths.
- Replace AuthGate's technical implementation copy with task-oriented access guidance; add visible field affordance, password visibility, paste, clear, keyboard-safe mobile layout, and explicit loading/error states.
- Make generated Token replacement a guarded state transition: reveal a generated value in a controlled dialog, copy it, require acknowledgement before it becomes eligible for save, and require a danger confirmation before disabling Token authentication.
- Reprioritize Generate and Settings layout at 1440×900 so primary export URLs/actions and active settings content remain visible without decorative KPI displacement; remove light/dark split surfaces and redundant nested form borders.
- Bind Workbench header counts to the existing Node Ledger/probe data lifecycle rather than rendering an indistinguishable `0` before the data state is known.

## Capabilities

### New Capabilities

- `workbench-route-resilience`: Client-route registration, 404 recovery, and non-empty loading/error/empty-state behavior for the CSP workbench.
- `workbench-auth-and-security-guardrails`: Usable access-token entry and safe Token/authentication state transitions that prevent operator lockout.
- `workbench-viewport-priority`: Responsive dark-console layout requirements for primary export and settings actions.

### Modified Capabilities

- None.

## Impact

- Frontend: router, App shell/shared state, AuthGate, Settings, Generate, theme/style selectors, and browser/unit regression tests.
- Backend APIs, SQLite data, probe pipeline, supported exports (Clash, Mihomo, Stash, Shadowrocket, Sing-box), and existing Node Ledger domain behavior remain unchanged.
- This change does not deploy CSP or modify production configuration.
