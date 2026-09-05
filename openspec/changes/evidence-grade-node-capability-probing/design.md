## Context

See proposal.md and its delta specifications. CSP is a migration/evolution project, not a clean-slate replacement: the existing FastAPI probe route resolves a frozen `ResolvedProbeConfig`, `probe_single_node()` owns a candidate node workflow, and that workflow calls both `spawn_node_runner()` and `check_media_unlock()`. Graphify evidence after a code-only extraction is 2,862 nodes and 6,156 edges; `probe_single_node()` is the tenth highest-degree symbol (24 edges), and the two critical call paths are direct one-hop edges to the runner and media dispatcher. Therefore changes must enter at the provider/result boundary, not fork the entire execution path.

The current chain already has the correct security nucleus: a per-node sing-box mixed listener, `proxy=<node loopback URL>`, and `trust_env=False`. It also has stable storage (`node_probe_results.media` JSON), a legacy-compatible key (`name|type|server:port`), frontend media filters, compact preview badges, and LedgerDrawer details. The weak point is semantic: provider functions return unversioned ad-hoc dictionaries. For example, a Netflix 200 is currently enough to claim full access, Disney+ checks a landing redirect rather than a supported-location flow, and identity falls back to a single Cloudflare trace response. These are not sufficient claims under CDN routing, bot challenges, endpoint drift, or a generic page response.

External reconnaissance supports the architectural direction but not direct code reuse. RegionRestrictionCheck tests Netflix using original and non-original titles, derives provider-specific regions, has a distinct YouTube CDN mapping check, and uses a global request deadline with bounded curl retries.[1] Its script also embeds live cookies/tokens in some providers, so CSP MUST NOT copy that pattern. Subs-Check combines availability, speed, streaming checks, conversion, scheduling, and a control panel, and warns that generic public speed endpoints may be blocked by nodes.[2] SubsNetflixCheck creates a local Clash path to test whole subscriptions and preserves results for later inspection.[3] Subconverter is a format conversion utility rather than a quality-check authority and supplies no compatible unlock-verdict engine.[4]

## Goals / Non-Goals

**Goals:**

- Produce a versioned provider-catalogue contract that makes each availability assertion reproducible from sanitized evidence.
- Preserve genuine candidate-node egress throughout identity, platform, and routing observations.
- Separate exit identity from CDN routing and separate platform restriction from host/network failure.
- Preserve all existing non-SCRIPT functionality, stable platform keys, filters, historical records, exports, and responsive workbench semantics.
- Make the diagnostic UI explain results compactly and accessibly at every required viewport.

**Non-Goals:**

- No browser automation, account login, user credential ingestion, private API token, copied cookie, CAPTCHA bypass, or static secret embedded in source or configuration.
- No direct host connectivity fallback, dual-write schema, destructive migration, re-keying of probe results, replacement of sing-box, or rewrite of parser/export code.
- No claim that CDN edge/IATA routing is physical node location or that a provider probe proves a user's logged-in entitlement.
- No expansion to every service found in external scripts in this change; the initial catalogue supports the present CSP platform set and adds routing evidence, not a giant unmaintainable matrix.

## Decisions

### D1. Evolve the existing node workflow at the provider/result boundary

`probe_single_node()` remains the sole owner of runner lifecycle, handshake liveness authority, node-wide budget, batch semaphore occupancy, cache, and persistence. The provider layer becomes a catalogue-driven evaluator and returns a normalized `ProviderResult`; service code composes it into the existing `media` mapping. No provider may launch an independent proxy, use system proxy variables, or bypass the runner.

The normalized result is additive and follows this shape:

```json
{
  "status": "verified|partial|restricted|ip_blocked|challenged|rate_limited|timeout|transport_error|inconclusive|disabled",
  "verdict": "full|originals_only|available|unsupported_region|blocked|challenge|rate_limited|unknown",
  "unlocked": true,
  "region": "US",
  "checked_at": 0,
  "evidence_version": "catalogue-2026-09-05",
  "confidence": "verified|conflicted|unavailable",
  "evidence": {
    "http_status": 200,
    "final_host": "www.netflix.com",
    "redirect_class": "none",
    "signals": ["non_original_available", "original_available"],
    "elapsed_ms": 0
  }
}
```

`evidence` is size-capped and allowlisted. It stores neither body text nor headers other than an approved response status/redirect classification. Existing simple fields (`status`, `label`, `region`, `unlocked`) continue to be populated when their semantics are known. Unknown responses become `inconclusive`; they do not inherit a happy-path default.

Alternatives rejected:

- Replacing CSP with a shell port of RegionRestrictionCheck would lose per-node structured lifecycle guarantees, introduce process/parser fragility, and risks copying secret-bearing upstream request recipes.
- A generic HTTP status classifier cannot distinguish an actual streaming entitlement-region signal from a CDN landing page.
- Replacing the media JSON column with a normalized table would make historical migration and filter consumers unnecessarily risky. JSON is the correct additive evidence envelope for this increment.

