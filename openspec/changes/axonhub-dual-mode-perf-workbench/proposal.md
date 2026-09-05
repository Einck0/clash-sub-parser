## Why

CSP Node Ledger currently loads every persisted probe record through `GET /api/probe/results`, including per-provider diagnostic evidence and egress-identity evidence. The service serializes each record under both `node_key` and name, so a 4 MB first-screen payload can block the Ledger even though its table and filters need only a small subset of each record. At the same time, the existing dark-first token layer uses near-black canvas values and the light mode remains a functional override rather than a coherent AxonHub-quality surface system.

## What Changes

- **BREAKING (read API shape):** make `GET /api/probe/results` return an envelope whose `results` map is keyed only by `node_key`; every value is a fixed lightweight summary with `status`, `latency_ms`, `speed_mbps`, `country`, `ip`, and a boolean media-unlock matrix. It MUST omit node identity evidence, per-provider diagnostics, AS/organization, server data, errors, timestamps, and all raw/sensitive artefacts.
- Add `GET /api/probe/results/detail?node_key=<exact-node-key>` as the sole on-demand detail endpoint. It returns the existing safe detailed probe representation for exactly one persisted node, including identity evidence and sanitized per-provider diagnostics, never credentials, request headers, cookies, tokens, raw response bodies, or proxy secrets.
- Update Node Ledger and its app summary path to consume the summary envelope on initial load, then merge one fetched detail record into the selected drawer state only after the drawer is opened. The drawer has a non-destructive loading, unavailable, and retry path and never re-fetches the collection to obtain evidence.
- Freeze one AxonHub dual-mode semantic token contract: pearl-white/cool-gray light surfaces with soft restrained elevation, and Slate `#0F172A`–`#1E293B` dark surfaces with Emerald/Teal semantic accents rather than void-black and indigo ownership. Apply it through the existing theme and shared UI primitive contracts.
- Freeze Primary, Secondary, Ghost, and Danger control semantics. Consolidate Node Ledger batch actions into an Action Bar and view switches/choice pairs into accessible segmented controls while retaining current actions, component props/emits, virtual scrolling, data filters, and route behavior.
- Add deterministic API payload/security checks plus fixture-backed multi-viewport browser gates for both themes, drawer detail loading, control geometry, keyboard/focus behavior, and no document-level horizontal overflow.

## Capabilities

### New Capabilities

- `probe-result-summary-and-detail`: bounded collection summaries and explicit single-node evidence retrieval for persisted probe results.
- `axonhub-dual-mode-workbench`: semantic light/dark surface, button, action-bar, and segmented-control behavior for the existing control plane.

### Modified Capabilities

- None. The existing `node-capability-probe` persistence requirements remain intact; the collection-read and evidence-on-demand contracts are new capabilities layered over the same stored records.

## Impact

- Backend scope: `backend/app/routers/probe.py`, `backend/app/services/probe/service.py`, probe schemas if response models are introduced, and targeted route/persistence tests. Existing database schema and stored probe records remain unchanged.
- Frontend scope: `frontend/src/api/index.ts`, `frontend/src/views/NodeLedger.vue`, `frontend/src/stores/app.ts`, `LedgerDrawer.vue`, `LedgerSearchFilter.vue`, shared `Button.vue`, and semantic CSS/test files. No framework, dependency, route, deployment, or configuration change is authorized.
- This is a compatibility change for direct callers of the old collection map. CSP’s owned frontend changes in the same release; external consumers must migrate to the documented envelope. No automatic data migration is required because this is an additive read projection over existing records.
