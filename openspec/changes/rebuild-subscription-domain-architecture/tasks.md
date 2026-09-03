## 1. Baseline, safety fixtures, and compatibility contract

- [x] 1.1 Record the exact legacy schema fingerprint, migration head state, container health, and uncommitted probe-work diff in a redacted baseline manifest; verify the manifest contains no URLs, tokens, node credentials, or generated full configurations
- [x] 1.2 Build redacted legacy fixture bundles covering fetched/manual nodes, duplicate names, renamed nodes, groups with includes/excludes/references, rules, DNS, generation settings, and probe results; verify fixtures load in an isolated test database
- [x] 1.3 Add normalized legacy-output fixtures for YAML, Script.js, preview, and single-subscription outputs; verify the comparator distinguishes approved formatting-only differences from semantic differences
- [x] 1.4 Freeze the v1 management API and `/yaml` `/script` compatibility contract with schema/response fixtures including auth and content headers; verify contract tests run in CI
- [x] 1.5 Add a redacted database backup-manifest and restore-verification test helper; verify it refuses to continue without a recoverable source artifact

## 2. Migration authority and database bridge

- [x] 2.1 Inventory every current `create_all`, bootstrap DDL, and runtime `ALTER TABLE` effect and map each to an explicit named Alembic revision; verify no current persistent column is absent from the migration map
- [x] 2.2 Create a migration preflight command that reads schema fingerprint, rows, references, and unmappable legacy shapes without mutating a database; verify it reports a synthetic unknown schema as blocking
- [x] 2.3 Create the Alembic bridge path for databases lacking `alembic_version`, including the legacy start-state fixture; verify upgrade from that fixture reaches one deterministic revision head
- [x] 2.4 Implement target schema revisions for logical IDs, source revisions, inventory, configuration revisions, bundles, probe profiles/jobs/observations, and quarantine records; verify both SQLite and PostgreSQL migration tests reach the same metadata schema
- [x] 2.5 Implement SQLite shadow-table copy, validation, and swap helpers for shape-changing revisions; verify a forced validation failure leaves the source table and data intact
- [x] 2.6 Remove schema mutation from application startup and replace it with revision/readiness checking; verify a behind database yields migration-required readiness and executes no DDL
- [x] 2.7 Add upgrade, downgrade/recovery, and row-preservation tests for every supported legacy start state; verify all cases preserve supported records and report quarantined records deterministically

## 3. Canonical source and node inventory

- [x] 3.1 Define domain models, schemas, and repositories for Source, SourceRevision, Node, NodeSourceLink, manual-source ownership, logical IDs, and lifecycle states; verify repository tests do not use display name or row ID as identity
- [x] 3.2 Implement protocol payload normalization and versioned safe fingerprinting with fixtures for supported proxy formats; verify equivalent renamed payloads share identity while semantic protocol differences do not
- [x] 3.3 Implement atomic source-refresh reconciliation that publishes a generation only after fetch, parse, normalize, validate, and link reconciliation succeed; verify a failed refresh leaves the last active generation selectable
- [x] 3.4 Migrate legacy `raw_nodes`, `source_nodes`, and `manual_nodes` into inventory/source-link records with a redacted per-category migration report; verify fixture counts, provenance, and quarantine behavior
- [x] 3.5 Implement inventory listing/detail DTOs with safe provenance fields and pagination/filtering; verify source URLs, credentials, and raw secret parameters are absent from serialized responses and logs
- [x] 3.6 Implement explicit, tested inventory deduplication policy at compilation time rather than persistence time; verify same-name/different-payload and same-payload/different-source fixtures retain their intended provenance
- [x] 3.7 Adapt subscription refresh scheduling to publish inventory generations through the shared fetch outcome; verify scheduler failure does not delete or partially replace active inventory

## 4. Logical configuration graph and compiler

