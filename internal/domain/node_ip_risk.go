package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ParseIPRiskScoreInt accepts only canonical integers in 0..100, including zero.
func ParseIPRiskScoreInt(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 3 || (len(raw) > 1 && raw[0] == '0') {
		return 0, false
	}
	for i := range raw {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n > 100 {
		return 0, false
	}
	return n, true
}

// ParseIPRiskScore mirrors the probe provider's strict score grammar without
// importing the probe layer: an integer, optionally followed by '%' and a
// whitespace-separated label. Keep the SQL predicate grammar in sync.
func ParseIPRiskScore(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if strings.Count(raw, "%") > 1 {
		return 0, false
	}
	if i := strings.IndexByte(raw, '%'); i >= 0 {
		rest := raw[i+1:]
		if rest != "" && !unicode.IsSpace(rune(rest[0])) {
			return 0, false
		}
		raw = raw[:i]
	}
	return ParseIPRiskScoreInt(raw)
}

// riskScoreFromEvidence decodes only the score's persistence contract. Invalid
// JSON or a non-string score yields unknown; unrelated evidence types do not
// invalidate a valid score. Shared by filtering and repository hydration.
func riskScoreFromEvidence(raw string) string {
	var evidence struct {
		RiskScore string `json:"risk_score"`
	}
	if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
		return ""
	}
	return evidence.RiskScore
}

func IsValidIPRiskBand(band string) bool {
	return band == "low" || band == "medium" || band == "high" || band == "unknown"
}

// CurrentIPRiskScore uses the latest observation only; an unsuccessful check
// must not revive an older successful score. No age TTL is imposed.
func CurrentIPRiskScore(node Node, latest map[ProbeKind]ProbeObservation, asOf time.Time) (int, bool) {
	obs, ok := latest[ProbeKindIPRisk]
	if !ok || obs.Kind != ProbeKindIPRisk || obs.Verdict != VerdictAvailable ||
		node.ConnectionRevision < 1 || obs.ConnectionRevision == nil ||
		*obs.ConnectionRevision != node.ConnectionRevision || obs.ObservedAt.IsZero() || obs.ObservedAt.After(asOf) {
		return 0, false
	}
	score := obs.RiskScore
	if obs.EvidenceData != "" {
		score = riskScoreFromEvidence(obs.EvidenceData)
	}
	return ParseIPRiskScore(score)
}

func IPRiskScoreBand(score int, known bool) string {
	if !known {
		return "unknown"
	}
	if score < 25 {
		return "low"
	}
	if score < 65 {
		return "medium"
	}
	return "high"
}
