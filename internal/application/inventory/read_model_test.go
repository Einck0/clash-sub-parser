package inventory_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestInventoryService_ReadModelWithRisk(t *testing.T) {
	db, subRepo, fetchRepo, nodeRepo, sourceRepo := setupTestEnv(t)
	ctx := context.Background()
	svc := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, nil)

	// 1. Create a parent subscription and node
	sub := &domain.Subscription{
		ID:                 "sub-readmodel-01",
		Name:               "ReadModel Test Sub",
		SourceURLSecretRef: "secret://sub/test",
		Enabled:            true,
		RefreshPolicy: domain.RefreshPolicy{
			IntervalSeconds:  86400,
			TimeoutSeconds:   30,
			MaxResponseBytes: 10485760,
		},
		Revision:  "rev-1",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create sub: %v", err)
	}

	nodeID1 := "node_0000000000000001"
	nodeID2 := "node_0000000000000002"

	nodes := []domain.Node{
		{
			LogicalID:                 nodeID1,
			Protocol:                  domain.ProtocolSS,
			DisplayName:               "Node-Allow",
			NormalizedConfigSecretRef: "secret://creds/node-1?token=pw1",
			Active:                    true,
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
		},
		{
			LogicalID:                 nodeID2,
			Protocol:                  domain.ProtocolVMess,
			DisplayName:               "Node-Block",
			NormalizedConfigSecretRef: "secret://creds/node-2?token=pw2",
			Active:                    true,
			CreatedAt:                 time.Now().UTC(),
			UpdatedAt:                 time.Now().UTC(),
		},
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("upsert batch: %v", err)
	}

	// 2. Setup provider settings
	providerRepo := sqlite.NewIPRiskProviderSettingsRepository(db)
	if err := providerRepo.Upsert(ctx, &domain.IPRiskProviderSettings{
		Provider:           "scamalytics",
		SchemaVersion:      "v1",
		Enabled:            true,
		SecretReference:    "secret://iprisk/key",
		MaxConcurrency:     5,
		RequestsPerMinute:  60,
		DailyRequestBudget: 1000,
		PerRequestTimeout:  time.Second,
		MaxResponseBytes:   1024,
	}); err != nil {
		t.Fatalf("upsert provider: %v", err)
	}

	// 3. Setup risk policy
	policyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	policyRevID := domain.MustNewUUIDv7()
	if err := policyRepo.Create(ctx, &domain.RiskPolicyRevision{
		RiskPolicy: domain.RiskPolicy{
			RevisionID: policyRevID,
			ProviderSelection: domain.RiskProviderSelection{
				Mode:      domain.RiskFusionSingleProvider,
				Providers: []domain.ProviderRef{{Provider: "scamalytics", SchemaVersion: "v1"}},
			},
			MaxObservationAge: 24 * time.Hour,
			MinimumConfidence: 50,
			ScoreBands: []domain.ScoreBand{
				{Min: 0, Max: 49, Band: domain.RiskBandLow, Action: domain.RiskActionAllow},
				{Min: 50, Max: 100, Band: domain.RiskBandCritical, Action: domain.RiskActionBlock},
			},
			UnknownAction: domain.RiskActionReview,
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if err := policyRepo.SetActive(ctx, policyRevID, true); err != nil {
		t.Fatalf("set active policy: %v", err)
	}

	// 4. Create observations for nodeID1 (score 10 -> allow) and nodeID2 (score 90 -> block)
	obsRepo := sqlite.NewIPRiskObservationRepository(db)
	score1, score2 := 10, 90
	conf := 90
	now := time.Now().UTC()

	if err := obsRepo.Create(ctx, &domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         nodeID1,
		ExitIdentityDigest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score1,
		Confidence:            &conf,
		NetworkClass:          domain.NetworkClassResidential,
		AnonymizerTraits:      []domain.AnonymizerTrait{},
		EvidenceDigest:        "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		RedactedSummary:       "safe residential score 10",
	}); err != nil {
		t.Fatalf("create obs 1: %v", err)
	}

	if err := obsRepo.Create(ctx, &domain.IPRiskObservation{
		ID:                    domain.MustNewUUIDv7(),
		NodeLogicalID:         nodeID2,
		ExitIdentityDigest:    "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Provider:              "scamalytics",
		ProviderSchemaVersion: "v1",
		ObservedAt:            now,
		ExpiresAt:             now.Add(24 * time.Hour),
		Status:                domain.IPRiskStatusAvailable,
		Score:                 &score2,
		Confidence:            &conf,
		NetworkClass:          domain.NetworkClassDatacenter,
		AnonymizerTraits:      []domain.AnonymizerTrait{domain.TraitTor},
		EvidenceDigest:        "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		RedactedSummary:       "safe datacenter tor score 90",
	}); err != nil {
		t.Fatalf("create obs 2: %v", err)
	}

	t.Run("list_nodes_read_model_without_filters", func(t *testing.T) {
		views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
		})
		if err != nil {
			t.Fatalf("ListNodesReadModel failed: %v", err)
		}
		if total != 2 || len(views) != 2 {
			t.Fatalf("expected 2 views, got %d, total %d", len(views), total)
		}

		for _, v := range views {
			if v.IPRiskSummary == nil {
				t.Fatalf("expected IPRiskSummary for node %s, got nil", v.LogicalID)
			}
			if err := v.IPRiskSummary.Validate(); err != nil {
				t.Errorf("summary validation failed for node %s: %v", v.LogicalID, err)
			}
		}
	})

	t.Run("filter_by_decision_block", func(t *testing.T) {
		views, total, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
			Pagination:    domain.Pagination{Page: 1, PageSize: 50},
			RiskDecisions: []domain.RiskAction{domain.RiskActionBlock},
		})
		if err != nil {
			t.Fatalf("ListNodesReadModel failed: %v", err)
		}
		if total != 1 || len(views) != 1 {
			t.Fatalf("expected 1 view, got %d, total %d", len(views), total)
		}
		if views[0].LogicalID != nodeID2 {
			t.Errorf("expected %s, got %s", nodeID2, views[0].LogicalID)
		}
		if views[0].IPRiskSummary.Decision != domain.RiskActionBlock {
			t.Errorf("expected block decision, got %s", views[0].IPRiskSummary.Decision)
		}
	})

	t.Run("get_node_detail_with_risk", func(t *testing.T) {
		detail, err := svc.GetNodeDetailWithRisk(ctx, nodeID1, "")
		if err != nil {
			t.Fatalf("GetNodeDetailWithRisk failed: %v", err)
		}
		if detail.Node.LogicalID != nodeID1 {
			t.Errorf("expected logical_id %s, got %s", nodeID1, detail.Node.LogicalID)
		}
		if detail.IPRiskSummary == nil {
			t.Fatalf("expected IPRiskSummary, got nil")
		}
		if detail.IPRiskSummary.Decision != domain.RiskActionAllow {
			t.Errorf("expected allow decision, got %s", detail.IPRiskSummary.Decision)
		}
	})

	t.Run("zero_sensitive_data_in_api_view", func(t *testing.T) {
		views, _, err := svc.ListNodesReadModel(ctx, domain.NodeFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 50},
		})
		if err != nil {
			t.Fatalf("ListNodesReadModel failed: %v", err)
		}

		raw, err := json.Marshal(views)
		if err != nil {
			t.Fatalf("marshal views: %v", err)
		}
		jsonStr := string(raw)

		for _, forbidden := range []string{"normalized_config_secret_ref", "secret://", "pw1", "pw2", "198.51.100."} {
			if strings.Contains(jsonStr, forbidden) {
				t.Fatalf("JSON contains forbidden substring %q: %s", forbidden, jsonStr)
			}
		}
	})
}
