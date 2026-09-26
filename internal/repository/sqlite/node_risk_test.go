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
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES (?, ?, ?, '203.0.113.10', 8388, ?, ?, ?, ?)
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
		configJSON := fmt.Sprintf(`{"method":"aes-256-gcm","password":"pw-%05d"}`, i)
		active := 1
		if i > 8000 {
			active = 0
		}
		timestamp := now.Add(time.Duration(i) * time.Millisecond).Format(time.RFC3339Nano)

		if _, err := nodeStmt.ExecContext(ctx, logicalID, string(proto), displayName, configJSON, active, timestamp, timestamp); err != nil {
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

	t.Run("large_page_over_10_items_retains_exact_observations", func(t *testing.T) {
		items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
			SortBy:     "display_name",
			SortOrder:  "ASC",
		})
		if err != nil {
			t.Fatalf("ListReadModel failed: %v", err)
		}
		if total != 10000 || len(items) != 50 {
			t.Fatalf("expected total 10000 and 50 items, got total=%d len=%d", total, len(items))
		}
		for i, item := range items {
			if item.IPRiskSummary == nil {
				t.Fatalf("item %d (%s) missing IPRiskSummary", i, item.Node.LogicalID)
			}
			if item.IPRiskSummary.Decision != domain.RiskActionAllow || item.IPRiskSummary.RiskBand != domain.RiskBandLow || item.IPRiskSummary.Status != domain.IPRiskStatusAvailable {
				t.Fatalf("item %d (%s) expected allow/low/available on page >10, got %+v", i, item.Node.LogicalID, item.IPRiskSummary)
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

func TestNodeRiskRepository_MultiProviderAndHistoryPagination(t *testing.T) {
	db, _ := setupTestDB(t)
	ctx := context.Background()

	// 1. Verify composite index from migration 000010 exists
	var indexCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'index'
		  AND tbl_name = 'ip_risk_observations'
		  AND name = 'idx_ip_risk_obs_node_observed_id';
	`).Scan(&indexCount); err != nil {
		t.Fatalf("failed to query sqlite_master for idx_ip_risk_obs_node_observed_id: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("expected idx_ip_risk_obs_node_observed_id index to exist, got count=%d", indexCount)
	}

	// 2. Seed providers (2 allowed in policy, 1 disallowed)
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	for _, prov := range []string{"scamalytics", "ipinfo", "spur"} {
		if err := providerRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
			Provider:           prov,
			SchemaVersion:      "v1",
			Enabled:            true,
			SecretReference:    "secret://iprisk/" + prov,
			MaxConcurrency:     10,
			RequestsPerMinute:  60,
			DailyRequestBudget: 10000,
			PerRequestTimeout:  2 * time.Second,
			MaxResponseBytes:   65536,
		}); err != nil {
			t.Fatalf("failed to upsert provider %s: %v", prov, err)
		}
	}

	// 3. Create active multi-provider risk policy (scamalytics + ipinfo)
	policyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	policyRevID := domain.MustNewUUIDv7()
	riskPolicy := domain.RiskPolicy{
		RevisionID: policyRevID,
		ProviderSelection: domain.RiskProviderSelection{
			Mode: domain.RiskFusionHighestRisk,
			Providers: []domain.ProviderRef{
				{Provider: "scamalytics", SchemaVersion: "v1"},
				{Provider: "ipinfo", SchemaVersion: "v1"},
			},
		},
		MaxObservationAge: 24 * time.Hour,
		MinimumConfidence: 50,
		ScoreBands: []domain.ScoreBand{
			{Min: 0, Max: 49, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
			{Min: 50, Max: 79, Band: domain.RiskBandHigh, Action: domain.RiskActionReview},
			{Min: 80, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
		},
		UnknownAction: domain.RiskActionReview,
	}
	if err := policyRepo.Create(ctx, &domain.RiskPolicyRevision{
		RiskPolicy: riskPolicy,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("failed to create multi-provider policy: %v", err)
	}
	if err := policyRepo.SetActive(ctx, policyRevID, true); err != nil {
		t.Fatalf("failed to activate multi-provider policy: %v", err)
	}

	// 4. Seed 30 nodes with multi-provider and deep historical observations
	now := time.Now().UTC().Truncate(time.Second)
	latestTime := now.Add(-5 * time.Minute)
	futureExp := now.Add(24 * time.Hour).Format(time.RFC3339Nano)
	exitDigest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	evidenceDigest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	for i := 1; i <= 30; i++ {
		logicalID := fmt.Sprintf("node-mp-%03d", i)
		displayName := fmt.Sprintf("MPNode-%03d", i)
		createdAt := now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
			VALUES (?, 'vless', ?, '203.0.113.20', 443, '{}', 1, ?, ?);
		`, logicalID, displayName, createdAt, createdAt); err != nil {
			t.Fatalf("insert node %s: %v", logicalID, err)
		}

		insertObs := func(obsID, provider string, obsAt time.Time, score int) {
			if _, err := db.ExecContext(ctx, `
				INSERT INTO ip_risk_observations (
					id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
					observed_at, expires_at, status, score, confidence, network_class,
					anonymizer_traits, evidence_digest, redacted_summary
				) VALUES (?, ?, ?, ?, 'v1', ?, ?, 'available', ?, 90, 'datacenter', '[]', ?, 'safe summary');
			`, obsID, logicalID, exitDigest, provider, obsAt.Format(time.RFC3339Nano), futureExp, score, evidenceDigest); err != nil {
				t.Fatalf("insert obs %s for %s: %v", obsID, logicalID, err)
			}
		}

		if i <= 24 {
			// Insert 12 older historical observations across both allowed providers
			for h := 1; h <= 12; h++ {
				histTime := latestTime.Add(-time.Duration(h) * time.Hour)
				prov := "scamalytics"
				score := 90 // opposite of allow
				if h%2 == 0 {
					prov = "ipinfo"
					score = 10 // opposite of block
				}
				insertObs(fmt.Sprintf("obs-hist-%03d-%02d", i, h), prov, histTime, score)
			}
		}

		switch {
		case i <= 8:
			// Same timestamp tie-break: ipinfo has higher id ('z' > 'a') -> allow / low
			insertObs(fmt.Sprintf("obs-mp-%03d-tie-a", i), "scamalytics", latestTime, 95)
			insertObs(fmt.Sprintf("obs-mp-%03d-tie-z", i), "ipinfo", latestTime, 15)
			// Disallowed provider with newer timestamp must be ignored
			insertObs(fmt.Sprintf("obs-mp-%03d-spur", i), "spur", latestTime.Add(time.Minute), 99)
		case i <= 16:
			// Same timestamp tie-break: scamalytics has higher id ('z' > 'a') -> block / critical
			insertObs(fmt.Sprintf("obs-mp-%03d-tie-a", i), "ipinfo", latestTime, 15)
			insertObs(fmt.Sprintf("obs-mp-%03d-tie-z", i), "scamalytics", latestTime, 92)
		case i <= 24:
			// Different timestamps across providers: ipinfo is newer -> review / high
			insertObs(fmt.Sprintf("obs-mp-%03d-older", i), "scamalytics", latestTime.Add(-time.Minute), 10)
			insertObs(fmt.Sprintf("obs-mp-%03d-newer", i), "ipinfo", latestTime, 65)
		case i <= 27:
			// Only disallowed provider observations -> treated as missing_observation (review / unknown)
			insertObs(fmt.Sprintf("obs-mp-%03d-spur-only", i), "spur", latestTime, 95)
		default:
			// 28..30: no observations at all -> missing_observation (review / unknown)
		}
	}

	// 5. Verify EXPLAIN QUERY PLAN uses idx_ip_risk_obs_node_observed_id for bounded window query
	t.Run("explain_query_plan_uses_composite_index", func(t *testing.T) {
		rows, err := db.QueryContext(ctx, `
			EXPLAIN QUERY PLAN
			WITH ranked_obs AS (
				SELECT o.id,
				       o.node_logical_id,
				       ROW_NUMBER() OVER (
				           PARTITION BY o.node_logical_id
				           ORDER BY o.observed_at DESC, o.id DESC
				       ) AS rn
				FROM ip_risk_observations o
				WHERE o.node_logical_id IN ('node-mp-001', 'node-mp-002', 'node-mp-003')
				  AND EXISTS (
				      SELECT 1
				      FROM risk_policy_providers rpp
				      WHERE rpp.revision_id = ?
				        AND rpp.provider = o.provider
				        AND rpp.schema_version = o.provider_schema_version
				  )
			)
			SELECT id, node_logical_id FROM ranked_obs WHERE rn = 1;
		`, policyRevID)
		if err != nil {
			t.Fatalf("EXPLAIN QUERY PLAN failed: %v", err)
		}
		defer rows.Close()

		var details []string
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatalf("scan explain row: %v", err)
			}
			details = append(details, detail)
		}
		planText := strings.Join(details, " | ")
		if !strings.Contains(planText, "idx_ip_risk_obs_node_observed_id") {
			t.Fatalf("expected EXPLAIN QUERY PLAN to use idx_ip_risk_obs_node_observed_id, got: %s", planText)
		}
		if strings.Contains(planText, "USE TEMP B-TREE") {
			t.Fatalf("expected EXPLAIN QUERY PLAN to avoid temp B-tree sort, got: %s", planText)
		}

		// Also verify full-CTE window query plan uses idx_ip_risk_obs_node_observed_id
		cteRows, err := db.QueryContext(ctx, `
			EXPLAIN QUERY PLAN
			WITH ranked_obs AS (
				SELECT o.id AS obs_id,
				       o.node_logical_id,
				       ROW_NUMBER() OVER (
				           PARTITION BY o.node_logical_id
				           ORDER BY o.observed_at DESC, o.id DESC
				       ) AS rn
				FROM ip_risk_observations o
				WHERE EXISTS (
					SELECT 1
					FROM risk_policy_providers rpp
					WHERE rpp.revision_id = ?
					  AND rpp.provider = o.provider
					  AND rpp.schema_version = o.provider_schema_version
				)
			)
			SELECT obs_id, node_logical_id FROM ranked_obs WHERE rn = 1;
		`, policyRevID)
		if err != nil {
			t.Fatalf("CTE EXPLAIN QUERY PLAN failed: %v", err)
		}
		defer cteRows.Close()

		var cteDetails []string
		for cteRows.Next() {
			var id, parent, notused int
			var detail string
			if err := cteRows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatalf("scan cte explain row: %v", err)
			}
			cteDetails = append(cteDetails, detail)
		}
		ctePlanText := strings.Join(cteDetails, " | ")
		if !strings.Contains(ctePlanText, "idx_ip_risk_obs_node_observed_id") {
			t.Fatalf("expected CTE EXPLAIN QUERY PLAN to use idx_ip_risk_obs_node_observed_id, got: %s", ctePlanText)
		}
		if strings.Contains(ctePlanText, "USE TEMP B-TREE") {
			t.Fatalf("expected CTE EXPLAIN QUERY PLAN to avoid temp B-tree sort, got: %s", ctePlanText)
		}
	})

	repo := sqlite.NewNodeRepository(db)

	assertExpectedNodeSummary := func(t *testing.T, item domain.NodeReadModel) {
		t.Helper()
		if item.IPRiskSummary == nil {
			t.Fatalf("node %s has nil IPRiskSummary", item.Node.LogicalID)
		}
		var idx int
		if _, err := fmt.Sscanf(item.Node.LogicalID, "node-mp-%03d", &idx); err != nil {
			t.Fatalf("unexpected logical_id %q: %v", item.Node.LogicalID, err)
		}

		switch {
		case idx >= 1 && idx <= 8:
			if item.IPRiskSummary.Decision != domain.RiskActionAllow ||
				item.IPRiskSummary.RiskBand != domain.RiskBandLow ||
				item.IPRiskSummary.Provider != "ipinfo" ||
				item.IPRiskSummary.Status != domain.IPRiskStatusAvailable {
				t.Fatalf("node %s (1..8) expected allow/low/ipinfo/available, got %+v", item.Node.LogicalID, item.IPRiskSummary)
			}
		case idx >= 9 && idx <= 16:
			if item.IPRiskSummary.Decision != domain.RiskActionBlock ||
				item.IPRiskSummary.RiskBand != domain.RiskBandCritical ||
				item.IPRiskSummary.Provider != "scamalytics" ||
				item.IPRiskSummary.Status != domain.IPRiskStatusAvailable {
				t.Fatalf("node %s (9..16) expected block/critical/scamalytics/available, got %+v", item.Node.LogicalID, item.IPRiskSummary)
			}
		case idx >= 17 && idx <= 24:
			if item.IPRiskSummary.Decision != domain.RiskActionReview ||
				item.IPRiskSummary.RiskBand != domain.RiskBandHigh ||
				item.IPRiskSummary.Provider != "ipinfo" ||
				item.IPRiskSummary.Status != domain.IPRiskStatusAvailable {
				t.Fatalf("node %s (17..24) expected review/high/ipinfo/available, got %+v", item.Node.LogicalID, item.IPRiskSummary)
			}
		default:
			if item.IPRiskSummary.Decision != domain.RiskActionReview ||
				item.IPRiskSummary.RiskBand != domain.RiskBandUnknown ||
				item.IPRiskSummary.Provider != "scamalytics" ||
				item.IPRiskSummary.Status != domain.IPRiskStatusUnknown ||
				item.IPRiskSummary.ReasonCode != "missing_observation" {
				t.Fatalf("node %s (25..30) expected review/unknown/scamalytics/unknown/missing_observation, got %+v", item.Node.LogicalID, item.IPRiskSummary)
			}
		}

		// Verify GetReadModel returns identical risk summary
		single, err := repo.GetReadModel(ctx, item.Node.LogicalID, policyRevID)
		if err != nil {
			t.Fatalf("GetReadModel(%s) failed: %v", item.Node.LogicalID, err)
		}
		if single.IPRiskSummary == nil ||
			single.IPRiskSummary.Decision != item.IPRiskSummary.Decision ||
			single.IPRiskSummary.RiskBand != item.IPRiskSummary.RiskBand ||
			single.IPRiskSummary.Provider != item.IPRiskSummary.Provider ||
			single.IPRiskSummary.Status != item.IPRiskSummary.Status {
			t.Fatalf("GetReadModel(%s) summary %+v != ListReadModel summary %+v", item.Node.LogicalID, single.IPRiskSummary, item.IPRiskSummary)
		}
	}

	// 6. Unfiltered pagination (>10 nodes per page) uses loadObservationsForNodes
	t.Run("unfiltered_pagination_over_10_nodes_no_duplicates_exact_match", func(t *testing.T) {
		seen := make(map[string]bool, 30)
		expectedPageLengths := []int{12, 12, 6}
		for pageIdx, wantLen := range expectedPageLengths {
			items, total, err := repo.ListReadModel(ctx, domain.NodeFilter{
				Pagination: domain.Pagination{Page: pageIdx + 1, PageSize: 12},
				SortBy:     "display_name",
				SortOrder:  "ASC",
			})
			if err != nil {
				t.Fatalf("page %d failed: %v", pageIdx+1, err)
			}
			if total != 30 {
				t.Fatalf("page %d: expected total 30 (not multiplied by providers/history), got %d", pageIdx+1, total)
			}
			if len(items) != wantLen {
				t.Fatalf("page %d: expected %d items, got %d", pageIdx+1, wantLen, len(items))
			}
			for _, item := range items {
				if seen[item.Node.LogicalID] {
					t.Fatalf("duplicate node %s across/within pages", item.Node.LogicalID)
				}
				seen[item.Node.LogicalID] = true
				assertExpectedNodeSummary(t, item)
			}
		}
		if len(seen) != 30 {
			t.Fatalf("expected 30 unique nodes across pages, got %d", len(seen))
		}
	})

	// 7. Risk-filtered pagination (>10 nodes) uses buildRiskEvaluationCTE window function
	t.Run("filtered_by_decision_and_provider_no_total_inflation_or_duplicates", func(t *testing.T) {
		// Allow: exactly 8 nodes (1..8)
		allowItems, allowTotal, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 15},
			RiskDecisions: []domain.RiskAction{domain.RiskActionAllow},
			SortBy:        "display_name",
			SortOrder:     "ASC",
		})
		if err != nil {
			t.Fatalf("filter allow failed: %v", err)
		}
		if allowTotal != 8 || len(allowItems) != 8 {
			t.Fatalf("expected 8 allow nodes, got total=%d len=%d", allowTotal, len(allowItems))
		}
		for _, item := range allowItems {
			assertExpectedNodeSummary(t, item)
		}

		// Block: exactly 8 nodes (9..16); nodes 1..8 had scamalytics block in tie-break loser and history, must NOT be included
		blockItems, blockTotal, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 15},
			RiskDecisions: []domain.RiskAction{domain.RiskActionBlock},
			SortBy:        "display_name",
			SortOrder:     "ASC",
		})
		if err != nil {
			t.Fatalf("filter block failed: %v", err)
		}
		if blockTotal != 8 || len(blockItems) != 8 {
			t.Fatalf("expected 8 block nodes (no duplicate provider rows), got total=%d len=%d", blockTotal, len(blockItems))
		}
		for _, item := range blockItems {
			assertExpectedNodeSummary(t, item)
		}

		// Review: 8 nodes (17..24) + 6 nodes (25..30) = 14 nodes (>10 nodes pagination)
		revPage1, revTotal1, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 11},
			RiskDecisions: []domain.RiskAction{domain.RiskActionReview},
			SortBy:        "display_name",
			SortOrder:     "ASC",
		})
		if err != nil {
			t.Fatalf("filter review page 1 failed: %v", err)
		}
		revPage2, revTotal2, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 2, PageSize: 11},
			RiskDecisions: []domain.RiskAction{domain.RiskActionReview},
			SortBy:        "display_name",
			SortOrder:     "ASC",
		})
		if err != nil {
			t.Fatalf("filter review page 2 failed: %v", err)
		}
		if revTotal1 != 14 || revTotal2 != 14 || len(revPage1) != 11 || len(revPage2) != 3 {
			t.Fatalf("unexpected review pagination: total1=%d total2=%d len1=%d len2=%d", revTotal1, revTotal2, len(revPage1), len(revPage2))
		}
		seenRev := make(map[string]bool, 14)
		for _, item := range append(revPage1, revPage2...) {
			if seenRev[item.Node.LogicalID] {
				t.Fatalf("duplicate node %s in review pagination", item.Node.LogicalID)
			}
			seenRev[item.Node.LogicalID] = true
			assertExpectedNodeSummary(t, item)
		}

		// Provider filter "ipinfo": 8 nodes (1..8) + 8 nodes (17..24) = 16 nodes (>10 nodes)
		ipinfoPage1, ipinfoTotal, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 12},
			RiskProviders: []string{"ipinfo"},
			SortBy:        "display_name",
			SortOrder:     "ASC",
		})
		if err != nil {
			t.Fatalf("filter provider ipinfo page 1 failed: %v", err)
		}
		ipinfoPage2, _, err := repo.ListReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 2, PageSize: 12},
			RiskProviders: []string{"ipinfo"},
			SortBy:        "display_name",
			SortOrder:     "ASC",
		})
		if err != nil {
			t.Fatalf("filter provider ipinfo page 2 failed: %v", err)
		}
		if ipinfoTotal != 16 || len(ipinfoPage1) != 12 || len(ipinfoPage2) != 4 {
			t.Fatalf("expected 16 ipinfo nodes (12+4), got total=%d len1=%d len2=%d", ipinfoTotal, len(ipinfoPage1), len(ipinfoPage2))
		}
		for _, item := range append(ipinfoPage1, ipinfoPage2...) {
			assertExpectedNodeSummary(t, item)
		}
	})
}
