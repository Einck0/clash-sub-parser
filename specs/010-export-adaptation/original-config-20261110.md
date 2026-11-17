# Latest original subscription configuration — current contract

Applies to `pi_run_20261110_024024_408762`, change `change_pi_run_20261110_024024_408762`, package `whole_task`, epoch 1. This supplement describes the implemented contract; it does not rewrite earlier continuation plans, failed checks, or incident evidence, and is not a verification or deployment claim.

## User goal and lifecycle

Each subscription keeps at most one latest usable original response body, before parser normalization, rename, filtering, or export conversion. A successfully committed reconciliation (including partial parse acceptance with at least one usable item) atomically replaces that body's row together with inventory facts. Fetch/HTTP/empty/parse/size/persistence/cancellation failures retain the prior copy. Source URL changes invalidate it transactionally; deletion cascades; disabling and ordinary history retention do not delete it. No historical backfill or extra fetch is performed.

## Response budget compatibility decision

The accepted bound is the existing per-subscription `RefreshPolicy.MaxResponseBytes`, translated by `fetch.OptionsFromPolicy`. A nonpositive/unset value selects the existing **10 MiB default**, not a new absolute storage ceiling. Explicitly configured larger budgets remain supported; reconciliation rejects bodies exceeding the effective configured budget before parsing or mutating inventory. The real fetch client also bounds wire and decompressed response sizes. Migration 29 requires a nonempty BLOB but intentionally does not introduce a second fixed maximum at the database layer.

This supersedes the **proposed** absolute 10 MiB cap and body length upper-bound check in the earlier continuation design. That proposal was not user wording. Imposing it would break previously configured larger subscriptions. `TestInventoryService_ReconcileSubscription_LargeConfiguredBudget` covers acceptance above 10 MiB within a 15 MiB configured budget; over-budget tests require rejection and retention of old bytes/facts. Storage remains plaintext and credential-bearing under the existing database/backup policy; configured large budgets therefore retain their existing resource implications.

## Concurrent refresh contract

The original 2026-11-10 single-slot implementation used revision/source and last-original-fetch-ID CAS, described here as first-commit-wins. The later storage continuation `pi_run_20261110_174812_1075841` added durable request-admission fencing while separating last attempt, saved raw and applied membership (see the storage continuation in `plan.md` and S007 in `tasks.md`). This is an engineering concurrency decision, not a quotation that the user selected a particular race winner.

Current reconciliation retains the source revision/URL and original-fetch-ID checks and also allocates a monotonically increasing, per-subscription attempt sequence transactionally before fetching. Success and failed-attempt persistence must still own that sequence and the applicable source revision. A later admitted attempt invalidates older outstanding candidates even if the later attempt subsequently fails. A stale candidate returns `subscription_refresh_conflict` and cannot replace the applied generation, raw bytes or the newer attempt's diagnostic. Admission itself neither clears the applied generation nor replaces raw bytes. A failed attempt which is successfully recorded updates last-attempt evidence while retaining the previously committed raw and applied facts; without a prior applied generation it must not fabricate one.

The sequence is database admission order, not wall-clock or completion order, and applies across service instances sharing the database. The editable subscription revision is not bumped by refresh. Revision/source changes remain conflicts. Existing original-fetch-ID CAS remains an additional guard; the latest admitted request is not guaranteed to commit. No hidden retry or refetch revives a superseded candidate. Cancellation or failed persistence must not be reported as a successfully saved diagnostic. A later ordinary refresh may proceed normally.

The existing newer-commit regression covers stale success after a newer success. Additional deterministic barrier regressions are required for newer admission followed by failure, for an older candidate completing while the newer request is pending, and for late stale failure after newer success, checking last-attempt/raw/applied/member consistency and absence of stale writes. Their presence in source or this contract is not a claim of cloud verification; use the actual integrated-SHA evidence.

## Management access and confidentiality

`GET /api/v1/subscriptions/{id}/original-config` returns the stored exact bytes as a fixed-name, no-store attachment without fetching again. It follows existing management authentication: admin bearer/session access in protected mode; unauthenticated management access in deliberately configured open mode. Recognized publication/export credentials remain forbidden in either mode. Open management access is not new authorization isolation: anyone who can reach an open management API can download stored credential-bearing originals. Use protected mode or restrict network access when confidentiality is required.

Ordinary DTOs and public export routes do not expose the original body. Shared error sanitization masks complete URL userinfo (including username), secrets, and bearer tokens while preserving capability-error 422/code semantics.

## Verification boundary

Cloud run `42678767734` for SHA `4dfd59493e7a519a9c75495622db50061f83742e` failed six Go checks. The corrective work aligns stale schema/node-source/auth/redaction fixtures and the old concurrent-access expectation with the above contracts, retaining semantic assertions rather than suppressing errors. The historical failed run remains failed. Only a subsequent same-SHA cloud check, image acceptance, and independent review can establish delivery; local compilation/tests and production operations are not authorized for this repair worker.
