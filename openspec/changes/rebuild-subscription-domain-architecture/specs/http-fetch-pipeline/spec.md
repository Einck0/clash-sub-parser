## ADDED Requirements

### Requirement: Refresh provenance and normalized outcome
The shared fetch pipeline SHALL return a normalized, secret-safe outcome that identifies the requested source reference, final validated URL class, response metadata needed for refresh reconciliation, bounded body result, and categorized failure. Callers SHALL use this outcome to publish or reject an inventory generation atomically.

#### Scenario: Successful subscription refresh
- **WHEN** a validated subscription fetch completes within its redirect, timeout, and byte limits
- **THEN** the caller receives a normalized successful outcome sufficient to parse and publish one source generation without retaining raw request credentials in diagnostics

#### Scenario: Fetch failure preserves active generation
- **WHEN** a fetch fails due to redirect validation, timeout, size limit, or upstream error
- **THEN** the caller receives a categorized failed outcome and retains the prior active source generation