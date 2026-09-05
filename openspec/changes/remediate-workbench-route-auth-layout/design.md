## Context

See proposal.md and the three new delta specifications. The browser audit and a fresh Graphify AST extraction established that `/probe` is absent from `frontend/src/router/index.ts`, while `backend/app/main.py` already mounts the probe API router and `NodeLedger.vue` owns the real node/probe loading lifecycle. `App.vue` renders header totals from an unpopulated `useAppStore().nodes` shape, making a pending data state indistinguishable from zero. Existing overlay primitives are governed by the active `unified-workbench-design-governance` change and SHALL be reused.

The on-disk `style.css` still owns a light `:root` palette and legacy global modal selectors, whereas workbench routes use semantic dark Tailwind tokens from `assets/theme.css`. `Generate.vue` places its current URL after an expansive controls section; `Settings.vue` exposes generated Tokens in the form without copy acknowledgement and sends all pending security flags directly during save.

## Goals / Non-Goals

**Goals:**
- Make route, header-data, access-token, and dangerous-security transitions explicit state machines with user-visible outcomes.
- Use existing workbench primitives and API contracts; preserve all non-SCRIPT functionality and persisted data.
- Establish browser-measured regression gates at 375×812, 768×1024, 1024×768, and 1440×900.

**Non-Goals:**
- Do not alter backend probe execution, supported protocol parsing, media/AI checks, speed-test semantics, exports, database schema, or authentication API contract.
- Do not add a component library, introduce a second modal implementation, deploy CSP, or edit reverse-proxy configuration.
- Do not rewrite unrelated legacy views solely for cosmetic consistency.

## Decisions

### D1. `/probe` is an alias to Node Ledger; unknown paths get a dedicated recovery view

`/nodes` and `/probe` SHALL resolve to the same lazily loaded Node Ledger component, preserving both existing URLs. A small `NotFound` view receives all unmatched paths and provides a single accessible return action. This is preferable to a redirect-only catch-all because the user must be told that a bookmarked/deep-linked path is invalid rather than silently losing route context.

### D2. Header summary has an explicit lifecycle

Extend the existing Pinia app store with a read-only `nodeSummary` state containing `status: idle|loading|ready|error`, `total`, and `probed`. A single summary refresh derives both counts from the same established Node Ledger/probe API responses. `App.vue` starts an initial refresh; `NodeLedger.vue` writes its already-loaded result into the same summary state. `WorkbenchHeader` renders a placeholder while idle/loading and factual zero only after ready.

Alternative: infer counts from `store.nodes?.length || 0`. Rejected because this confuses absent data with a loaded empty inventory and is the documented inconsistency root cause.

### D3. AuthGate uses controlled input affordances and visual-viewport-safe placement

AuthGate remains a focused authentication form, but uses one labelled input group with visible boundary/focus styles, an integrated show/hide control, clear control only when non-empty, and paste control that calls Clipboard API without making Clipboard availability a dependency. Its shell uses `min-height: 100dvh`, safe-area padding, and a scrollable content region; the form is aligned toward the visual top on narrow devices so a keyboard cannot trap the submit button below the layout. Technical cookie/query-token explanatory copy is removed from the surface; the user sees only the access purpose and recovery instruction.

Alternative: retain a centered fixed card. Rejected because keyboard occlusion is a geometry failure, not a copy issue.

### D4. Generated Token and authentication mode use guarded pending state

Settings distinguishes manual text from a generated replacement. A generated value opens existing `AppModal` with selectable content, Copy action, and acknowledgement control. Clipboard fallback uses a transient textarea only for browser compatibility; failure never marks acknowledgement. `save()` rejects a generated-but-unacknowledged replacement before issuing requests. Manual Token entry remains saveable to preserve established administration use.

On load, retain `persistedAuthEnabled`. Before persistence, a transition `true -> false` calls the existing `store.confirm({ danger: true })`; cancellation restores the UI to enabled and returns before either security or probe update request. After successful token save, existing `loginAuthToken(newToken)` remains required to keep the current session viable. The separate “show token” checkbox is replaced by an input-tail eye control.

Alternative: automatically copy and save the generated Token. Rejected because browser copy can fail and automatic persistence is exactly the irreversible lockout path under review.

### D5. Dark token ownership and above-fold priority are enforced at composition boundaries

`assets/theme.css` remains the semantic token owner. The executor will remove the competing light default palette and legacy modal ownership in `style.css` only after preserving selectors still consumed by legacy routes through semantic variables. Generate will expose the selected target's current export URL and copy/import action in a compact primary panel immediately below the route header, before optional metrics or configuration controls. The existing full multi-target selectors, QR, short/full URLs, all five target exports, and download/preview behavior remain intact.

Settings will use section headings, concise rows, and state descriptions instead of individually boxed checkbox cards. The node-probe settings remain intact and scroll naturally after the security controls; no content is hidden or deleted.

### D6. Verification must exercise the browser and state transitions

Add/extend deterministic frontend tests covering: `/probe`, unknown route recovery, header loading versus ready-zero, copy success/failure acknowledgement, confirmation cancellation, and all export target identities. Add to the existing real-browser geometry suite fixture interception for no-secret security settings and export responses. The suite measures document overflow and visible primary controls at the four required viewports; it never triggers a destructive save, probe, reset, or live external request.

## Risks / Trade-offs

- [Concurrent work changes shared theme/App files] → executor re-reads each file immediately before patching, performs minimal ownership-scoped changes, and reports collisions.
- [Clipboard API is absent in headless or insecure contexts] → treat it as a recoverable UI error with selectable Token; test both success and failure.
- [Summary refresh doubles Node Ledger traffic] → App refresh is bounded to initial shell load and Node Ledger reuses/writes its already fetched data; no polling loop is introduced.
- [Style legacy consumers still depend on old variables] → audit selectors and run all existing frontend tests/build before deleting compatibility declarations.

## Migration Plan

1. Capture current worktree state; add failing route, state, interaction, and geometry tests without changing live data.
2. Implement route recovery and node-summary lifecycle, then test direct/deep navigation.
3. Implement AuthGate and Settings guardrails using existing AppModal/ConfirmDialog; verify generated/manual Token paths and no request on cancelled disable.
4. Reorder Generate priority and migrate only affected legacy CSS to semantic dark tokens; run viewport gates and all established frontend verification.
5. No data migration, service restart, container rebuild, deployment, or runtime configuration mutation is authorized. Rollback is source rollback of this frontend-only change.
