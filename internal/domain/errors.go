package domain

import "errors"

// Common domain errors for validation and invariant checking
var (
	ErrInvalidName              = errors.New("name cannot be empty")
	ErrInvalidURL               = errors.New("url cannot be empty")
	ErrInvalidUpdateInterval    = errors.New("update interval cannot be negative")
	ErrInvalidPort              = errors.New("port must be between 1 and 65535")
	ErrMissingServer            = errors.New("server cannot be empty")
	ErrMissingName              = errors.New("node name cannot be empty")
	ErrUnsupportedProtocol      = errors.New("unsupported protocol")
	ErrMissingRequiredField     = errors.New("missing required protocol field")
	ErrInvalidGroupType         = errors.New("invalid node group type")
	ErrInvalidRuleType          = errors.New("invalid rule type")
	ErrMissingProxy             = errors.New("proxy target cannot be empty")
	ErrInvalidRuleValue         = errors.New("rule value cannot be empty for this rule type")
	ErrInvalidProbeStatus       = errors.New("invalid probe status")
	ErrMissingNodeKey           = errors.New("node key cannot be empty")
	ErrUnsupportedExportTarget  = errors.New("unsupported export target")
	ErrNilOptions               = errors.New("options cannot be nil")
)
