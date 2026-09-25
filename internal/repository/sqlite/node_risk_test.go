package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func seed10000NodesWithRisk(t *testing.T, db *sql.DB) (domain.RiskPolicy, string) {
	t.Helper()
	ctx := context.Background()

	// 1. Parent subscription for foreign key constraints
	_, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES ('sub-risk-001', 'Risk Test Sub', 'secret://test/sub', 1, 'rev-1', '2026-09-16T00:00:00Z', '2026-09-16T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("failed to insert subscription: %v", err)
	}

	// 2. Provider settings
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	err = providerRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
		Provider:           "scamalytics",
		SchemaVersion:      "v1",
		Enabled:            true,
		SecretReference:    "secret://iprisk/scamalytics",
		MaxConcurrency:     10,
		RequestsPerMinute:  60,
		DailyRequestBudget: 10000,
		PerRequestTimeout:  2 * time.Second,
		MaxResponseBytes:   65536,
	})
	if err != nil {
		t.Fatalf("failed to insert provider settings: %v", err)
	}

	// 3. Active risk policy
	policyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	policyRevID := domain.MustNewUUIDv7()
	riskPolicy := domain.RiskPolicy{
		RevisionID: policyRevID,
		ProviderSelection: domain.RiskProviderSelection{
			Mode:      domain.RiskFusionSingleProvider,
			Providers: []domain.ProviderRef{{Provider: "scamalytics", SchemaVersion: "v1"}},
		},
		MaxObservationAge: 24 * time.Hour,
		MinimumConfidence: 50,
		ScoreBands: []domain.ScoreBand{
			{Min: 0, Max: 49, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
			{Min: 50, Max: 79, Band: domain.RiskBandHigh, Action: domain.RiskActionReview},
			{Min: 80, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
		},
		TraitRules: []domain.TraitRule{
			{Trait: domain.TraitTor, Action: domain.RiskActionBlock},
		},
		UnknownAction: domain.RiskActionReview,
	}

	rev := &domain.RiskPolicyRevision{
		RiskPolicy: riskPolicy,
		CreatedAt:  time.Now().UTC(),
	}
	if err := policyRepo.Create(ctx, rev); err != nil {
		t.Fatalf("failed to create risk policy revision: %v", err)
	}
	if err := policyRepo.SetActive(ctx, policyRevID, true); err != nil {
		t.Fatalf("failed to set active risk policy: %v", err)
	}

	// 4. Batch insert 10,000 nodes inside a transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	nodeStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		t.Fatalf("failed to prepare node stmt: %v", err)
	}
	defer nodeStmt.Close()

	obsStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ip_risk_observations (
			id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
			observed_at, expires_at, status, score, confidence, network_class,
			anonymizer_traits, evidence_digest, redacted_summary
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		t.Fatalf("failed to prepare observation stmt: %v", err)
	}
	defer obsStmt.Close()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)
	futureStr := now.Add(24 * time.Hour).Format(time.RFC3339Nano)
	pastObsStr := now.Add(-48 * time.Hour).Format(time.RFC3339Nano)
	pastExpStr := now.Add(-24 * time.Hour).Format(time.RFC3339Nano)

	protocols := []domain.Protocol{
		domain.ProtocolSS,
		domain.ProtocolVMess,
		domain.ProtocolVLESS,
		domain.ProtocolTrojan,
	}

	for i := 1; i <= 10000; i++ {
		logicalID := fmt.Sprintf("node-risk-%05d", i)
		proto := protocols[i%len(protocols)]
		displayName := fmt.Sprintf("RiskNode-%05d", i)
		secretRef := fmt.Sprintf("secret://credentials/node-%05d?token=super-secret-pw-%05d", i, i)
		active := 1
		if i > 8000 {
			active = 0
		}
		timestamp := now.Add(time.Duration(i) * time.Millisecond).Format(time.RFC3339Nano)

		if _, err := nodeStmt.ExecContext(ctx, logicalID, string(proto), displayName, secretRef, active, timestamp, timestamp); err != nil {
			t.Fatalf("insert node %d: %v", i, err)
		}

		// Distribute observations:
		// 1..1000: score 20, confidence 90 -> allow / low
		// 1001..2000: score 65, confidence 80 -> review / high
		// 2001..3000: score 90, confidence 95 -> block / critical
		// 3001..4000: score 10, confidence 90, trait "tor" -> block / critical
		// 4001..5000: score 10, confidence 90, but expired -> review / unknown (stale)
		// 5001..10000: no observations -> review / unknown (missing_observation)
		if i <= 5000 {
			obsID := fmt.Sprintf("obs-%05d", i)
			exitDigest := fmt.Sprintf("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			evidenceDigest := fmt.Sprintf("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
			status := "available"
			score := 10
			confidence := 90
			traitsJSON := "[]"
			obsTime := nowStr
			expTime := futureStr

			switch {
			case i <= 1000:
				score = 20
			case i <= 2000:
				score = 65
				confidence = 80
			case i <= 3000:
				score = 90
				confidence = 95
			case i <= 4000:
				score = 10
				traitsJSON = "[\"tor\"]"
			case i <= 5000:
				score = 10
				obsTime = pastObsStr
				expTime = pastExpStr
			}

			if _, err := obsStmt.ExecContext(ctx, obsID, logicalID, exitDigest, "scamalytics", "v1",
				obsTime, expTime, status, score, confidence, "datacenter", traitsJSON, evidenceDigest, "safe summary"); err != nil {
				t.Fatalf("insert observation %d: %v", i, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit 10,000 nodes: %v", err)
	}

	return riskPolicy, policyRevID
}

func TestNodeRiskRepository_10000NodesReadModelAndFiltering(t *testing.T) {
	db, _ := setupTestDB(t)
	_, policyRevID := seed10000NodesWithRisk(t, db)
	repo := sqlite.NewNodeRepository(db)
	ctx := context.Background()

	t.Run("unfiltered_list_total_and_pagination", func(t *testing.T) {
		start := time.Now()
		items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
		})
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("ListReadModel failed: %v", err)
		}
		if total != 10000 {
			t.Errorf("expected total 10000, got %d", total)
		}
		if len(items) != 50 {
			t.Errorf("expected 50 items, got %d", len(items))
		}
		if elapsed > 200*time.Millisecond {
			t.Errorf("query too slow (%v), expected server-side indexed speed under 200ms", elapsed)
		}

		for _, item := range items {
			if item.IPRiskSummary == nil {
				t.Fatalf("expected IPRiskSummary to be populated, got nil for node %s", item.Node.LogicalID)
			}
			if err := item.IPRiskSummary.Validate(); err != nil {
				t.Errorf("invalid IPRiskSummary: %v", err)
			}
		}
	})

	t.Run("page_size_capped_at_100", func(t *testing.T) {
		items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 200},
		})
		if err != nil {
			t.Fatalf("ListReadModel failed: %v", err)
		}
		if total != 10000 {
			t.Errorf("expected total 10000, got %d", total)
		}
		if len(items) != 100 {
			t.Errorf("expected page_size capped at 100, got %d", len(items))
		}
	})

	t.Run("filter_by_risk_decision_block", func(t *testing.T) {
		// 1000 score 90 + 1000 tor trait = 2000 blocked nodes
		items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			RiskDecisions: []domain.RiskAction{domain.RiskActionBlock},
		})
		if err != nil {
			t.Fatalf("filter by block failed: %v", err)
		}
		if total != 2000 {
			t.Errorf("expected total 2000 blocked nodes, got %d", total)
		}
		if len(items) != 50 {
			t.Errorf("expected 50 items, got %d", len(items))
		}
		for _, item := range items {
			if item.IPRiskSummary == nil || item.IPRiskSummary.Decision != domain.RiskActionBlock {
				t.Errorf("expected block decision, got %+v", item.IPRiskSummary)
			}
		}
	})

	t.Run("filter_by_risk_decision_allow", func(t *testing.T) {
		// 1000 score 20 = 1000 allowed nodes
		items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			RiskDecisions: []domain.RiskAction{domain.RiskActionAllow},
		})
		if err != nil {
			t.Fatalf("filter by allow failed: %v", err)
		}
		if total != 1000 {
			t.Errorf("expected total 1000 allowed nodes, got %d", total)
		}
		for _, item := range items {
			if item.IPRiskSummary == nil || item.IPRiskSummary.Decision != domain.RiskActionAllow {
				t.Errorf("expected allow decision, got %+v", item.IPRiskSummary)
			}
			if item.IPRiskSummary.RiskBand != domain.RiskBandLow {
				t.Errorf("expected low band, got %+v", item.IPRiskSummary)
			}
		}
	})

	t.Run("filter_by_risk_decision_review", func(t *testing.T) {
		// 1000 score 65 + 1000 expired + 5000 no observations = 7000 review nodes
		_, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			RiskDecisions: []domain.RiskAction{domain.RiskActionReview},
		})
		if err != nil {
			t.Fatalf("filter by review failed: %v", err)
		}
		if total != 7000 {
			t.Errorf("expected total 7000 review nodes, got %d", total)
		}
	})

	t.Run("filter_by_risk_band_critical", func(t *testing.T) {
		// 1000 score 90 + 1000 tor trait = 2000 critical nodes
		_, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
			RiskBands:  []domain.RiskBand{domain.RiskBandCritical},
		})
		if err != nil {
			t.Fatalf("filter by critical band failed: %v", err)
		}
		if total != 2000 {
			t.Errorf("expected total 2000 critical nodes, got %d", total)
		}
	})

	t.Run("filter_by_risk_status_stale", func(t *testing.T) {
		// 1000 expired nodes
		items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:   domain.Pagination{Page: 1, PageSize: 50},
			RiskStatuses: []domain.IPRiskStatus{domain.IPRiskStatusStale},
		})
		if err != nil {
			t.Fatalf("filter by stale status failed: %v", err)
		}
		if total != 1000 {
			t.Errorf("expected total 1000 stale nodes, got %d", total)
		}
		for _, item := range items {
			if item.IPRiskSummary == nil || item.IPRiskSummary.Status != domain.IPRiskStatusStale {
				t.Errorf("expected stale status, got %+v", item.IPRiskSummary)
			}
		}
	})

	t.Run("filter_by_risk_status_unknown", func(t *testing.T) {
		// 5000 nodes without observations
		_, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:   domain.Pagination{Page: 1, PageSize: 50},
			RiskStatuses: []domain.IPRiskStatus{domain.IPRiskStatusUnknown},
		})
		if err != nil {
			t.Fatalf("filter by unknown status failed: %v", err)
		}
		if total != 5000 {
			t.Errorf("expected total 5000 unknown nodes, got %d", total)
		}
	})

	t.Run("filter_by_provider", func(t *testing.T) {
		// All 10000 nodes are evaluated against the active policy provider "scamalytics"
		_, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			RiskProviders: []string{"scamalytics"},
		})
		if err != nil {
			t.Fatalf("filter by provider failed: %v", err)
		}
		if total != 10000 {
			t.Errorf("expected total 10000 nodes for scamalytics provider, got %d", total)
		}

		// Non-existent provider
		_, zeroTotal, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			RiskProviders: []string{"nonexistent_provider"},
		})
		if err != nil {
			t.Fatalf("filter by nonexistent provider failed: %v", err)
		}
		if zeroTotal != 0 {
			t.Errorf("expected total 0 nodes for nonexistent provider, got %d", zeroTotal)
		}
	})

	t.Run("combined_risk_and_core_filters", func(t *testing.T) {
		// Filter: active_only=true, protocol=ss, risk_decision=block
		items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			ActiveOnly:    true,
			Protocols:     []domain.Protocol{domain.ProtocolSS},
			RiskDecisions: []domain.RiskAction{domain.RiskActionBlock},
		})
		if err != nil {
			t.Fatalf("combined filter failed: %v", err)
		}
		// Blocked nodes: 2001..4000. All <= 8000 are active.
		// i%4 == 0 (since index 0 in protocols is SS):
		// For i in 2001..4000: how many are multiples of 4? Exactly 2000 / 4 = 500.
		if total != 500 {
			t.Errorf("expected total 500 combined filter nodes, got %d", total)
		}
		for _, item := range items {
			if !item.Node.Active {
				t.Errorf("expected active node, got inactive")
			}
			if item.Node.Protocol != domain.ProtocolSS {
				t.Errorf("expected SS protocol, got %s", item.Node.Protocol)
			}
			if item.IPRiskSummary == nil || item.IPRiskSummary.Decision != domain.RiskActionBlock {
				t.Errorf("expected block decision, got %+v", item.IPRiskSummary)
			}
		}
	})

	t.Run("explicit_policy_revision_filter", func(t *testing.T) {
		_, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:          domain.Pagination{Page: 1, PageSize: 50},
			RiskPolicyRevisions: []string{policyRevID},
			RiskDecisions:       []domain.RiskAction{domain.RiskActionBlock},
		})
		if err != nil {
			t.Fatalf("policy revision filter failed: %v", err)
		}
		if total != 2000 {
			t.Errorf("expected total 2000 nodes for explicit policy, got %d", total)
		}
	})

	t.Run("zero_sensitive_data_or_ip_leakage", func(t *testing.T) {
		items, _, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
		})
		if err != nil {
			t.Fatalf("ListReadModel failed: %v", err)
		}

		for _, item := range items {
			if item.IPRiskSummary == nil {
				continue
			}
			summaryJSON, err := json.Marshal(item.IPRiskSummary)
			if err != nil {
				t.Fatalf("failed to marshal IPRiskSummary: %v", err)
			}
			jsonStr := string(summaryJSON)

			forbiddenSubstrings := []string{
				"198.51.100.",
				"2001:db8:",
				"super-secret-pw",
				"api_key",
				"apikey",
				"raw_payload",
			}
			for _, forbidden := range forbiddenSubstrings {
				if strings.Contains(jsonStr, forbidden) {
					t.Fatalf("IPRiskSummary JSON contains forbidden secret/IP %q: %s", forbidden, jsonStr)
				}
			}
		}
	})
}

