## Purpose

Make CSP access-token entry and security-setting transitions explicit, accessible, and recoverable so operators can safely authenticate and rotate credentials without self-lockout.

## ADDED Requirements

### Requirement: Access-token entry is operable on desktop and mobile
The access gate SHALL provide a labelled token field with visible focus/field boundaries, password visibility control, paste action when Clipboard permission is available, clear action for populated input, and a submit action that remains reachable above the virtual keyboard.

#### Scenario: Mobile token entry
- **WHEN** a user focuses the token field at a 375 CSS pixel viewport with a virtual keyboard present
- **THEN** the focused field and submit action remain reachable through the page's controlled scroll layout

#### Scenario: Clipboard unavailable
- **WHEN** Clipboard access is denied or unavailable
- **THEN** the access gate reports a non-fatal paste failure and retains manual entry capability

#### Scenario: Authentication failure
- **WHEN** submitted credentials are rejected
- **THEN** the access gate keeps the entered value available for correction and exposes an assertive, human-readable error message

### Requirement: Generated Token requires recovery acknowledgement
The system SHALL not make a generated replacement Token eligible for persistence until the operator has had a visible opportunity to copy it and explicitly acknowledges retaining it.

#### Scenario: Successful generated Token copy
- **WHEN** an operator generates a Token and activates copy
- **THEN** the system reports copy success and enables the acknowledgement control

#### Scenario: Save without acknowledgement
- **WHEN** an operator has a generated replacement Token but has not acknowledged retention
- **THEN** the system prevents the settings save and explains the recovery requirement

#### Scenario: Clipboard copy failure
- **WHEN** generated Token copy fails
- **THEN** the system keeps the Token visibly selectable and does not mark it acknowledged

### Requirement: Authentication disabling is a dangerous state transition
The system SHALL require an explicit danger confirmation before persisting a change from Token authentication enabled to disabled.

#### Scenario: Cancellation retains authentication
- **WHEN** an operator cancels the confirmation to disable Token authentication
- **THEN** the pending authentication setting remains enabled and no settings request is sent

#### Scenario: Confirmed authentication disable
- **WHEN** an operator explicitly confirms disabling Token authentication
- **THEN** the system persists the requested state and presents its resulting security status
