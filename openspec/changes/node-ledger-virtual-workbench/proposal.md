## Why

The in-progress NodeLedger rewrite currently imports APIs that do not exist, so production builds fail. Its simplified UI also omits established NodeLedger operations and can accidentally limit batch probing to the DOM-visible virtual window instead of the complete filtered result.

## What Changes

- Restore the NodeLedger’s contract with the existing ledger, probe, and proxy-chain APIs; no backend API, historical data, or persistent schema changes are required.
- Split NodeLedger state, filtering, and operations from presentation while keeping all existing node-management functions: full-protocol probes, cancellation of future probe chunks, selected-or-filtered targeting, probe-data clearing, detail inspection, and node-level dialer-proxy configuration and clearing.
- Render the dense table through the installed `@tanstack/vue-virtual` component, preserving selection and full filtered-result operations independently of virtual row visibility.
- Complete the Workbench components so metrics, multi-dimensional filters, media and AI-unlock filters, sorting, and detail actions preserve the legacy NodeLedger’s observable behavior.
- Add behavior-focused frontend tests and require typecheck, frontend tests, production build, backend tests, and container health verification before deployment.

## Capabilities

### New Capabilities

- `node-ledger-workbench`: A production NodeLedger interface that manages, filters, probes, inspects, and configures the complete node inventory without losing operations to virtualized rendering.

### Modified Capabilities

- None.

## Impact

- Affected frontend files include `frontend/src/views/NodeLedger.vue`, the ledger and virtual-table components, API imports, and focused frontend tests.
- Existing backend endpoints remain the source of truth: `/api/proxy-chains/meta/node-ledger`, `/api/probe/*`, `/api/proxy-chains`, and `/api/node-groups`.
- The installed `@tanstack/vue-virtual` dependency is reused; no new runtime dependency, database migration, or API-contract change is introduced.
