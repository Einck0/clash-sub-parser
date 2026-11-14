package domain_test

import (
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/platform"
)

func TestIPRiskScoreParserMatchesProvider(t *testing.T) {
	for _, raw := range []string{"0", "24", "25", "64", "65", "100", "15% low", "15%", "15 % low", "\t15%\tlow\n", "\u200315\u00a0", "", "garbage", "00", "01", "-1", "+1", "1.0", "101", "15junk", "15%low", "15%% low", "15% low%", "15%\u00a0low", "15%\u0085low", "0\x00garbage", "15% label\x00tail", "15% label\x00%", "15%\x00"} {
		t.Run(raw, func(t *testing.T) {
			got, ok := domain.ParseIPRiskScore(raw)
			want, valid := platform.ParseRiskScore(raw)
			if got != want || ok != valid {
				t.Fatalf("domain = (%d,%v), provider = (%d,%v)", got, ok, want, valid)
			}
		})
	}
}

func TestIPRiskStructuredFilterEvidenceAndBands(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	rev := int64(2)
	node := domain.Node{ConnectionRevision: rev}
	base := domain.ProbeObservation{Kind: domain.ProbeKindIPRisk, Verdict: domain.VerdictAvailable, ConnectionRevision: &rev, ObservedAt: now.Add(-365 * 24 * time.Hour)}
	for _, tc := range []struct{ score, band string }{{"0", "low"}, {"24", "low"}, {"25", "medium"}, {"64", "medium"}, {"65", "high"}, {"100", "high"}, {"bad", "unknown"}} {
		t.Run(tc.score, func(t *testing.T) {
			obs := base
			obs.RiskScore = tc.score
			latest := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindIPRisk: obs}
			c := domain.FilterCondition{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: tc.band}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
			if ok, reason := domain.MatchesFilter(&domain.NodeFilterSpec{Conditions: []domain.FilterCondition{c}}, node, nil, latest, now); !ok {
				t.Fatal(reason)
			}
		})
	}
	old := int64(1)
	for _, scenario := range []string{"missing", "error", "restricted", "old_revision", "nil_revision", "future", "zero_time", "malformed", "wrong_kind"} {
		t.Run(scenario, func(t *testing.T) {
			obs := base
			obs.RiskScore = "0"
			switch scenario {
			case "error":
				obs.Verdict = domain.VerdictError
			case "restricted":
				obs.Verdict = domain.VerdictRestricted
			case "old_revision":
				obs.ConnectionRevision = &old
			case "nil_revision":
				obs.ConnectionRevision = nil
			case "future":
				obs.ObservedAt = now.Add(time.Hour)
			case "zero_time":
				obs.ObservedAt = time.Time{}
			case "malformed":
				obs.RiskScore = "bad"
			case "wrong_kind":
				obs.Kind = domain.ProbeKindGeo
			}
			latest := map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindIPRisk: obs}
			if scenario == "missing" {
				latest = nil
			}
			for _, c := range []domain.FilterCondition{
				{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: "unknown"},
				{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpNotEquals, Value: "low"},
			} {
				if ok, reason := domain.MatchesCondition(c, node, nil, latest, now); !ok {
					t.Fatal(reason)
				}
			}
			if ok, _ := domain.MatchesCondition(domain.FilterCondition{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpLTE, Value: "100"}, node, nil, latest, now); ok {
				t.Fatal("unknown score matched threshold")
			}
		})
	}
	base.RiskScore = "0"
	if ok, reason := domain.MatchesCondition(domain.FilterCondition{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpLTE, Value: "0"}, node, nil, map[domain.ProbeKind]domain.ProbeObservation{domain.ProbeKindIPRisk: base}, now); !ok {
		t.Fatal(reason)
	}
}

func TestIPRiskFilterValidation(t *testing.T) {
	for _, c := range []domain.FilterCondition{
		{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpEquals, Value: "10"},
		{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpLTE, Value: "101"},
		{Field: domain.FilterFieldIPRiskScore, Op: domain.FilterOpLTE, Value: "15% low"},
		{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpContains, Value: "low"},
		{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: "critical"},
		{Field: domain.FilterFieldIPRiskBand, Op: domain.FilterOpEquals, Value: "low", Platform: "ip_risk"},
	} {
		if err := c.Validate(); err == nil {
			t.Errorf("accepted invalid condition: %+v", c)
		}
	}
}
