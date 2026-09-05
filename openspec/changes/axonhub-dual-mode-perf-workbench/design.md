## Context

See `proposal.md` and its two new capability specifications. The current collection endpoint returns `dict[node_key|name, full_probe_record]` from `get_all_db_probe_results()`. Each record includes full media evidence and optional `identity_evidence`; every record is duplicated under its name and key (`backend/app/services/probe/service.py:243-276`). `NodeLedger.vue` and `stores/app.ts` load this endpoint during initial display and normalize it locally (`frontend/src/views/NodeLedger.vue:647-674`, `frontend/src/stores/app.ts:50-76`). The detail drawer currently receives its record synchronously as a prop and renders diagnostics directly (`frontend/src/components/ledger/LedgerDrawer.vue:44-158`).

The existing frontend already has Vue 3/Vite, a shared `Button` variant contract, virtualized desktop ledger, a mobile renderer, a theme switcher, and existing `data-theme` roots. `theme.css` defines both roots, but its dark canvas is near-black `#08090a` and its light palette has independent values (`frontend/src/assets/theme.css:84-217`). The currently defined Button variants are Primary, Secondary, Ghost, and Danger (`frontend/src/components/ui/Button.vue:52-75`), while several Ledger controls still use local raw buttons.

Graphify is not installed in this worker image (`command -v graphify` returned no executable). A checked-in graph exists and records `NodeLedger.vue` and `probe.py` as architecture hubs, but its report is from commit `d29bfe44`, whereas current HEAD is `bd8a2ae`; it is therefore contextual only. This design is grounded in the current direct call-chain evidence above, not a fabricated live graph.

## Goals / Non-Goals

**Goals:**

- Make first Ledger paint depend on one sub-50 KiB summary page, not a duplicated full-evidence collection.
- Preserve all stored probe evidence and make disclosure a precise, retryable, exact-key operation.
- Freeze a complete semantic visual/control contract that an executor can apply without deciding color ownership, response shape, or interaction semantics.
- Make performance, privacy, browser geometry, and theme behavior independently testable.

**Non-Goals:**

- Do not change probe runner behavior, node credentials, media classification rules, persistence schema, database contents, endpoint authentication policy, or deployment configuration.
- Do not create a compatibility endpoint that silently preserves the old full collection response, nor accept a node name as a detail identifier. That would retain the oversized ambiguity rather than removing it.
- Do not replace the Vue/Tailwind architecture, virtualizer, overlays, routes, or component library.
- Do not use AxonHub marks, copy its brand assets, add gradients, or add per-row blur/animation.

## Decisions

### D1. Cursor-paged byte-bounded summary collection is the new read contract

`GET /api/probe/results` becomes an explicit envelope:

```text
{
  "results": {
    "<node_key>": {
      "status": "ok|fail|timeout|skipped|unknown",
      "latency_ms": 83 | null,
      "speed_mbps": 42.1 | null,
      "country": "JP" | null,
      "ip": "203.0.113.7" | null,
      "media": { "chatgpt": true, "gemini": false, "youtube": true, "netflix": false, "disney": false, "meta_ai": false, "bilibili": true }
    }
  },
  "next_cursor": "<last emitted node_key>" | null,
  "has_more": true | false
}
```

The summary projection uses only exact fields above. `media` is a known-platform boolean matrix derived with the existing verified-full predicate semantics: unsupported, partial, inconclusive, or absent values are false. It never copies provider status, regions, evidence, errors, identity evidence, organization, ASN, checked timestamp, node configuration, or response diagnostic strings. Empty/nonexistent media values are false so consumers always get a stable matrix.

Records are ordered ascending by database `node_key`, retrieved with `node_key > cursor`, and capped by both requested/default `limit` and 51,200 serialized UTF-8 bytes. The executor must measure the candidate envelope using the actual JSON encoding before committing it. A too-large single summary cannot happen under the fixed projection; if it did, return HTTP 500 with a safe bounded error and test it rather than violate the byte ceiling. There is no name alias in `results`, eliminating the existing twice-serialized data and ambiguous external semantics.

The frontend loads page one and renders immediately. It then fetches later pages sequentially in the background, merges by `node_key`, and updates filters/metrics reactively. A route departure/unmount aborts the background chain. UI filters operate over summaries loaded so far, with visible loading/progress wording until `has_more` is false; no synchronous full collection request is permitted. This preserves virtual scrolling because rows remain the authoritative inventory and probes are normalized by `node_key`.

