## 1. Freeze baseline and add regression harnesses

- [x] 1.0 Run `graphify extract <project> --code-only`, `graphify god-nodes` and the named overlay `graphify path` queries before changing code; record the AST node/edge counts, core hubs and permitted UI call boundaries in the design evidence, then verify no implementation task changes a graph-identified backend/core pivot.
- [x] 1.1 Re-read the current CSP theme, global stylesheet, every `.modal`/`.modal-backdrop` consumer, `BaseDrawer`, frontend test scripts and `git status`; record the exact legacy-token and overlay-consumer inventory, preserve concurrent probe changes, and verify no implementation patch overwrites newer disk edits.
- [x] 1.2 Add failing unit/static tests that reject remaining legacy token aliases, second root themes and raw legacy modal/backdrop consumers; verify each test first fails against the current implementation for the intended absence of governance.
- [x] 1.3 Add failing non-destructive Playwright geometry tests/harnesses for 375/768/1024/1440px that measure document overflow, overlay bounds, modal scale, drawer presentation and keyboard lifecycle; verify the new test fails before the primitive migration and can run without production-data mutation.

## 2. Implement CSP semantic primitive ownership

- [x] 2.1 Extend the CSP theme entry with semantic spacing, radius, layer and motion tokens plus modal scale values 448/640/960px and drawer widths 320/480px; remove root legacy aliases from `style.css` and verify the legacy-token test passes without flattening semantic surface hierarchy.
- [x] 2.2 Implement `AppModal` by reusing the proven modal accessibility lifecycle and allow only `sm|md|lg`; verify desktop geometry, scroll ownership, dialog semantics, Escape, focus trap, scroll lock and focus restoration with a failing-then-passing component/browser test.
- [x] 2.3 Evolve/export the existing `BaseDrawer` as the documented AppDrawer contract without a duplicate focus implementation; move desktop/mobile dimensions to semantic tokens and verify left navigation remains 320px edge-drawer while right panels are 480px desktop drawers and `<640px` bottom sheets with safe area and 44px controls.

## 3. Migrate CSP overlay consumers without changing domain behaviour

- [x] 3.1 Migrate QuickExportModal to `AppModal size="md"`; verify merged/single subscription selection, five targets, URL, Scheme, QR and copy feedback remain functional and no caller supplies private modal geometry.
- [x] 3.2 Migrate RuleImportModal and RulePresetsModal to AppModal `md`/`lg`, remove their private style blocks and legacy wrapper classes, and verify parsing/search/selection/apply semantics remain available.
- [x] 3.3 Migrate NodeGroupModal and the manual-node edit overlay to AppModal `lg`/`md`; replace any nested raw fixed overlay with an explicitly tested governed solution and verify group editing, regex preview/edit, capability controls and manual YAML save/cancel remain functional.
- [x] 3.4 Remove the global `.modal/.modal-backdrop` and consumer-local overlay geometry only after all named consumers migrate; verify source inventory reports zero legacy references and existing non-SCRIPT workflow tests remain green.

## 4. Establish real-rendered release gates

- [x] 4.1 Complete the Playwright geometry suite with fixture/interception or test-only harness support where required; verify every named modal/drawer consumer and core table route at 375/768/1024/1440px, including no document overflow and local scrolling containment.
- [x] 4.2 Run CSP frontend unit tests, `npm run typecheck`, `npm run build`, `npm run audit:visual:strict`, and the Playwright geometry suite; retain actual command output and verify no test was skipped or converted to a static spelling-only substitute.

## 5. Synchronize Eindash’s future CPA UI contract

- [x] 5.1 Re-read Eindash’s current CPA OpenSpec artifacts and active working tree, then update only its planning documents to adopt this token, Modal scale, Drawer scale, Bottom Sheet and 375/768/1024/1440px browser-gate contract; verify `openspec validate refactor-eindash-cpa-quota-console --strict` passes without editing Eindash application code or runtime configuration.

## 6. Independent final gate

- [ ] 6.1 Have reviewcommon independently inspect the full CSP and Eindash planning diffs, run declared CSP browser/type/build/visual checks, and adversarially verify 375/768/1024/1440px DOM geometry, keyboard dialog lifecycle, legacy-token removal, non-SCRIPT functional preservation, absolute-path rules and task-skill compliance; approve only on explicit `verdict: APPROVED`.
