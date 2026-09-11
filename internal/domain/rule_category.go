package domain

// RuleCategory represents a logical grouping or tier of traffic routing rules.
type RuleCategory struct {
	ID          int64  `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	SortOrder   int    `json:"sort_order" yaml:"sort_order"`
	Enabled     bool   `json:"enabled" yaml:"enabled"`
}
