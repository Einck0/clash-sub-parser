## Purpose

为配置控制台建立按领域组织、具有明确加载与保存状态的前端交互契约，使复杂编辑流程不会因为巨型页面、无类型传输和竞态刷新而丢失操作或显示陈旧结果。

## ADDED Requirements

### Requirement: Feature-owned control-plane state
The user interface SHALL organize subscription, inventory, groups, policies, generation, history, and security interactions into feature-owned state and typed API contracts. A view SHALL NOT require unrelated feature state to perform its primary operation.

#### Scenario: Editing a group does not reload unrelated draft
- **WHEN** an operator saves a proxy-group expression
- **THEN** the UI refreshes only the affected group, compiler diagnostics, and explicitly dependent previews without discarding an unrelated unsaved subscription draft

### Requirement: Mutation lifecycle visibility
Every state-changing action SHALL expose pending, success, validation-failure, transport-failure, and retryable states. The UI SHALL prevent duplicate submission of the same mutation while it is pending.

#### Scenario: Double save is prevented
- **WHEN** an operator activates the same save action repeatedly before the first response completes
- **THEN** one mutation is submitted and the UI exposes the pending state until it resolves

### Requirement: Response ordering and cancellation
The UI SHALL prevent an older asynchronous response from overwriting a newer selection, refresh, or mutation result.

#### Scenario: Rapid inventory filter changes
- **WHEN** an operator changes an inventory filter twice before the first request completes
- **THEN** the interface displays results for the latest filter only

### Requirement: Explainable compilation and policy status
The UI SHALL show active configuration revision, inventory generation, compilation validity, and safe diagnostics for generated output and policy exclusions.

#### Scenario: Generate with invalid configuration
- **WHEN** compilation detects a circular group reference
- **THEN** the UI presents the relevant diagnostic and does not present stale output as current

### Requirement: Accessible operator controls
Interactive controls, dialogs, destructive confirmations, validation errors, and asynchronous status SHALL be keyboard-operable and have accessible names and status feedback.

#### Scenario: Keyboard user cancels destructive action
- **WHEN** a keyboard user opens a destructive confirmation dialog and presses Escape
- **THEN** the dialog closes safely, focus returns to the invoking control, and no mutation is sent