- [x] 4.1 Define logical-ID configuration models for groups, rules, DNS, generation settings, policies, ordered membership expressions, and configuration revisions; verify imports do not depend on surrogate database IDs
- [x] 4.2 Write parser and validation tests for each expression kind, unknown reference, self reference, ordered inclusion/exclusion, and circular graph; verify validation diagnostics identify expression locations safely
- [x] 4.3 Implement the pure compilation input/result API with versioned provenance, semantic fingerprint, and structured diagnostics; verify it performs no database writes or network operations
- [x] 4.4 Implement the canonical graph resolver with deterministic ordering, selectors, policy predicates, references, group-member expansion, exclusion, and fallback semantics; verify golden graph fixtures are byte-stable across repeated runs
- [x] 4.5 Implement policy eligibility adapters that distinguish valid, missing, stale, failed, incompatible, and insufficient observations; verify every exclusion exposes the correct category
- [x] 4.6 Implement YAML, Script.js, preview, ledger, and single-subscription target adapters consuming only the canonical compilation result; verify target-parity fixtures contain the same semantic node/group sets
- [x] 4.7 Add compiler cache invalidation keyed by configuration revision, inventory generation, policy revision, compiler version, and target; verify a change to any input invalidates only affected results
- [x] 4.8 Add legacy-to-logical configuration importer and shadow compiler comparison; verify unapproved semantic output drift blocks the compatibility gate

## 5. Probe profiles, jobs, and observations

- [x] 5.1 Define ProbeProfile, ProbeJob, ProbeJobItem, ProbeObservation, platform-result, and retention schemas linked to canonical node identity; verify repeated observations remain immutable and queryable
- [x] 5.2 Port current real egress, geo-consensus, media/AI, and speed probes behind a profile-aware provider interface; verify host proxy inheritance is disabled and platform challenges are not reported as transport failure
- [x] 5.3 Implement job orchestration with global and per-job concurrency, byte/duration budgets, port leases, cancellation, cleanup, and terminal statuses; verify a forced timeout/cancellation reclaims all test resources
- [x] 5.4 Replace name-key result upserts and process-local cache assumptions with observation queries and explicit eligibility selection; verify renamed and duplicate-name nodes receive independent observations
- [x] 5.5 Implement periodic scheduler submission with overlap prevention and observable queued/running/skipped/completed states; verify two schedule ticks cannot create equivalent concurrent jobs
- [x] 5.6 Migrate or quarantine legacy probe records with profile provenance marked as legacy where unavailable; verify migration output counts are deterministic and filtering treats incompatible legacy records correctly
- [x] 5.7 Replace subscription and group filtering fields with policy DTOs and compiler evaluation; verify stale/missing/failed observations never satisfy positive speed or unlock requirements
- [x] 5.8 Add redaction tests for job status, errors, logs, and observation APIs; verify no node credentials or source secrets appear in failure output

## 6. Versioned bundles, revision history, and secret boundary

- [x] 6.1 Define versioned logical configuration-bundle schema, bundle migrators, checksum, and revision metadata; verify a bundle round-trips through JSON without row IDs or secret fields
- [x] 6.2 Implement non-mutating import preflight for schema version, references, unsupported fields, compiler validity, and migration requirements; verify it returns multiple blockers without changing active configuration
- [x] 6.3 Implement atomic revision creation, activation, and rollback with compilation verification inside the transaction boundary; verify a forced persistence or compilation error leaves the previous revision active
- [x] 6.4 Implement bounded configuration history and protected retention cleanup; verify active, rollback-protected, and referenced revisions are never deleted by cleanup
- [x] 6.5 Replace snapshot restore and table-copy export/import/reset routes with bundle/revision adapters while preserving v1 response semantics during the compatibility window; verify current auth credentials survive all import/reset/restore paths
- [x] 6.6 Add cross-instance restore tests using databases with deliberately different surrogate IDs; verify equivalent logical configuration and generated outputs are restored
- [x] 6.7 Separate source-secret and management-secret handling from configuration bundles and provide explicit safe labels; verify exported bundles, revision history, and diagnostics contain no credential material

## 7. Backend API transition and operational contracts

- [x] 7.1 Introduce typed v2 inventory, compilation, diagnostics, policy, job, observation, bundle-preflight, and revision endpoints with explicit pagination and error envelopes; verify OpenAPI contract tests cover every endpoint
- [x] 7.2 Implement v1 adapters for existing subscription, node-group, probe, generation, history, export/import, `/yaml`, and `/script` behavior; verify legacy API fixture compatibility throughout shadow mode
- [x] 7.3 Replace router-owned transaction and scheduler logic with domain services and repositories whose transaction boundaries are explicit; verify service tests cover rollback, retries, and no hidden commit paths
- [x] 7.4 Add authorization/CSRF coverage for every new mutating endpoint and export-compatible query-token path; verify unauthenticated, cookie-authenticated, and CSRF-invalid requests have correct outcomes
- [x] 7.5 Add resource-boundary tests for fetch, YAML/parsing, expression payloads, probe requests, downloads, and generated output size; verify failing requests do not partially publish source, revision, or job state
- [x] 7.6 Add operational readiness checks for migration revision, active configuration revision, scheduler leadership, and runner availability; verify readiness never reports healthy with an unresolved migration requirement

