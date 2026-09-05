## Context

See `proposal.md` and its two delta specifications. CSP is a Vue 3/Vite control plane whose visual ownership already has an accessible `AppModal` and `BaseDrawer` compatibility alias, a token entry, a large legacy compatibility stylesheet, a Node Ledger composed from `NodeLedger.vue`, `LedgerSearchFilter.vue`, desktop/mobile row components and `LedgerDrawer.vue`. The current default dark palette remains Slate-blue (`#090D16`, `#0F172A`, `#1E293B`), while the token file duplicates theme values between `@theme` and selector roots. The Node Ledger already contains the required domain capabilities: country selection, media/AI capability matrix, latency/speed thresholds, multiselect/batch actions, table/card/mobile views, node inspection and controlled probing.

### Evidence and boundary

The previously validated Graphify evidence for the overlapping design-governance change (`unified-workbench-design-governance`) recorded a code-only AST graph of 2,840 nodes and 6,139 edges. Its high-coupling pivots (`Subscription`, `Node`, `NodeGroup`, `probe_single_node()`, `fetch_subscription_nodes()`, compiler/API models) are explicitly outside this change. The direct UI chain is narrow: `NodeLedger.vue → LedgerSearchFilter.vue/LedgerDrawer.vue → BaseDrawer.vue`, and overlay consumers import `AppModal.vue`. `graphify` is not installed in the current worker image, so this design reuses that recorded extraction rather than inventing a new graph result; the missing runtime has been recorded on the Kanban task.

This implementation must be a visual-system migration, not a domain rewrite. It must not alter API calls, probe request construction, response normalization, filters' data meaning, selection/batch behavior, export targets, backend/data storage, credentials, routes, dependencies or deployment.

## Goals / Non-Goals

**Goals:**

- Make the existing Workbench feel like a precision dark control plane through a small, auditable semantic token system and consistent elevation.
- Give the filter/ledger/inspector surfaces a dense, calm hierarchy that materially improves scanning without concealing data or controls.
- Apply intentional and accessible micro-interactions to shared primitives and high-frequency ledger controls.
- Extend real-browser and static release gates so future visual drift cannot silently reintroduce flat colors, ungoverned blur, or geometry/accessibility regressions.

**Non-Goals:**

- Do not clone Linear or Raycast branding, use their logos, add gradients, replace the existing application architecture, introduce a component library, or switch frameworks.
- Do not change the existing modal scale (`sm|md|lg`: 448/640/960px), drawer scale (320px left; 480px right), breakpoints, data density contract, backend operations or non-SCRIPT functionality.
- Do not touch application configuration, running services, deployment manifests, secrets, stored data or any path outside CSP frontend plus this change's OpenSpec/testing scope.

## Decisions

### D1. Establish one dark elevation ladder using semantic tokens

The frontend theme entry is the sole value-authoring point. The default dark ladder shall be:

| Semantic role | Value / treatment | Use |
| --- | --- | --- |
| `canvas` | `#08090a` | application void and page background |
| `surface-panel` | `#0d0f12` | top bar, persistent navigation, major control panels |
| `surface-card` | `#121417` | cards, tables, filter shell, dialogs/drawers |
| `surface-raised` | `#181b20` | hover/selected neutral surfaces, popover subregions |
| `surface-inset` | `#090b0d` | search/input recesses and data-cell wells |
| hairline | `rgba(255,255,255,0.07)` | card/row/control containment |
| highlight | `rgba(255,255,255,0.055)` inset top edge | Raycast-like physical depth, never a broad glow |
| primary text | `#f1f3f5` | headings/data of primary importance |
| muted text | `#8b919a` / `#626870` | metadata and disabled content |
| accent | cool indigo only | selected, primary action and keyboard focus |

Status values remain semantic: success/verified green, warning/partial amber, failure/destructive red, informational blue/cyan only where meaning requires it. Token names must express role, not their old source color. The executor will map current Tailwind semantic utilities to the new values and remove obsolete aliases only when all current consumers are converted.

Alternative considered: blanket replacement of all existing dark declarations with a new `body` background. Rejected because style.css includes many feature-specific rules and would retain a parallel visual API / flatten surfaces.

### D2. Preserve existing UI primitive contracts; evolve presentation centrally

`AppModal.vue` and `BaseDrawer.vue` remain the only overlay owners. Their public props, focus lifecycle, Teleport, modal/drawer scales and mobile bottom-sheet behavior remain unchanged. The executor centralizes their visual backdrop, shell, hairline, inset-highlight and open/close transition appearance in semantic primitives/tokens. A subtle `backdrop-filter: blur(4px)` is permitted only on the shared fixed overlay backdrop and must have an opaque-enough fallback background; callers and page chrome must not add blur.

This follows the existing governance primitive instead of introducing a new visual-dialog abstraction. It preserves the previous a11y evidence and means all Quick Export, rules, node groups, manual-node edit and ledger inspector surfaces inherit the same material quality.

Alternative considered: adding blur/box shadows individually to every modal. Rejected because it would re-create the component-local ownership the baseline change removed.

### D3. Turn the Ledger filter into a compact control plane without changing state or controls