Alternative considered: return all projected summaries in one response. Rejected: 4,055 stable records can still exceed the 50 KiB hard requirement, and a silent truncation would make filtering incorrect. Alternative considered: retain aliases by node name for current callers. Rejected: name is not collision-safe and aliases recreate the payload duplication and old mixed contract.

### D2. Exact-key detail endpoint is the only evidence disclosure boundary

`GET /api/probe/results/detail?node_key=<exact URL-encoded key>` looks up only the unique `NodeProbeResult.node_key`. It returns one safe detailed record projected by the same existing sanitation rules. It may expose `identity_evidence`, `identity_confidence`, full media provider fields and their allowlisted `evidence` payloads, but it must not manufacture a `diagnostics` field if existing storage does not contain one. The existing provider-level evidence object is the canonical diagnostic payload.

The endpoint returns 422 for missing/blank input and 404 for an absent exact key. It has no `name`, prefix, wildcard, server, or `id` fallback. This avoids a drawer selecting one duplicate-named node and receiving another node’s evidence. It must use the same application router/auth middleware as the existing endpoint; it creates no public bypass.

`LedgerDrawer` receives separate `summaryProbe`, `detailProbe`, `detailStatus` (`idle | loading | ready | unavailable`), and `retryDetail` inputs/events or an equivalent frozen component contract. Upon selected node change, NodeLedger opens the drawer using summary values, starts detail fetch for its `node_key`, and ignores late responses unless their requested key still equals the selected key. The detail response replaces/merges only the selected record in the in-memory probe map; it is never persisted by the frontend. The drawer’s retry calls only the GET detail endpoint; it does not call `/probe/node`, batch probe, clear results, or chain mutation.

Alternative considered: return detail inline with `GET /results?include_evidence=true`. Rejected: the collection contract would become toggle-dependent and invite accidental first-screen oversized requests. Alternative considered: use node name as a more readable URL. Rejected for ambiguity and possible disclosure across duplicate names.

### D3. No data migration, import, or schema change

This work is a fresh read-projection and presentation change over the existing `node_probe_results` table. It is neither a clean-slate replacement nor an offline import. Historical persisted JSON remains in place and no migration is authorized. The summary serializer must gracefully derive booleans from legacy media forms; the detail serializer keeps current additive compatibility and redaction behavior. Deployment rollback is therefore source rollback only.

### D4. Freeze the AxonHub semantic-token contract and ownership

`frontend/src/assets/theme.css` is the one value-authoring location. `@theme` names map to semantic variables; `:root`/`[data-theme]` bind their values. `style.css` and component styles consume variables/utilities and must not create parallel raw palette ownership. The frozen roles are:

| Role | Light | Dark | Usage |
| --- | --- | --- | --- |
| `canvas` | `#F8FAFC` | `#0F172A` | application background |
| `surface-panel` | `#FFFFFF` | `#111C31` | header/sidebar/persistent framing |
| `surface-card` | `#FFFFFF` | `#16233A` | cards, filters, table/drawer panels |
| `surface-raised` | `#F1F5F9` | `#1E293B` | hover, active neutral surfaces |
| `surface-inset` | `#EAF0F7` | `#0B1324` | inputs/data wells |
| `border-subtle` | `rgba(15,23,42,.10)` | `rgba(148,163,184,.18)` | containment |
| `text-main` | `#0F172A` | `#E2E8F0` | primary data/headings |
| `text-muted` | `#475569` | `#94A3B8` | metadata |
| `accent` | `#0F766E` | `#2DD4BF` | primary, selected, focus |
| `status-success` | `#047857` | `#34D399` | verified/success |
| `status-info` | `#0F766E` | `#22D3EE` | informational |

Light elevation is limited to `0 1px 2px rgba(15,23,42,.04), 0 8px 24px rgba(15,23,42,.06)` and dark elevation to restrained slate opacity shadows. No global black body rule, generic button CSS, or hard-coded Indigo primary treatment may override these roles in newly touched paths. Existing legacy CSS outside the allocated migration should not be mass-rewritten; the executor performs a selector inventory and only removes declarations demonstrably superseded by semantic tokens.

Alternative considered: retain the previous indigo accent and only change background values. Rejected because status/primary ownership stays visually mixed and violates the requested Emerald/Teal direction. Alternative considered: use `#000` or `#08090a` for contrast. Rejected by the explicit Dark mode contract and operator comfort requirement.

### D5. Control hierarchy, Action Bar, and segmented-control semantics