func TestNodeRiskRepository_GetReadModel(t *testing.T) {
	db, _ := setupTestDB(t)
	_, policyRevID := seed10000NodesWithRisk(t, db)
	repo := sqlite.NewNodeRepository(db)
	ctx := context.Background()

	t.Run("existing_node_with_risk", func(t *testing.T) {
		model, err := repo.GetReadModel(ctx, "node-risk-00001", policyRevID)
		if err != nil {
			t.Fatalf("GetReadModel failed: %v", err)
		}
		if model.Node.LogicalID != "node-risk-00001" {
			t.Errorf("expected logical_id node-risk-00001, got %s", model.Node.LogicalID)
		}
		if model.IPRiskSummary == nil {
			t.Fatalf("expected IPRiskSummary, got nil")
		}
		if model.IPRiskSummary.Decision != domain.RiskActionAllow {
			t.Errorf("expected allow decision, got %s", model.IPRiskSummary.Decision)
		}
		if model.IPRiskSummary.RiskBand != domain.RiskBandLow {
			t.Errorf("expected low band, got %s", model.IPRiskSummary.RiskBand)
		}
	})

	t.Run("non_existent_node_returns_not_found", func(t *testing.T) {
		_, err := repo.GetReadModel(ctx, "node-risk-nonexistent", policyRevID)
		if err == nil {
			t.Fatalf("expected error for non-existent node, got nil")
		}
		de, ok := domain.AsDomainError(err)
		if !ok || de.Category != domain.CategoryNotFound {
			t.Errorf("expected not found error, got %v", err)
		}
	})
}
