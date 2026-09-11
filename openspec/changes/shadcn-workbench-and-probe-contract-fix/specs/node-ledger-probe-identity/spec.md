## Purpose

Defines one stable, privacy-safe identity and status contract between the current node ledger, persisted probe summaries, live probe updates, and paged ledger presentation.

## ADDED Requirements

### Requirement: Canonical current-node probe identity

The system SHALL derive the current ledger node identity and persisted probe identity from the exact tuple `name|type|server:port`, after trimming string components and serializing a missing port as an empty segment. The Node Ledger collection SHALL return a non-empty `node_key` for every current ledger row, and the probe summary collection SHALL retain a `results` map keyed only by the same exact `node_key`.

No client SHALL construct, normalize, fuzzy-match, prefix-match, or infer this identity from a display name, endpoint, or a partial key. A display name is not unique identity.

#### Scenario: Current ledger node matches persisted summary by exact key

- **WHEN** a current ledger node has the same canonical identity as a persisted probe record
- **THEN** its returned `node_key` exactly equals the key in the summary `results` map and the client can obtain that record without a name lookup

#### Scenario: Reused display name does not cross-associate probe evidence

- **WHEN** two nodes have the same display name but different type, server, or port
- **THEN** their distinct canonical keys keep their status and evidence separate and the client MUST NOT select either record solely by name

### Requirement: Bounded identity-bearing probe summary

`GET /api/probe/results` SHALL remain a bounded paged collection ordered by `node_key` with a `results`, `next_cursor`, and `has_more` envelope. Every `results` value SHALL contain exactly `node_key`, `name`, `status`, `latency_ms`, `speed_mbps`, `country`, `ip`, and `media`; the map key and returned `node_key` MUST be equal.

The summary SHALL not contain server, port, type, ASN, organization, error text, checked time, identity evidence, provider diagnostics, raw bodies, credentials, authorization data, cookies, tokens, or proxy connection secrets. Its existing payload limit and cursor continuation guarantees remain in force.

#### Scenario: Summary projects a minimal identity pair

- **WHEN** a caller requests a page containing persisted probe record `k`
- **THEN** `results[k].node_key` equals `k`, `results[k].name` is the record display name, and no prohibited detailed or sensitive field is present

#### Scenario: A summary page remains privately bounded

- **WHEN** detailed probe evidence or an unusually large media observation is stored for a record
- **THEN** the collection response uses only its allowed summary fields, obeys its configured UTF-8 byte ceiling, and retains a continuation cursor when additional eligible records remain

### Requirement: Deterministic ledger status presentation

The client SHALL resolve every ledger status, metric, filter decision, counter, card, mobile row, desktop row, detail-launch summary, and progressive-page merge by the row `node_key`. A one-off live probe response may also be associated by name only during that response lifecycle and only when the client can prove that name maps to exactly one current row; the persisted and paged summary path MUST NOT depend on that fallback.

Only the absence of a matching probe record represents `untested`. A received `ok`, `fail`, `timeout`, or `skipped` status SHALL retain a distinct visible state. A received status outside the supported set SHALL be normalized to `unknown` and rendered as a distinct unknown-state label; it MUST NOT be rendered or filtered as untested.

#### Scenario: Persisted timeout remains visible after reload

- **WHEN** the API returns a summary record with `status: timeout` for a current row and the Node Ledger reloads or merges later pages
- **THEN** the matching row visibly reports timeout rather than untested

#### Scenario: Unrecognized stored status is not hidden as untested

- **WHEN** the API returns a matching summary record with an unrecognized non-empty status
- **THEN** the client displays the distinct unknown state and does not count the row as untested

#### Scenario: No matching record is visibly untested

- **WHEN** a current ledger row has no matching summary record after all applicable summary pages have been processed
- **THEN** the client displays untested for that row and may include it in untested targeting

### Requirement: Current-set-convergent probe collection

The probe summary collection SHALL expose only records whose `node_key` belongs to the current enabled final-node ledger. The service SHALL converge persisted probe records and in-memory probe cache entries after a successful operation that changes the current enabled final-node set, including successful remote subscription refresh, manual-node recomputation, a selection-affecting subscription update, or subscription deletion.

Convergence SHALL remove records absent from the newly computed current key set, be idempotent, and be scoped exclusively to probe persistence/cache. A failed subscription refresh SHALL preserve both the current active node set and existing probe records. A late probe write for a removed node MUST remain invisible to summary reads and be removed by the next successful convergence; it MUST NOT displace a current node in pagination.

#### Scenario: Historical renamed traffic placeholder no longer consumes summary pages

- **WHEN** a successful refresh removes a former node whose display name changed with traffic metadata and no current final node has its former key
- **THEN** its persisted probe record and cache entry are removed and subsequent summary pages contain only current-node records

#### Scenario: Failed refresh does not discard usable evidence

- **WHEN** a remote subscription refresh fails and the prior active subscription nodes remain in effect
- **THEN** probe records for those prior current nodes remain available and no convergence deletion occurs from the failed attempt

#### Scenario: Late removed-node write cannot corrupt current ledger summary

- **WHEN** a probe begun before a successful refresh writes a result for a node that the refresh removed
- **THEN** summary reads exclude that result, current rows remain pageable without it, and the next convergence removes the stale record
