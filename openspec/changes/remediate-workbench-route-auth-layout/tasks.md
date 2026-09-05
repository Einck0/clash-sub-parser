## 1. Baseline and regression harnesses

- [x] 1.1 Re-read the live CSP worktree, active OpenSpec changes, router/App/store/AuthGate/Settings/Generate/theme/style files, and existing tests immediately before editing; record concurrent ownership and verify no patch overwrites unrelated probe or design-governance work.
- [x] 1.2 Add failing deterministic tests for `/probe`, unmatched-route recovery, header loading-versus-ready-zero state, generated Token acknowledgement, cancelled auth disable, and preserved five export targets; verify each fails for the intended missing behavior.
- [x] 1.3 Extend the existing non-destructive browser geometry harness with 375×812, 768×1024, 1024×768, and 1440×900 fixture states for AuthGate, Generate, Settings, route recovery, and document overflow; verify initial failures identify current route/layout defects.

## 2. Route and summary-state resilience

- [x] 2.1 Register `/probe` as the compatible Node Ledger quality-control entry and add an accessible catch-all recovery view with a Node Ledger return action; verify direct `/probe`, `/nodes`, and an unknown URL each render their specified result.
- [x] 2.2 Introduce the single node-summary lifecycle state described in design.md and connect App shell/header plus Node Ledger loading to it; verify pending data never displays factual zero while a loaded empty inventory does.

## 3. Access and Token safety interaction

- [x] 3.1 Rebuild AuthGate around labelled visible input affordances, show/hide, conditional clear, clipboard paste failure handling, and visual-viewport-safe mobile placement; verify keyboard/mobile geometry and authentication-error retention without changing backend login APIs.
- [x] 3.2 Implement generated-Token modal copy/acknowledgement flow using the existing governed AppModal and preserve selectable fallback on copy failure; verify save is prevented before acknowledgement and existing manual Token entry remains saveable.
- [x] 3.3 Add the dangerous confirmation boundary for `auth_enabled: true -> false`, cancel restoration, and post-save session refresh behavior; verify cancellation emits no settings request and confirmed state retains existing API contract.

## 4. Dark workbench composition and viewport priority

- [x] 4.1 Reconcile affected legacy style selectors with the semantic dark theme without creating a second token owner or raw modal implementation; verify Generate and Settings have coherent surfaces/contrast and all existing frontend visual-governance tests remain green.
- [x] 4.2 Reorder Generate so selected target URL plus copy/import action is initially visible at 1440×900 while retaining all five targets, QR, short/full URLs, download, and preview behavior; verify browser measurement and export target regression tests.
- [x] 4.3 Refactor only Settings control grouping needed for scanability and remove nested checkbox-card presentation without deleting any auth, fetch-proxy, probe, media, speed, download, backup, reset, or usage capability; verify desktop/mobile labels and control dependencies.

## 5. Integration verification and handoff

- [x] 5.1 Run the relevant frontend unit tests, `npm run typecheck`, `npm run build`, `npm run audit:visual:strict`, and real browser geometry suite; retain actual outputs and verify no destructive API call or production-data mutation occurs.
- [x] 5.2 Run `graphify update .`, inspect the changed call boundaries, and produce a concise handoff listing changed files, actual verification commands/results, collision notes, preserved non-SCRIPT capabilities, and residual risks for independent review.