## 8. Frontend feature-slice reconstruction

- [x] 8.1 Establish typed generated or schema-derived API contracts, typed transport errors, query keys, abort/request-sequence support, and entity-level mutation pending keys; verify an old delayed response cannot overwrite a newer query result
- [x] 8.2 Split subscriptions into feature-owned list, editor, refresh-state, and source-generation components; verify saving a subscription preserves unrelated drafts and reports refresh failures safely
- [x] 8.3 Replace the monolithic node ledger with inventory filter/table/detail, probe-history, policy-explanation, and virtualized result components; verify large fixture rendering remains bounded and selection changes cancel stale work
- [x] 8.4 Rebuild group and policy editors around ordered expression controls and compiler diagnostics; verify invalid references/cycles are displayed without replacing the operator draft
- [x] 8.5 Rebuild generation and history screens around active revision, inventory generation, compilation fingerprint, output diagnostics, bundle preflight, and restore confirmation; verify stale generated output is never presented as current after an invalid compile
- [x] 8.6 Apply shared accessible dialog, confirmation, loading, error, and live-status primitives to all destructive and asynchronous operations; verify keyboard escape, focus restoration, accessible names, and duplicate-submit prevention
- [x] 8.7 Preserve v1 routes during the compatibility window and cut each feature slice behind a flag; verify browser journey tests cover create/edit/cancel/error/retry/navigation behavior for each slice

## 9. Quality gates, shadow operation, and release rehearsal

- [x] 9.1 Add CI gates for Alembic upgrade paths, SQLite/PostgreSQL schema parity, golden compiler parity, API contracts, backend unit/integration tests, frontend typecheck/unit tests, and browser journeys; verify a deliberate fixture or migration drift fails the relevant gate
- [x] 9.2 Add a redacted secret scan and artifact policy for fixtures, bundles, logs, snapshots, and CI reports; verify test fixtures cannot contain private URLs, tokens, or proxy credentials
- [x] 9.3 Implement shadow read mode that compares legacy and new compiler output without changing live writes; verify it records only safe diff summaries and can be disabled immediately
- [x] 9.4 Write and rehearse the production runbook: backup verification, preflight, migration, data counts, compiler parity, API smoke, feature-flag progression, and rollback; verify rehearsal uses a disposable copy and leaves production untouched
- [x] 9.5 Execute pre-cutover verification on a fresh clone and clean database, then on a sanitized production-shaped copy; verify all commands, expected artifacts, and failure exit codes are documented
- [x] 9.6 During an approved maintenance window, execute backup → preflight → migration → verification → read flag → write flag with recorded outcomes; verify ready, parity, `/yaml`, `/script`, and management API smoke before declaring success

## 10. Compatibility retirement and post-cutover cleanup

- [x] 10.1 Monitor the declared compatibility window for v1 adapter usage, compiler diffs, job cleanup failures, migration alerts, and restore results; verify no unresolved P1 defect remains before retirement
- [x] 10.2 Remove legacy JSON node mirrors, runtime schema DDL, table-copy snapshot/import paths, duplicate resolvers, and name-key probe caches only after compatibility criteria are met; verify dead-code search and migration tests find no remaining runtime schema mutation
- [x] 10.3 Archive legacy data only through the verified recovery-bundle path and retain migration audit metadata; verify a restoration drill can recover a supported revision without secrets in the bundle
- [x] 10.4 Update operator documentation, API migration guide, rollback guide, architecture document, and OpenSpec status; verify docs match the final tested commands and observable behavior
- [x] 10.5 Run the final independent code audit and deployment review, resolve all blocking findings, and validate the completed OpenSpec change before archival; verify the audit result is APPROVED and `openspec validate` passes
