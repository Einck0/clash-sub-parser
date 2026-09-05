## 1. Freeze the visual baseline and red test gates

- [x] 1.1 Re-read the current frontend worktree, `theme.css`, `style.css`, shared Button/Modal/Drawer, Node Ledger filter/row/drawer components and existing browser/visual tests; record the exact touched-selector inventory and confirm no backend/domain/API file is in scope before each patch.
- [x] 1.2 Add failing targeted audit tests for the void/panel/card token ladder, a single governed overlay-only blur allowance, rejection of ungoverned blur/legacy Slate visual values/width transitions, and semantic token ownership; verify each new test fails for its intended absent behavior before production styling changes.
- [x] 1.3 Extend the non-destructive Playwright/browser harness with failing Node Ledger assertions at 375/768/1024/1440px for no document overflow, filter/region/media controls, table/card/mobile presentation, overlay semantics/geometry and reduced-motion lifecycle; verify fixtures do not alter production data.

## 2. Implement the shared Linear/Raycast visual foundation

- [x] 2.1 Replace the default dark semantic theme values with the frozen void `#08090a`, panel `#0d0f12`, card `#121417`, raised/inset, text, hairline, inset-highlight, focus and targeted motion tokens; verify the token/audit tests pass and no parallel root/legacy visual vocabulary remains in modified paths.
- [x] 2.2 Update shared Button, AppModal and BaseDrawer/AppDrawer presentation to consume the new token/elevation/motion contract while preserving every public prop, emit, modal/drawer scale and a11y lifecycle; verify dialog/drawer unit/browser checks cover keyboard, Escape, focus restore, overlay bounds and overlay-only blur fallback.
- [x] 2.3 Implement the reduced-motion path for shared interactions and governed overlays; verify a reduced-motion browser assertion can open/close each primitive without persistent opacity, transform or blur animation.

## 3. Rebuild Node Ledger visual density without business regression

- [x] 3.1 Recompose `LedgerSearchFilter` into the defined primary, evidence and operational control rails using the shared visual vocabulary; preserve its exact `v-model`, props and emits, all source/protocol/status/chain/sort/media/country/speed controls, selection/batch actions and view switcher, then verify existing filter/domain tests and responsive browser assertions pass.
- [x] 3.2 Refine `NodeLedger`, metrics, virtual table/card/mobile list and their badges/status affordances with tokenized scanning hierarchy, non-layout-shifting hover/focus/selected states and preserved 36px desktop row density; verify table/card/mobile inspection, filters, selection and controlled probe triggers retain existing UI behavior with fixture data.
- [x] 3.3 Refine `LedgerDrawer` internal cards/evidence presentation through the shared drawer contract; verify outbound identity, country, latency, streaming/AI evidence, chain tabs and existing permitted actions remain accessible from desktop and mobile presentations.
- [x] 3.4 Remove only the superseded compatibility selectors or component-local visual declarations proven unused by the new source audit; verify every named Modal/Drawer consumer and all non-SCRIPT export, rule, manual-node and Node Group workflows retain their existing primitive/API behavior.

## 4. Prove and independently gate the release candidate

- [x] 4.1 Run `npm test`, `npm run typecheck`, `npm run audit:visual:strict`, `npm run build` and `npm run test:e2e` from `frontend`; retain real output and fix failures without weakening tests, skipping browser coverage or broadening scope.
- [x] 4.2 Perform an exploratory browser pass of the rendered Node Ledger and at least one governed modal plus drawer in all declared responsive classes; record console errors and verify no horizontal overflow, inaccessible focus path, unreadable semantic status or live-data mutation occurs.
- [ ] 4.3 Have `reviewcommon` independently inspect the exact diff and frozen OpenSpec, re-run declared checks, adversarially verify the non-SCRIPT preservation, absolute-path rule, token/blur boundaries and multi-viewport DOM geometry; accept only an explicit `verdict: APPROVED`.
