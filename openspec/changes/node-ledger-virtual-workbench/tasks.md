## 1. Domain Tests and Harness Freeze

- [x] 1.1 Add failing unit test for `frontend/src/views/nodeLedgerDomain.ts` covering node-to-probe matching, multi-field search, multi-media AND filtering, min-speed filtering, metric shortcuts, and sorting; verify it fails with `npm test tests/node_ledger_domain.test.mjs`.
- [x] 1.2 Add failing test verifying batch probe target selection uses all filtered rows when no row is selected and uses selected rows regardless of virtual-table mounted positions; verify it fails with `npm test tests/node_ledger_domain.test.mjs`.
- [x] 1.3 Add failing test verifying probe batch chunking stops scheduling subsequent chunks when aborted while keeping accumulated chunk results; verify it fails with `npm test tests/node_ledger_domain.test.mjs`.
- [x] 1.4 Add failing test verifying node-level dialer replacement deletes matching node-level bindings before creating the new validated binding; verify it fails with `npm test tests/node_ledger_domain.test.mjs`.

## 2. Core Implementation

- [x] 2.1 Implement `frontend/src/views/nodeLedgerDomain.ts` and verify all tests in `tests/node_ledger_domain.test.mjs` pass.
- [x] 2.2 Refactor `frontend/src/views/NodeLedger.vue` to use real exports (`getNodeLedger`, `getProbeResults`, `probeNode`, `probeNodesFull`, `clearProbeResults`, `getProxyChains`, `getNodeGroups`, `createProxyChain`, `deleteProxyChain`), remove nonexistent imports (`getNodes`, `runProbeJob`, `cancelProbeJob`), wire `VirtualNodeTable`, `LedgerMetricsBar`, `LedgerSearchFilter`, and `LedgerDrawer`, and verify `npm test` passes.
- [x] 2.3 Upgrade `VirtualNodeTable.vue` with accessible role attributes, row selection slot support, and responsive viewport sizing; verify with `npm test tests/ui_primitives.test.mjs`.
- [x] 2.4 Update `LedgerMetricsBar.vue`, `LedgerSearchFilter.vue`, and `LedgerDrawer.vue` to support all filter, diagnostic, dialer proxy, and clear-chain interactions; verify with `npm test tests/ledger_components.test.mjs` and `npm test tests/ledger_drawer.test.mjs`.

## 3. Verification and Deployment

- [x] 3.1 Run frontend static checks and build: execute `npm run typecheck` and `npm run build` in `frontend/` and verify exit code 0.
- [x] 3.2 Run backend regression suite: execute `/data/py310/bin/pytest backend/tests/ -q` and verify all tests pass.
- [x] 3.3 Build production Docker image and update running container: execute `docker compose build app && docker compose up -d app` and verify container status becomes healthy.
- [x] 3.4 Verify service endpoints and public egress: verify `curl -f http://127.0.0.1:8000/api/health` and `curl -fsSI https://sub.einck.top/` return success status.
