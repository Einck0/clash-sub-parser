## Purpose

Provides a bounded, privacy-safe read projection for large persisted probe-result collections and a separate exact-key retrieval path for evidence that an operator explicitly chooses to inspect.

## ADDED Requirements

### Requirement: Bounded paged probe-result summary collection
The system SHALL expose `GET /api/probe/results` as a summary-only paged collection. It SHALL accept optional `cursor` and `limit` query parameters, order results deterministically by `node_key`, default `limit` to 100, reject a limit outside 1 through 100 with HTTP 422, and return an envelope containing `results`, `next_cursor`, and `has_more`. `results` SHALL be keyed only by `node_key` and each value SHALL contain only `status`, `latency_ms`, `speed_mbps`, `country`, `ip`, and `media`, where every media-platform value is a boolean full-unlock summary.

The serialized UTF-8 body of every successful summary response SHALL be at most 51,200 bytes. When the next candidate would exceed that budget, the service SHALL stop before it, set `has_more` true, and return its predecessor as `next_cursor`; it SHALL not silently omit a continuation cursor. A response with no remaining results SHALL return an empty `results` object, `next_cursor: null`, and `has_more: false`.

#### Scenario: First Node Ledger page is bounded and evidence-free
- **WHEN** an operator opens the Node Ledger and it calls `GET /api/probe/results` without a cursor
- **THEN** the service returns at most 100 deterministically ordered summary records in an envelope no larger than 51,200 UTF-8 bytes and contains no diagnostics, identity evidence, ASN, organization, error text, timestamps, server fields, or duplicated name-key entries

#### Scenario: Client continues a truncated summary page
- **WHEN** a summary response is stopped by its configured record or byte budget
- **THEN** it returns `has_more: true` and an exact `next_cursor` that lets the client request the next non-overlapping `node_key` range

### Requirement: Exact-key on-demand probe evidence detail
The system SHALL expose `GET /api/probe/results/detail?node_key=<exact-node-key>` for one persisted probe result. A missing or blank `node_key` SHALL be rejected with HTTP 422, and an unknown exact key SHALL return HTTP 404. A successful result SHALL retain the safe existing probe detail semantics, including identity evidence and normalized provider diagnostics when present.

The detail response MUST NOT include proxy-node credentials, authorization headers, cookies, tokens, password fields, proxy URLs containing credentials, raw HTTP bodies, or unbounded diagnostic text. It SHALL not accept a fuzzy name, prefix, or server-address lookup as a substitute for `node_key`.

#### Scenario: Drawer loads evidence for its selected node only
- **WHEN** an operator opens the detail drawer for a summary record with node key `n`
- **THEN** the client requests the detail endpoint with the exact URL-encoded value of `n` and receives evidence only for `n`

#### Scenario: Unknown or ambiguous lookup cannot disclose evidence
- **WHEN** a caller provides a missing, empty, partial, or unknown `node_key`
- **THEN** the service rejects missing or empty input with HTTP 422, returns HTTP 404 for a non-existent exact key, and does not return another node's diagnostic or identity evidence

### Requirement: Summary and detail do not rewrite stored probe records
The system SHALL generate summary and detail responses from the existing persisted probe record without a database migration, destructive rewrite, identity-key change, or data backfill. Existing probe execution, persistence, media-evidence sanitization, cache behavior, and node capability filtering SHALL remain unchanged.

#### Scenario: Historical evidence remains available after read-projection rollout
- **WHEN** a historical persisted probe record contains legacy media values or evidence-grade diagnostic fields
- **THEN** its summary is derived safely for the collection endpoint and its valid stored evidence remains available only through the exact-key detail endpoint without changing the stored row