`LedgerSearchFilter.vue` retains its model/event API and every current filter/action. It is restructured only as needed for presentation into: a primary control rail (search / source / protocol / status / chain / sort), an evidence rail (media/AI chips and region chips), and an operational rail (probe options, threshold, selection/batch actions, view mode). On desktop the container appears as one coherent rounded control surface with internal separators rather than separate generic cards. On narrow screens the rails wrap in source order, controls have at least 44px usable targets where actionable, and no horizontal scroll is introduced.

Region badges retain textual region names/counts; media/AI chips retain platform name/count/status; latency status retains numeric text. Soft tokenized indicator dots and border/foreground changes supplement those strings, not replace them.

Alternative considered: hiding advanced filters behind a new popover/drawer. Rejected because it reduces the visible domain signal and risks breaking high-frequency operators' scan flow.

### D4. Retain ledger density while improving row affordance and evidence scanning

The current virtual table's 36px desktop row target remains unchanged. The executor may add tokenized hover/selected/keyboard-visible styling, stronger table header/inset separation, status-dot treatment and refined badges; it cannot remove columns, virtual scrolling, card view, mobile list, click-to-inspect behavior or current action accessibility. The ledger inspector keeps its actual evidence, chain controls and tabs; only its shared drawer appearance and contained cards are changed.

Performance boundary: visual state changes use `opacity`, `color`, `background-color`, `border-color`, and compositor-safe `transform` only. No width/height animation, row remeasurement loop, per-row filter/blur, indiscriminate `transition: all`, or expensive shadow animation is permitted.

### D5. Define a tokenized interaction matrix with a reduced-motion path

- Secondary controls: targeted 150ms color/border/opacity response; no layout shift.
- Primary actions: controlled indigo surface/foreground and 1px/inner-highlight response; no generic saturated blue fill.
- Filter chips/badges: selected state changes accent containment and readable foreground; hover only increases semantic elevation/border clarity.
- Rows/cards: 150ms background/border/foreground transition, no scale effect.
- Modal/drawer: opacity backdrop plus bounded transform shell transition defined by existing motion tokens; shared backdrop blur only as governed above.
- `prefers-reduced-motion`: the same open/selected state lands immediately or in 0.01ms; blur transition and non-essential transforms are disabled.

Alternative considered: use dramatic scale, floating glow or animated gradients to advertise modernity. Rejected because they damage data-dense readability and violate the existing visual-audit constraints.

### D6. Extend verification rather than trusting screenshots or source spelling

The executor adds/updates targeted tests under existing frontend conventions. Static visual audit must distinguish legitimate centralized token values / shared overlay blur from ungoverned hard-coded declarations. Browser checks reuse the existing 375/768/1024/1440px harness and assert:

1. `scrollWidth <= clientWidth` on covered Node Ledger and overlay states.
2. Filter search, region and media chip interaction change the existing UI state/result summary without breaking focus.
3. table/card/mobile ledger view and selected/hover/focus affordances remain visible and readable.
4. AppModal/AppDrawer retain dialog semantics, title, Escape/focus behavior, declared geometry and overlay backdrop ownership.
5. `prefers-reduced-motion: reduce` does not block opening/closing and has no durable transition/animation.
6. no non-overlay `backdrop-filter`, component-local visual tokens, hard-coded legacy Slate visual values, width transition or production data mutation occurs.

Tests must intercept/fixture data rather than use a real token or alter live probe/subscription records. Visual inspection is supplemental; a screenshot alone is not a pass condition.

## Risks / Trade-offs

- [The `style.css` compatibility sheet contains many hard-coded visual declarations] → inventory each selector touched, move recurring values into semantic tokens, and preserve unrelated legacy behavior until an audit-backed migration is complete.
- [The existing global visual audit bans all backdrop blur] → change the rule only to accept a uniquely scoped shared primitive/backdrop selector, add positive and negative fixtures, and reject any other occurrence.
- [High-density rows become less legible after darkening] → verify contrast/semantic text in browser at all four viewports and keep numeric/status text alongside color treatments.
- [Concurrent work changes frontend CSS or theme assets] → executor re-reads each current on-disk file immediately before patching; changes remain restricted to allocated UI/test paths.
- [A visual refactor accidentally changes a probe/batch action] → preserve all component prop/emits and API call sites; test the non-destructive action paths with fixtures; reviewer compares event/API call surfaces and domain files against the frozen boundary.
- [Blur causes poor performance or incompatible rendering] → use only 4px backdrop blur on a single fixed overlay, keep opaque fallback, and remove blur for reduced motion; no per-row blur.

## Migration Plan

1. Capture `git status`, inspect the current theme, shared primitives, Ledger filter/row/drawer consumers and existing visual/browser tests; build the executor's exact selector/consumer inventory.
2. Add/adjust failing audit and browser tests for the new token/elevation ownership, restricted overlay blur, interaction states and reduced-motion behavior before visual edits.
3. Replace the default dark token ladder centrally, then migrate shared Button/Modal/Drawer presentation without changing their public contracts.
4. Refine the Node Ledger header, filter surface, metrics, virtual table/card/mobile rows and inspector using only semantic utilities/tokens; preserve all input/model/emit/API behavior.
5. Remove superseded conflicting compatibility declarations only after all affected consumers use the new vocabulary and the audit proves no ungoverned residual remains.
6. Run frontend tests, typecheck, strict visual audit, production build and Playwright browser gate. Do not deploy or restart any service.
7. If release validation fails, restore only the frontend visual diff for this change. No database/API/config migration is involved, so normal source rollback is sufficient.
