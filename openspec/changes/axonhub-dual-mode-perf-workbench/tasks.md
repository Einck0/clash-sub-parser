## 1. Lock API projection and red tests

- [x] 1.1 Add targeted backend fixtures and failing tests for the `GET /api/probe/results` envelope, exact allowed summary keys, boolean media matrix, no name aliases/evidence/diagnostics/identity/credential fields, deterministic cursor continuation, and UTF-8 encoded body at or below 51,200 bytes; verify the focused pytest module first fails before implementation.
- [x] 1.2 Add failing route tests for `GET /api/probe/results/detail?node_key=...`, including exact URL-encoded lookup, blank input HTTP 422, unknown key HTTP 404, historical-media detail compatibility, and recursive credential/headers/cookies/tokens/raw-body redaction; verify them with the backend test command before implementation.

## 2. Implement bounded probe-result reads

- [x] 2.1 Implement pure persisted-record summary/detail serializers and byte-accounted cursor pagination without changing probe execution, cache, persistence writers, node keys, or database schema; verify the new serializer/unit tests pass and a generated large-evidence fixture still returns a summary body <=51,200 bytes.
- [x] 2.2 Change `GET /api/probe/results` to the frozen summary envelope and add the exact-key detail route under the existing router/auth policy; verify all focused probe persistence/route tests pass and no full-evidence collection or name-key duplicate response remains.
- [x] 2.3 Re-run the backend probe regression suite, including existing additive persistence, secret-redaction, routes, orchestration, execution, and capability predicate tests; verify no test requires outbound traffic or modifies a live database.

## 3. Make Ledger detail loading progressive and safe

- [x] 3.1 Update the frontend API layer and summary normalization to consume the paged envelope, merge pages by `node_key`, preserve legacy full-unlock filtering semantics from boolean media summaries, and expose no old full-collection compatibility path; verify focused Node Ledger domain tests cover page merging and media filter correctness.
- [x] 3.2 Update `NodeLedger.vue` and the app summary lifecycle to render page one first, sequentially load later pages with abort-on-unmount/navigation behavior, and visibly distinguish incomplete from complete ledger data; verify fixture-backed tests show initial render does not wait for later pages and virtual-table selection remains independent of mounted rows.
- [x] 3.3 Extend `LedgerDrawer.vue` and its parent contract with `idle`, `loading`, `ready`, and `unavailable` evidence states, exact-key late-response guards, and GET-only retry; verify unit/browser tests prove opening a drawer triggers one exact detail request, stale responses cannot overwrite a new selection, and retry does not call probe, clear, or chain mutation endpoints.

## 4. Apply frozen AxonHub visual and control contract

- [x] 4.1 Replace only the current semantic token ownership in `theme.css` with the frozen pearl-white/cool-gray light and `#0F172A`–`#1E293B` dark values, Emerald/Teal accent/status roles, and restrained elevation; verify source/static audit and computed-style browser assertions prove both themes render expected canvas/surface/text values and dark canvas is not `#000000` or `#08090A`.
- [x] 4.2 Update the shared `Button` presentation while preserving its public props and variants, then migrate allocated Ledger controls to Primary/Secondary/Ghost/Danger semantics with visible focus, loading, disabled states, and 44px narrow/40px wide effective targets; verify component tests and DOM geometry assertions pass.
- [x] 4.3 Recompose Ledger batch actions into one labelled Action Bar and table/grid choices into a labelled accessible segmented control; add the existing drawer tabs’ required tab semantics and keyboard path without changing props/emits, filters, selection, probe calls, virtual scrolling, or mobile list behavior; verify 375/768/1024/1440 viewport fixture tests cover pointer/keyboard activation and no horizontal overflow.
- [x] 4.4 Inventory and remove only superseded CSS declarations in allocated shared/Ledger selectors after consumers use semantic roles; verify `npm run audit:visual:strict` passes without weakening its overlay, blur, raw-color, or motion guardrails.

## 5. Integration evidence and independent gates

- [x] 5.1 Run `pytest -q backend/tests/test_probe_persistence_and_api.py backend/tests/test_probe_routes.py backend/tests/test_probe_orchestration_contract.py backend/tests/test_probe_orchestration_execution.py backend/tests/test_probe_capability_predicate.py` from `backend` and retain actual passing output.
- [x] 5.2 Run `npm test`, `npm run typecheck`, `npm run audit:visual:strict`, `npm run build`, and `npm run test:e2e` from `frontend`; verify all pass without skipped browser scenarios, live-node access, production mutation, or test weakening.
- [ ] 5.3 Capture fixture-backed Chromium evidence at 375, 768, 1024, and 1440 widths in both themes for initial summary render, progressive completion, detail loading/unavailable/retry, Action Bar, segmented views, drawer tabs, focus geometry, and no overflow; retain console output and screenshots for the Critic gate.
- [ ] 5.4 Request independent `reviewcommon` review against this change and retain an explicit `verdict: APPROVED` after it inspects the exact diff, re-runs declared checks, and verifies the API/privacy/token boundaries; then request the pre-created Critic child to perform its dual-theme multi-viewport Chromium visual gate and retain `PASSED` before release consideration.
