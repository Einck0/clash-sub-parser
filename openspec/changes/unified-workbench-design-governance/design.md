## Context

See `proposal.md` and the `workbench-design-governance` delta specification. CSP currently has a semantic Tailwind theme entry and an accessible `BaseDrawer`, but `frontend/src/style.css` contains 92 legacy-variable references plus global `.modal/.modal-backdrop` behavior. `QuickExportModal` contains a separate Tailwind dialog implementation; Rule Import, Rule Presets and Node Group editing still use that legacy global modal; `SubscriptionForm` has a further manual-node modal. The present test suite primarily inspects source text and does not measure a rendered browser.

### Evidence: Graphify code graph (2026-09-04)

Before freezing this design, `graphify extract /home/service/clash-sub-parser --code-only` produced a code-only AST graph of 2,840 nodes, 6,139 edges and 160 communities. `graphify god-nodes` identifies the high-coupling backend/core pivots as `getApiErrorMessage()` (69 edges), `Subscription` (47), `Base` (46), `LogicalConfigurationBundle` (45), `Node` (40), `CanonicalGraphResolver` (38), `NodeGroup` (32), `fetch_subscription_nodes()` (28), `CompilationResult` (25) and `probe_single_node()` (24). They are outside this UI-governance change boundary. The direct overlay traces are narrow and explicit: `QuickExportModal.vue <--imports_from-- App.vue --contains--> showQuickExport`; `RuleCategoryDetail.vue --imports_from--> RuleImportModal.vue`; and Node Group state is reachable through the API/component edge to `components/NodeGroupModal.vue`. Therefore this change may refactor shared UI ownership and named overlay consumers, but MUST NOT alter API, subscription, compiler or probe nodes.

Eindash is a separate React 18/Vite repository with an active CPA-console OpenSpec change. Its current design contains a second modal implementation and visual decisions that conflict with the new shared control-plane constraints. This task freezes CSP implementation and the Eindash design contract; it does not combine repositories, modify CPA business behavior or deploy either application.

## Goals / Non-Goals

**Goals:**

- Establish a single token source and one a11y-safe overlay ownership model per application.
- Give CSP all required legacy overlay workflows the same modal or drawer lifecycle without changing their domain behavior.
- Make viewport geometry and modal-scale drift objectively reproducible in a browser.
- Propagate the same externally observable design contract into Eindash before CPA UI implementation begins.

**Non-Goals:**

- Do not rewrite each CSP view into Tailwind, redesign data density, replace `BaseDrawer`'s already proven focus helper, add a component-library dependency, or modify backend behavior.
- Do not delete non-SCRIPT functionality, stored data, supported export targets, existing routes, or manual-node workflow semantics.
- Do not change Eindash production code, settings, secrets, dependencies, service state or deployment during the synchronization task.

## Decisions

### D1. Token ownership is centralized, not a bulk blind search-and-replace

`frontend/src/assets/theme.css` remains the only authoring location for token values. It will define CSS custom properties and Tailwind mappings for a compact semantic vocabulary, including `--space-*`, `--radius-*`, `--z-*`, `--motion-*`, and the existing color semantics. `style.css` becomes a compatibility layout stylesheet only for selectors which remain necessary after the overlay migration; it MUST consume semantic properties and cannot declare a root theme, legacy alias, backdrop or generic modal rule.

The executor must inventory each old token's semantic intent before replacement. For example, old primary foreground/background use the explicit `text-main`/`surface-base` vocabulary, while status and selection appearances use status/accent semantics. A textual substitution of every `surface` token to the same new surface is rejected because it would collapse layer hierarchy. The existing visual audit will gain an explicit legacy-token rule rather than treating its absence as a manual convention.

Alternative considered: retain legacy aliases as one-line bridges in `:root`. Rejected because this leaves a parallel API that new components can consume and makes the declared removal unverifiable.

### D2. `AppModal` is a thin a11y wrapper; `BaseDrawer` evolves into `AppDrawer`

A new `components/ui/AppModal.vue` shall use the existing `useModalA11y` composable rather than creating a second focus-trap implementation. Its public contract is deliberately small:

```text
v-model / modelValue: boolean
size: sm | md | lg
labelled title: required string
close event: emitted after model update
footer slot: optional
```

The primitive owns Teleport, a shared opaque backdrop, a fixed shared stacking tier, `role=dialog`, `aria-modal`, generated `aria-labelledby`, close trigger, focus lifecycle, scroll lock, content shell and size class mapping. Size maps to CSS variables owned by the theme: 448px, 640px and 960px with `min(100% - 32px, var(--modal-size))`; no caller receives a width prop or private width class. It has a fixed header/footer and an `overflow-y-auto` body so the panel itself does not accidentally become a nested uncontrolled scroll surface.

`BaseDrawer` will be renamed/exported as `AppDrawer` only if all imports can be migrated atomically; otherwise the existing public component name remains a documented compatibility alias whose implementation adopts the `AppDrawer` contract. This avoids a wide unrelated rename while still producing one implementation. It owns only two semantic widths: 320px left navigation and 480px right desktop details. Existing drag, close, focus and safe-area code remains, but all width/radius values move to tokens and any caller-local drawer sizing is eliminated.

