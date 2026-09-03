## Purpose

确保真实用户数据库可以从现有运行版本安全演进到新领域模型，并让发布检查验证迁移、数据保全、输出兼容和可回滚性，而不依赖启动时隐式改表。

## ADDED Requirements

### Requirement: Versioned schema ownership
The system SHALL use versioned database migrations as the sole mechanism that changes persistent schema in production. Application startup SHALL verify the database revision and SHALL NOT perform ad hoc DDL repairs.

#### Scenario: Database is behind application revision
- **WHEN** an application starts against a database below the required migration revision
- **THEN** it reports a clear migration-required readiness failure and does not mutate schema until the approved migration command runs

### Requirement: Migration preflight and backup gate
Before a destructive or shape-changing migration, the system SHALL evaluate the existing database for blockers, produce a redacted summary, and require a recoverable backup artifact or explicitly verified equivalent before cutover.

#### Scenario: Preflight finds unmappable legacy record
- **WHEN** a legacy record cannot be assigned a required logical identity
- **THEN** preflight reports the record category and reason without changing production data or starting the cutover

### Requirement: Data-preserving upgrade
An approved migration SHALL preserve supported subscriptions, manual nodes, groups, rules, DNS settings, generation settings, and current security settings. Any legacy representation that cannot be faithfully migrated SHALL be quarantined with an operator-visible recovery report.

#### Scenario: Legacy JSON node data migrates
- **WHEN** a legacy database contains active subscription and manual node JSON data
- **THEN** the upgraded inventory contains corresponding provenance-preserving nodes and the migration report states imported and quarantined counts

### Requirement: Output parity gate
The release process SHALL compare normalized outputs from representative legacy fixtures before and after migration, permitting only explicitly approved compatibility differences.

#### Scenario: Unapproved output drift blocks release
- **WHEN** a fixture produces a different normalized YAML or Script.js semantic result after migration
- **THEN** the release gate fails and identifies the fixture and diff category

### Requirement: Reversible cutover
The migration workflow SHALL define a recovery path that restores the prior application and backed-up data if post-cutover readiness, migration verification, or output parity fails.

#### Scenario: Post-cutover verification fails
- **WHEN** the upgraded service fails mandatory readiness or parity verification
- **THEN** operators can restore the prior data and application version using the documented recovery procedure