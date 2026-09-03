## Purpose

使配置备份、迁移、导入和恢复围绕一个版本化的逻辑配置图运行，在任意失败时保护现有运行配置与本机认证秘密。

## ADDED Requirements

### Requirement: Versioned portable bundle
The system SHALL export configuration as a versioned bundle containing logical identifiers, declared schema version, configuration revision metadata, and all configuration required to reproduce supported outputs. The bundle SHALL NOT include authentication secrets, subscription credentials, or probe runner artifacts.

#### Scenario: Export is portable across database instances
- **WHEN** a configuration is exported from one instance and imported into another empty instance
- **THEN** logical groups, references, policies, and supported outputs are reproduced without relying on source database row IDs

### Requirement: Preflight before mutation
The system SHALL provide a non-mutating import preflight that validates bundle version, structural integrity, logical references, unsupported fields, and migration requirements before accepting an import operation.

#### Scenario: Invalid bundle is rejected before import
- **WHEN** a bundle includes a missing group reference or unsupported future schema version
- **THEN** preflight returns all detected blocking diagnostics and the active configuration remains unchanged

### Requirement: Atomic restore and import
The system SHALL apply an accepted import or restore as a single configuration-revision transition. If validation, persistence, or compilation verification fails, the active revision SHALL remain unchanged.

#### Scenario: Compilation failure rolls back restore
- **WHEN** a bundle passes syntax validation but fails required compilation verification
- **THEN** no partial configuration revision becomes active and the prior revision remains usable

### Requirement: Local-secret preservation
The system SHALL preserve local authentication settings and source credentials unless the operator explicitly performs a separate secret-management action. Importing a bundle SHALL NOT silently replace those values.

#### Scenario: Import into protected instance
- **WHEN** an operator imports a valid bundle into an instance with management authentication enabled
- **THEN** the existing management credential remains valid after import

### Requirement: Restorable revision history
The system SHALL retain bounded revision metadata and the validated bundle needed to restore each retained configuration revision, including the compiler and schema versions used for verification.

#### Scenario: Restore retained revision
- **WHEN** an operator selects a retained configuration revision
- **THEN** the system preflights and restores its logical graph atomically and reports the resulting active revision