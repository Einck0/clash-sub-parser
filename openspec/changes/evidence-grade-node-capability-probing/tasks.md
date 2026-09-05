## 1. Provider contract baseline and secure backend core

- [x] 1.1 Add a versioned, credential-free provider catalogue and normalized provider/identity result types under the existing probe boundary; write fixtures for verified, partial, restricted, IP-blocked, challenge, rate-limit, timeout, transport-error, and unrecognized responses, and verify every evaluator fixture test first fails then passes.
- [x] 1.2 Refactor the existing media/AI evaluators to use the catalogue while retaining all current CSP platform keys and legacy-compatible availability fields; verify current probe orchestration tests plus new Netflix full/originals-only and contract-drift tests pass.
- [x] 1.3 Replace single-source geo fallback with two-provider consensus and add a separately typed YouTube CDN-routing observation; verify agreement, one-unavailable, disagreement, and CDN-country-mismatch fixtures never substitute host egress or overwrite verified exit identity.
- [x] 1.4 Enforce the existing service deadline across each catalogue evaluation and at most one retry only for transient transport failure; verify a fake host proxy environment, timeout, cancellation, and definitive-restriction test prove loopback proxy pinning, `trust_env=False`, no inappropriate retry, sibling-result preservation, and runner/port cleanup.

## 2. Additive persistence and capability-selection compatibility

- [x] 2.1 Persist normalized evidence additively in the existing media JSON payload without an identity-key rewrite or historical data migration; verify a mixed historical/new-result persistence round trip retains old platform semantics and exposes sanitized additive evidence.
- [x] 2.2 Centralize the verified-only full-unlock predicate used by subscription/node-group resolution and frontend-consumable probe interpretation; verify legacy full values still pass, while originals-only, restricted, challenged, rate-limited, timeout, transport-error, and inconclusive values fail a required full-unlock filter.
- [x] 2.3 Extend route/schema/API regression coverage for additive result fields while preserving existing request shapes; verify focused backend tests pass and no response includes secret-like headers, cookies, proxy credentials, or raw bodies.

## 3. Evidence-first responsive node diagnostics

- [x] 3.1 Update NodeLedger domain normalization/filter helpers and API types to consume the stable provider result contract; verify pure frontend tests cover every normalized state, full versus originals-only filtering, and historical payload compatibility.
- [x] 3.2 Update existing NodePreviewList, LedgerDrawer, and compact mobile ledger renderer to show semantic badges plus reachable evidence explanation (verdict, region, confidence, sanitized signal summary, evidence version, timestamp); verify component tests prove inconclusive/partial results never receive an unlock-success badge and no evidence surface can render a secret.
- [x] 3.3 Apply the established visual contract to new diagnostic UI only: shared AppModal/AppDrawer sizing, existing semantic tokens, dense bordered rows, 4px/8px rhythm, tabular numerics, focus-visible, and no private geometry; verify real browser DOM checks at 375x812, 768x900, 1024x900, and 1440x900 assert no horizontal overflow, 44px interactive targets on narrow widths, and shared overlay geometry.

## 4. Integrated verification and independent review gate

- [x] 4.1 Run focused backend provider/orchestration/persistence tests, frontend unit tests, typecheck, build, and visual/DOM geometry checks; verify every command succeeds with real output and graphify update reflects changed source dependencies.
- [ ] 4.2 Have an independent reviewer inspect the final diff against all four delta specs and design, independently rerun the declared verification, audit host-egress isolation, redaction, legacy filtering, fixture coverage, and responsive geometry; verify an APPROVED verdict before any deployment or runtime rebuild.