`Button.vue` retains its public `variant`, `size`, `loading`, `disabled`, `icon`, and native type contract. Its variants are frozen as: Primary is the sole affirmative main action per context; Secondary is supporting/reversible; Ghost is contextual/low emphasis; Danger is destructive and only reaches mutation after existing confirmation. Button loading must preserve width/label context and set `aria-busy`; disabled remains non-actionable. Target geometry is >=44 CSS px under 640px and >=40 CSS px otherwise.

`LedgerSearchFilter.vue` owns the Action Bar: selected batch probe, selection clear, untested/failed probe, and clear persisted probe data. It receives a programmatic label such as `aria-label="节点质检操作"`. `LedgerDrawer.vue` uses a labelled `role="tablist"` only for its existing diagnostic/chain/parameter/JSON tabs, adding correct `role="tab"`, `aria-selected`, `aria-controls`, and keyboard navigation if currently absent. Table/grid view becomes a labelled segmented `role="radiogroup"` with two buttons carrying `role="radio"`, `aria-checked`, and 44/40 geometry. These changes preserve existing events exactly: `change-view`, `probe-selected`, `clear-selection`, `probe-untested`, and `clear-probe-data`.

Alternative considered: build a new generic compatibility wrapper for legacy controls. Rejected: it introduces a private parallel API and hides unsafe control ownership rather than consolidating it.

### D6. Test gates and acceptance evidence

Backend tests create fixtures containing oversized provider evidence and identity evidence, then assert: summary shape allows exactly the projected keys; each media entry is boolean; name aliases do not occur; body <=51,200 bytes; cursors are deterministic/non-overlapping; missing/unknown exact detail keys return 422/404; detail preserves safe evidence; recursive secret scans find no credential keys/values in either response. They must use in-process ASGI and a test database, never a live probe.

Frontend unit tests cover summary-envelope normalization/pagination merge, late-detail response suppression, unavailable/retry behavior, and no probe mutation during retry. Playwright intercepts all `/api/**` calls with fixtures. At 375, 768, 1024, and 1440 CSS-pixel widths in both `light` and `dark`, it asserts document `scrollWidth <= clientWidth`, summary endpoint byte-constrained pagination behavior, drawer detail request only after open, loading/unavailable/retry semantics, selected segment and focus accessibility, target geometry, virtual table/card/mobile access, color-status text coexistence, and no console errors. It also asserts the computed canvas colors and dark canvas is not near-black. Browser screenshots inform Critic but do not replace DOM assertions.

## Risks / Trade-offs

- [Progressive loading means filters initially operate on an incomplete summary set] → clearly mark loading/progress and preserve the current rendered results; do not claim collection completion until `has_more` is false.
- [Direct external callers expect the old map and aliases] → explicitly version the behavior as a breaking read contract in the changelog/spec; no silent compatibility endpoint is retained.
- [A normalized `node_key` could be missing from a ledger row] → do not request detail for that row; render summary-only/unavailable state and cover it with a fixture test.
- [Background pages race drawer detail or page navigation] → use AbortController and exact-key/sequence guards; stale responses cannot overwrite current selection.
- [Legacy `style.css` contains broad rules] → audit touched selectors, migrate only controlled areas, and rely on cross-theme real-browser tests; do not run a blind global replacement.
- [Evidence data can contain secrets] → summary excludes it entirely; detail only uses existing allowlisted/sanitized fields and recursive test scanning.
- [Payload budget logic regressions] → measure actual encoded envelope after every candidate addition rather than estimate object size.

## Migration Plan

1. Re-read the current target files and existing tests immediately before edits. Add failing backend contract/security/size tests and frontend pagination/drawer tests before application code.
2. Implement pure serializers and cursor query logic; retain the existing persistence writer, cache, and all probe execution paths. Add the exact-key detail route behind the existing router/auth policy.
3. Adapt API wrappers, app-summary progressive loading, NodeLedger background page merging, and drawer state machine. Keep all mutation endpoints and their calls unchanged.
4. Apply the frozen semantic tokens and migrate only shared Button plus the allocated Ledger Action Bar/segments/drawer controls. Preserve virtual table 36px density and existing breakpoint behavior.
5. Run focused backend and frontend tests, typecheck, strict visual audit, production build, and fixture-backed Playwright tests. No live probe, real node mutation, container rebuild, gateway restart, deploy, or configuration write is part of this task.
6. If a release candidate fails, roll back only the application/source diff. Stored records need no rollback because none were migrated or rewritten. Existing historical detail remains in the database throughout.