### D2. Use a declarative, credential-free catalogue with fixture-owned contracts

A catalogue entry owns the platform key, probe kind, non-secret request description, accepted positive and negative signals, priority, evidence version, and optional bounded fallback. Implementations may share small evaluator primitives (HTML marker, JSON field, redirect, multi-title catalogue) but providers own their semantic maps.

Initial catalogue decisions:

| Platform | Probe shape | Positive proof | Negative/inconclusive treatment |
| --- | --- | --- | --- |
| Netflix | original + non-original title evidence | both signals = `full`; original-only = `originals_only` | restriction/redirect marker = restricted; bot page/429 = challenged/rate-limited; unmatched body = inconclusive |
| YouTube Premium | public premium page | known availability marker plus extracted region | country-unavailable marker = restricted; Google challenge = challenged; unknown page = inconclusive |
| Disney+ | credential-free supported-location public flow only | explicit supported-country signal | no static bearer/cookie flow; if a non-secret stable flow cannot prove support, return disabled/inconclusive rather than fabricated `ok` |
| ChatGPT, Gemini, Meta AI | public availability/status pages | explicit service-supported marker or documented public status response | country message = restricted; access-control challenge/429 remains distinct |
| Bilibili | public playback API region code | documented success/region outcome | documented region code = restricted; unknown JSON = inconclusive |
| YouTube CDN | mapping response | route/IATA mapping recorded as routing hint | never used to populate `country` or platform unlock |

Every active entry gets fixtures for verified, restricted, partial where applicable, challenge/rate-limit, and unrecognized contract drift. A provider is enabled only after its fixtures pass. This creates a safe review path when an upstream service changes behavior: update the evidence version, add fixtures, review, then activate; it is not acceptable to change a string marker directly in a live code path.

### D3. Identity uses consensus; CDN is independent evidence

Implement `IdentityObservation` separately from `ProviderResult`. At least two independent providers are attempted through the exact node loopback proxy, using the same service deadline. A successful verified identity requires equality of normalized IP and ISO country from at least two providers. ASN/organization are carried only when they agree or are explicitly attributed; they must not be manufactured from a Cloudflare trace fallback.

Results:

- agreement: expose `ip`, `country`, `asn`/organization if consensus supports them, plus `identity_evidence.confidence=verified`;
- one success/one unavailable: expose no verified identity and `confidence=unavailable`;
- disagreement: expose no node country/ASN claim and `confidence=conflicted`;
- all unavailable: retain provider failure observations without host fallback.

YouTube mapping is rendered beside, never inside, identity evidence. A CDN can legitimately route a user to another region; mixing those would corrupt country filters and give operators a very persuasive lie. Tiny evil, enormous consequences.

### D4. Deadline, retry, and cancellation budget stays service-owned

The existing service-level deadline remains the outer wall-clock deadline for one catalogue evaluation, including permitted fallback work. A transient network failure can make at most one retry only if it is idempotent and remaining wall-clock budget permits it. Restrictions, IP blocks, challenges, 429s, and unrecognized contracts do not retry. `asyncio.wait_for` wraps the complete provider evaluator and preserves the existing independent concurrent gathering model.

The runner is still cleaned up by its async context manager. A provider result never downgrades a successfully transported node to failed. Conversely, transport failure means no platform/identity/speed request runs, so no accidental host path can appear in a partial result.

### D5. Preserve compatibility in both persistence and selection semantics

No Alembic schema migration is required for `NodeProbeResult.media`: it is already a JSON object. The persistence contract performs a structural additive merge when it writes a new observation, retaining known legacy simple keys while replacing the same platform's stale result atomically. Existing fields and result keys remain readable. No legacy row is bulk-rewritten.

A single pure predicate owns filter semantics across NodeLedger and subscription/node-group resolution:

- verified `full` or verified generic `available` passes;
- `originals_only` is visible but does not pass a full Netflix requirement;
- all restricted/error/inconclusive outcomes fail required-unlock filtering;
- recognized historical `full`, `originals`, and prior generic `ok` retain their established consumer behavior until new data supersedes them.

This resolves an existing duplication hazard: NodeLedger currently checks its own status list while preview components independently decide which badges to show. Both must consume the same shared helper/normalizer, with frontend presentation only mapping the normalized result to label/color.

### D6. Explainability UI uses existing responsive primitives and tokens

`LedgerDrawer` becomes the authoritative proof surface. It presents an evidence table/list with provider, verdict, region, confidence, sanitized signal summary, evidence version, and time. NodePreviewList and mobile compact Ledger cards show only short semantic badges and accessible titles; tapping/opening the existing details panel exposes the proof.

