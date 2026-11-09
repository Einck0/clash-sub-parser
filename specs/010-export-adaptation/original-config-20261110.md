# Latest original subscription configuration — current contract

Applies to `pi_run_20261110_024024_408762`, change `change_pi_run_20261110_024024_408762`, package `whole_task`, epoch 1. This supplement describes the implemented contract; it does not rewrite earlier continuation plans, failed checks, or incident evidence, and is not a verification or deployment claim.

## User goal and lifecycle

Each subscription keeps at most one latest usable original response body, before parser normalization, rename, filtering, or export conversion. A successfully committed reconciliation (including partial parse acceptance with at least one usable item) atomically replaces that body's row together with inventory facts. Fetch/HTTP/empty/parse/size/persistence/cancellation failures retain the prior copy. Source URL changes invalidate it transactionally; deletion cascades; disabling and ordinary history retention do not delete it. No historical backfill or extra fetch is performed.

## Response budget compatibility decision

The accepted bound is the existing per-subscription `RefreshPolicy.MaxResponseBytes`, translated by `fetch.OptionsFromPolicy`. A nonpositive/unset value selects the existing **10 MiB default**, not a new absolute storage ceiling. Explicitly configured larger budgets remain supported; reconciliation rejects bodies exceeding the effective configured budget before parsing or mutating inventory. The real fetch client also bounds wire and decompressed response sizes. Migration 29 requires a nonempty BLOB but intentionally does not introduce a second fixed maximum at the database layer.

This supersedes the **proposed** absolute 10 MiB cap and body length upper-bound check in the earlier continuation design. That proposal was not user wording. Imposing it would break previously configured larger subscriptions. `TestInventoryService_ReconcileSubscription_LargeConfiguredBudget` covers acceptance above 10 MiB within a 15 MiB configured budget; over-budget tests require rejection and retention of old bytes/facts. Storage remains plaintext and credential-bearing under the existing database/backup policy; configured large budgets therefore retain their existing resource implications.

## Concurrent refresh contract

The editable subscription revision is not bumped by successful refreshes. Reconciliation snapshots both that revision/source URL and the last committed original's unique fetch ID before fetching, then rechecks them before transactional mutations. A changed revision/source URL returns `subscription_revision_conflict`; a refresh committed in the meantime returns `subscription_refresh_conflict`. The losing candidate leaves no successful fetch, payload, original, or inventory mutations. This is first-commit-wins among overlapping candidates, not a guarantee that the last-started network request wins. A later ordinary refresh can succeed. There is no hidden refetch/retry.

Deterministic ordered tests cover a slow earlier refresh losing to an already committed later refresh, with/without an existing original and across one/two service instances. Concurrent-access tests allow only successful results or this specific conflict, require a commit for each subscription, check read errors, and verify committed fact counts and original/source consistency.

## Management access and confidentiality

`GET /api/v1/subscriptions/{id}/original-config` returns the stored exact bytes as a fixed-name, no-store attachment without fetching again. It follows existing management authentication: admin bearer/session access in protected mode; unauthenticated management access in deliberately configured open mode. Recognized publication/export credentials remain forbidden in either mode. Open management access is not new authorization isolation: anyone who can reach an open management API can download stored credential-bearing originals. Use protected mode or restrict network access when confidentiality is required.

Ordinary DTOs and public export routes do not expose the original body. Shared error sanitization masks complete URL userinfo (including username), secrets, and bearer tokens while preserving capability-error 422/code semantics.

## Verification boundary

Cloud run `42678767734` for SHA `4dfd59493e7a519a9c75495622db50061f83742e` failed six Go checks. The corrective work aligns stale schema/node-source/auth/redaction fixtures and the old concurrent-access expectation with the above contracts, retaining semantic assertions rather than suppressing errors. The historical failed run remains failed. Only a subsequent same-SHA cloud check, image acceptance, and independent review can establish delivery; local compilation/tests and production operations are not authorized for this repair worker.
