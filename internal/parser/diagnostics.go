package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

const maxDiagnostics = 32

// Diagnostic identifies a rejected entry or empty document without retaining
// any subscription text.
// Format, Protocol, Reason and Field are parser-owned, finite vocabularies.
// Entry is the one-based YAML element / whitespace-delimited URL ordinal;
// zero identifies a document-level diagnostic when there are no entries.
type Diagnostic struct {
	Format   string `json:"format"`
	Entry    int    `json:"entry"`
	Protocol string `json:"protocol,omitempty"`
	Reason   string `json:"reason"`
	Field    string `json:"field"`
}

type ReasonCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// issue never contains upstream errors or user-supplied values.
type issue struct{ reason, field string }

func (e *issue) Error() string { return e.reason + ": " + e.field }

func parseIssue(reason, field string) error { return &issue{reason: reason, field: field} }

func (r *ExtractResult) reject(format string, entry int, protocol string, err error) {
	r.Rejected++
	reason, field := "invalid_entry", "entry"
	var problem *issue
	if errors.As(err, &problem) {
		reason, field = problem.reason, problem.field
	}
	counted := false
	for i := range r.ReasonCounts {
		if r.ReasonCounts[i].Reason == reason {
			r.ReasonCounts[i].Count++
			counted = true
			break
		}
	}
	if !counted && len(r.ReasonCounts) < maxDiagnostics {
		r.ReasonCounts = append(r.ReasonCounts, ReasonCount{Reason: reason, Count: 1})
		sort.Slice(r.ReasonCounts, func(i, j int) bool { return r.ReasonCounts[i].Reason < r.ReasonCounts[j].Reason })
	}
	if len(r.Diagnostics) < maxDiagnostics {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Format: format, Entry: entry, Protocol: protocol, Reason: reason, Field: field})
	}
}

// emptySubscriptionResult describes an empty document without inventing a
// rejected node. All values are parser-owned; no subscription text is retained.
func emptySubscriptionResult(format, field string) ExtractResult {
	return ExtractResult{
		Diagnostics:  []Diagnostic{{Format: format, Entry: 0, Reason: "empty_subscription", Field: field}},
		ReasonCounts: []ReasonCount{{Reason: "empty_subscription", Count: 1}},
	}
}

func knownProtocol(raw string) string {
	protocol, err := protocolFor(raw)
	if err != nil {
		return ""
	}
	return string(protocol)
}

// DiagnosticSummary is deterministic and bounded regardless of input size. It
// deliberately serializes only diagnostics produced internally, never raw errors.
func (r ExtractResult) DiagnosticSummary() string {
	if r.Rejected == 0 && len(r.Diagnostics) == 0 {
		return ""
	}
	payload := struct {
		Rejected     int           `json:"rejected"`
		ReasonCounts []ReasonCount `json:"reason_counts"`
		Entries      []Diagnostic  `json:"entries"`
	}{r.Rejected, r.ReasonCounts, r.Diagnostics}
	encoded, _ := json.Marshal(payload)
	return fmt.Sprintf("[stage:parse] %s", encoded)
}

func (r *ExtractResult) wrapped() {
	for i := range r.Diagnostics {
		r.Diagnostics[i].Format = "base64_" + r.Diagnostics[i].Format
	}
}
