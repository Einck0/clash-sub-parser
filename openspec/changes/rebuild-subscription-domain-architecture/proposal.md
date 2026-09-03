## Why

`clash-sub-parser` can still pass its current unit and build checks while the runtime model keeps accumulating incompatible sources of truth: nodes live as JSON blobs, group membership stores mutable IDs and derived mirrors, probing uses a second identity model, and production schema evolution bypasses Alembic. This makes new features expensive, makes restore/import unsafe, and turns a green test run into an incomplete signal for a live configuration manager.

The project now needs a domain-level redesign rather than another local refactor: preserve the existing user configuration and public outputs, establish one canonical data and compilation path, and make each future capability land on an auditable migration and contract.

## What Changes

- Replace blob-centered subscription/node handling with a canonical inventory that assigns stable identities, preserves source provenance, and records refresh generations without exposing subscription credentials.
- Replace mutable-ID and derived-field proxy-group configuration with explicit membership expressions and a single resolver shared by preview, YAML, Script.js, single-subscription output, and node ledger.
- Establish a versioned compilation pipeline that produces deterministic artifacts plus diagnostics, rather than allowing each endpoint to assemble its own view of nodes and groups.
- Rebuild node probing around immutable observation records, explicit probe profiles, bounded jobs, and policy evaluation; retain the distinction between successful proxy egress, network reachability, speed, and platform capability.
- Make Alembic the only production schema-evolution mechanism; add a preflighted, resumable migration from existing SQLite data and a compatibility window for current APIs and exports.
- Replace table-copy snapshot/import behavior with versioned configuration bundles that validate references before mutation, preserve security secrets locally, and restore a coherent configuration graph.
- Split the frontend into feature-owned views, typed API contracts, composables, and a shared query/mutation lifecycle so large editor pages no longer own unrelated state and transport logic.
- Strengthen release gates with migration-upgrade, compilation-parity, API-contract, and browser journey coverage. Current uncommitted probe work is preserved as input to the redesign but is not accepted as the new architecture without revalidation.

## Capabilities

### New Capabilities
- `canonical-node-inventory`: Stable node identities, source provenance, refresh generations, and normalized node lifecycle.
- `configuration-compiler`: One deterministic resolver/compiler for previews and all generated artifacts, with diagnostics and parity guarantees.
- `probe-observation-and-policy`: Immutable probe observations, bounded execution, explicit profile semantics, and filter-policy evaluation.
- `versioned-configuration-bundles`: Schema-versioned export, import, snapshot, restore, preflight, and secret-preservation contracts.
- `migration-and-release-safety`: Alembic-only schema evolution, live-data migration preflight/rollback rules, and release verification requirements.
- `control-plane-ui-architecture`: Feature-owned frontend state, typed contracts, resilient mutation lifecycle, and operator-visible compilation/probe status.

### Modified Capabilities
- `config-transfer`: Replace table serialization with coherent, versioned configuration-bundle behavior while preserving credential protections.
- `node-capability-probe`: Define probing as real per-node egress/capability observation with stable identity and controlled execution semantics.
- `node-probe-persistence-and-filtering`: Replace mutable cache/name-key behavior with retained observations and explicit policy-based filtering.
- `http-fetch-pipeline`: Require fetch provenance and normalized refresh outcomes for the canonical inventory pipeline.

## Impact

- Affects all backend domain models, Alembic revisions, fetch/schedule/probe/generate services, REST schemas/routes, and the SQLite runtime database; PostgreSQL remains supported through the same migration path.
- Affects the Vue routes, API client, stores/composables, especially `NodeLedger`, subscription, node-group, generation, settings, and configuration-history experiences.
- **Compatibility:** current `/yaml`, `/script`, management API, and existing SQLite configuration receive an explicit compatibility and migration window. Legacy fields/endpoints are removed only after parity fixtures and migration verification pass.
- Deployment remains a single-container Docker Compose application initially; this change does not introduce a mandatory external queue or database service.
- Requires a staged implementation with backups, dry-run migration reports, deterministic fixtures, and release gates before production cutover.