Alternative considered: one primitive with a `kind=modal|drawer` switch. Rejected because drawer edge/gesture behavior and modal centering/body scroll differ materially; shared a11y infrastructure is sufficient commonality.

### D3. Overlay migration preserves component-local content, not layout responsibility

Each legacy overlay retains its current props, emitted events, API calls, parsing and form state. The migration only replaces its outer backdrop/content shell with the correct primitive and moves private `<style>` layout into semantic classes or a shared UI stylesheet.

| Consumer | Primitive | Scale / placement | Retained behavior |
| --- | --- | --- | --- |
| QuickExportModal | AppModal | `md` | merged/single subscription, five targets, URL, scheme, QR, copy |
| RuleImportModal | AppModal | `md` | YAML/text/link parsing, selection, target proxy/category, apply |
| RulePresetsModal | AppModal | `lg` | search, selection, proxy/category override, apply |
| NodeGroupModal | AppModal | `lg` | all group entries, regex preview/edit, capability filters and save |
| SubscriptionForm manual-node edit | AppModal | `md` | YAML edit, validation and apply/cancel |
| existing secondary inspectors | AppDrawer | documented existing placement | detail/preview functionality |

Nested regex preview inside Node Group editing is a real nested modal case. The executor shall either replace it with a modal state that explicitly manages stacking/focus return to the parent or make it an in-modal panel. It must not leave a second raw fixed overlay. A test must cover the selected solution.

### D4. Browser tests measure behavior, not test source spelling

A dedicated Playwright geometry suite will start a deterministic frontend harness or use the existing `E2E_BASE_URL` when provided. It must not mutate production data. API fixtures/interception are permitted solely to render normal non-secret UI states. At 375, 768, 1024 and 1440 CSS px, the suite will:

1. load the covered route and assert `document.documentElement.scrollWidth <= clientWidth`
2. open every named overlay from an accessible user-visible trigger
3. read `getBoundingClientRect()` for the dialog/panel and assert it lies inside `visualViewport` or window bounds with the declared gutters
4. assert `role=dialog`, `aria-modal`, non-empty accessible title and close trigger; keyboard Escape closes and focus returns
5. assert Modal max width against declared `sm|md|lg` values at desktop, and Drawer presentation/bounds against desktop or mobile contract
6. assert intended local table/body scrolling does not widen the document

If an existing complete E2E fixture cannot expose a consumer without destructive setup, the executor shall build a test-only route/harness under existing frontend test conventions, with no runtime route exposed in production builds. Static token audit complements but never substitutes for these DOM assertions.

### D5. Eindash consumes the contract through OpenSpec, not copied CSS

The executor assigned to Eindash will update only `refactor-eindash-cpa-quota-console` OpenSpec design/tasks to declare: its local token entry is sole visual source; modal sizes map to 448/640/960px; desktop secondary drawer is 480px; mobile right-side detail becomes the governed bottom sheet; real browser geometry checks use 375/768/1024/1440px. It must explicitly remove conflicting planned decorative gradients/legacy generic modal assumptions before CPA UI implementation, but cannot edit the current React/CSS source in this card.

This preserves repository autonomy. Both applications share behavioural primitives and test criteria, not a shared package or copied class names.

## Risks / Trade-offs

- [Legacy global selectors have hidden consumers] → inventory all `.modal`, `.modal-backdrop` and legacy-token references before deletion; a static test fails if any remains outside a named short-lived migration allowlist, and no permanent allowlist is permitted.
- [A modal consumer lacks a safe E2E setup] → use intercepted, non-secret fixture data or test-only harness, never click destructive production operations.
- [Nested modal focus/scroll leaks] → make it an explicit test scenario and reject raw `position: fixed` nested overlays.
- [Concurrent probe work already modifies CSS/theme files] → executor re-reads exact on-disk files immediately before each patch; scope is limited to UI governance paths and the reviewer checks diff ownership.
- [Eindash architecture drifts later] → treat its updated OpenSpec task list as a prerequisite for any CPA UI executor; final cross-project review fails when either design lacks the common clauses.

## Migration Plan

1. Capture current CSP and Eindash working trees, inventory token/overlay consumers, and add failing static/DOM tests before code migration.
2. Implement CSP tokens and shared primitives with test-first evidence, then migrate one consumer at a time while preserving its behaviour tests.
3. Remove legacy global modal/token rules only after every consumer is migrated and tests demonstrate no remaining reference.
4. Add the multi-viewport browser gate and run it with unit tests, typecheck, production build and strict visual audit.
5. Independently update Eindash's CPA OpenSpec contract and task prerequisites without touching its implementation.
6. The review card checks both artifacts plus CSP diff/test output. It may approve only if all gates pass; no deployment is authorized by this plan.

Rollback before deployment is a normal source rollback: restore the previous frontend artifact and leave backend/data untouched. There is no schema, configuration, migration, container rebuild or service restart in scope.