Visual contract (inherited and mandatory):

- `AppModal`: `sm=448px`, `md=640px`, `lg=960px`; no local modal sizing;
- `AppDrawer`: shared desktop drawer and mobile Bottom Sheet below the established breakpoint; no private width/radius;
- 4px/8px token rhythm; existing semantic tokens for success, warning, danger, and neutral; `tabular-nums` for codes and timings;
- no card soup or glassy dashboard surface: dense bordered diagnostic rows, not big decorative cards;
- 44px minimum interactive target at mobile widths, `focus-visible`, Escape/focus restore, reduced-motion, safe area, `min-w-0`, and no document horizontal overflow.

The UI deliberately does not expose raw response bodies. It must say “inconclusive: contract signal unrecognized” instead of pretending it knows why a remote vendor changed a page.

### D7. Testing is fixtures first, then integration and real DOM geometry

Backend verification layers:

1. Pure catalogue tests: each fixture maps deterministically to the expected normalized result, secret redaction, confidence, and legacy compatibility fields.
2. HTTP transport tests: assert each client receives `proxy=<runner loopback>` and `trust_env=False`; force environment proxies and prove no direct fallback is used.
3. Deadline/cancellation tests: provider timeout keeps sibling results, stops retries, releases the semaphore/runner port, and does not corrupt node status.
4. Persistence/filter tests: history remains readable; legacy full passes; partial/inconclusive does not pass a full-unlock requirement.
5. API tests: original route request shapes remain valid and updated responses include additive fields.

Frontend verification layers:

1. Pure normalization/filter tests for every status, historical compatibility, and full vs originals-only behavior.
2. Component tests for badge labels and Drawer evidence fields with no secret render.
3. Actual browser/DOM tests at 375x812, 768x900, 1024x900, 1440x900: `scrollWidth <= innerWidth`, all proof controls visible/reachable, action targets at least 44px where applicable, and measured shared overlay geometry.

### D8. Critic findings are converted into deterministic UI gate criteria

There is no independently supplied Critic report on this card. Rather than invent one, this design turns the already established visual-risk evidence into checkable acceptance gates: no private geometry; no card-soup redesign; dense semantic diagnostic rows; status is never determined by color alone; visual state matches the normalized verdict; and four viewport DOM geometry assertions are required. The independent review card must execute these checks against the concrete diff and can reject aesthetic regression, unexplainable status, or secret exposure.

## Risks / Trade-offs

- [Public endpoints and HTML markers drift] → versioned catalogue fixtures, `inconclusive` quarantine, and a review-required provider update policy; never silently promote to unlock.
- [Two identity providers increase request cost] → run only after transport success, use existing service deadline, bounded concurrency, and no retries after definitive provider responses.
- [Strict consensus can leave more nodes without a country] → this is intentional honesty; unknown/conflicted is safer than wrong flags and broken country filtering.
- [A public provider later needs browser/session behavior] → mark disabled/inconclusive and defer to a future explicitly authorized browser-probe design; do not smuggle credentials into CSP.
- [Legacy consumers rely on loose `ok` values] → a single compatibility predicate is fixture-tested with historical payloads; API stays additive.
- [Diagnostic data becomes verbose or leaks information] → allowlisted, size-capped evidence and no raw request/response artifacts; the UI presents summaries only.
- [External scripts contain unsafe secrets] → only borrow test principles, never request bodies, cookies, bearer tokens, or opaque payloads.[1]

## Migration Plan

1. Add fixture corpus and normalization types with no runtime provider behavior change; run the test suite red-to-green for each contract before moving a provider.
2. Add a catalogue evaluator alongside existing platform functions, adopt one provider at a time, and retain legacy-compatible fields in result output.
3. Add identity consensus and CDN-routing evidence without changing historical `node_probe_results` rows or node keys.
4. Route existing NodeLedger, preview, subscription, and node-group decisions through the shared predicate; add evidence presentation through existing Drawer/mobile renderers.
5. Run backend, frontend type/build, and real-DOM viewport gates; independent review verifies source scan, diff, fixtures, and deployed behavior only after implementation work is complete.
6. Deploy with normal image rollout only after review approval. Rollback is application-image rollback: the JSON evidence payload is additive and old code ignores unknown fields. Do not run a destructive database downgrade or delete probe history.

## Open Questions

- Provider endpoints and marker fixtures will be revalidated by the implementation executor immediately before activation. This is a maintenance-input refresh, not a scope decision: a drifted provider remains `inconclusive`/disabled under this fixed design until its reviewed fixture update is available.

## Sources

[1] https://github.com/lmc999/RegionRestrictionCheck
[2] https://github.com/beck-8/subs-check
[3] https://github.com/missuo/SubsNetflixCheck
[4] https://github.com/tindy2013/subconverter
