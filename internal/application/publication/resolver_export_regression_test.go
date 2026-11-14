package publication_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/compiler"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/internal/resolver"
)

// parsedMihomoConfig models the top-level proxies and proxy-groups in compiled Mihomo YAML.
type parsedMihomoConfig struct {
	Proxies []struct {
		Name   string `yaml:"name"`
		Type   string `yaml:"type"`
		Server string `yaml:"server"`
		Port   int    `yaml:"port"`
	} `yaml:"proxies"`
	ProxyGroups []struct {
		Name    string   `yaml:"name"`
		Type    string   `yaml:"type"`
		Proxies []string `yaml:"proxies"`
	} `yaml:"proxy-groups"`
}

// setupSyntheticInventory seeds an enabled subscription, a successful fetch record,
// and the provided nodes linked via node_sources, ensuring consistent facts inventory queries succeed.
func setupSyntheticInventory(t *testing.T, ctx context.Context, db *sql.DB, nodes []domain.Node) {
	t.Helper()
	now := domain.NowUTC()
	subRepo := sqlite.NewSubscriptionRepository(db)
	subID := "sub-synth-" + domain.MustNewUUIDv7()
	fetchID := "fetch-synth-" + domain.MustNewUUIDv7()

	sub := &domain.Subscription{
		ID:                 subID,
		Name:               "Synthetic Subscription",
		SourceURLSecretRef: "secret://synthetic-feed",
		Enabled:            true,
		Revision:           "rev-synth-1",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := subRepo.Create(ctx, sub); err != nil {
		t.Fatalf("create synthetic subscription: %v", err)
	}

	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	fetch := &domain.SubscriptionFetch{
		ID:             fetchID,
		SubscriptionID: subID,
		StartedAt:      now,
		FinishedAt:     &now,
		Outcome:        domain.FetchOutcomeSuccess,
		ContentDigest:  "sha256:synth-fetch-digest",
		NodesParsed:    len(nodes),
		NodesValid:     len(nodes),
	}
	if err := fetchRepo.Create(ctx, fetch); err != nil {
		t.Fatalf("create synthetic fetch: %v", err)
	}

	nodeRepo := sqlite.NewNodeRepository(db)
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("upsert synthetic nodes: %v", err)
	}

	for _, n := range nodes {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
			VALUES (?, ?, ?)
		`, n.LogicalID, subID, fetchID); err != nil {
			t.Fatalf("link synthetic node %s to source: %v", n.LogicalID, err)
		}
	}
}

// setupSyntheticProbeObservation creates the required probe run record to satisfy foreign key
// constraints before persisting the probe observation.
func setupSyntheticProbeObservation(t *testing.T, ctx context.Context, db *sql.DB, nodeID string, verdict domain.ProbeVerdict, connRev int64) {
	t.Helper()
	now := domain.NowUTC()
	runID := "probe-run-" + domain.MustNewUUIDv7()
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	if err := probeRunRepo.Create(ctx, &domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: runID,
		ActorScope:     "synthetic-test",
		ConfigRevision: "rev-synth",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("create synthetic probe run: %v", err)
	}

	obsRepo := sqlite.NewProbeObservationRepository(db)
	if err := obsRepo.Create(ctx, &domain.ProbeObservation{
		ID:                 domain.MustNewUUIDv7(),
		ProbeRunID:         runID,
		NodeLogicalID:      nodeID,
		Kind:               domain.ProbeKindBaseline,
		Verdict:            verdict,
		EvidenceDigest:     "sha256:synthetic-evidence",
		ObservedAt:         now,
		ConnectionRevision: &connRev,
	}); err != nil {
		t.Fatalf("create synthetic probe observation: %v", err)
	}
}

func TestPublicationResolverUsesGlobalPoolOnlyForInheritingGroups(t *testing.T) {
	for _, inherit := range []bool{false, true} {
		name := "independent_inherit_false"
		if inherit {
			name = "inheriting_inherit_true"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := newCleanSQLiteDB(t)
			now := domain.NowUTC()

			hkUnavailableID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.41", 8388, nil)
			hkBackupUnavailableID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.42", 8388, nil)
			usAvailableID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.43", 8388, nil)

			nodes := []domain.Node{
				{
					LogicalID:          hkUnavailableID,
					Protocol:           domain.ProtocolSS,
					DisplayName:        "HK unavailable",
					Server:             "198.51.100.41",
					Port:               8388,
					Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic-hk-1"},
					Active:             true,
					ConnectionRevision: 1,
					CreatedAt:          now,
					UpdatedAt:          now,
				},
				{
					LogicalID:          hkBackupUnavailableID,
					Protocol:           domain.ProtocolSS,
					DisplayName:        "Hk backup unavailable",
					Server:             "198.51.100.42",
					Port:               8388,
					Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic-hk-2"},
					Active:             true,
					ConnectionRevision: 1,
					CreatedAt:          now,
					UpdatedAt:          now,
				},
				{
					LogicalID:          usAvailableID,
					Protocol:           domain.ProtocolSS,
					DisplayName:        "US available",
					Server:             "198.51.100.43",
					Port:               8388,
					Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "synthetic-us-1"},
					Active:             true,
					ConnectionRevision: 1,
					CreatedAt:          now,
					UpdatedAt:          now,
				},
			}

			setupSyntheticInventory(t, ctx, db, nodes)
			setupSyntheticProbeObservation(t, ctx, db, hkUnavailableID, domain.VerdictError, 1)
			setupSyntheticProbeObservation(t, ctx, db, hkBackupUnavailableID, domain.VerdictError, 1)
			setupSyntheticProbeObservation(t, ctx, db, usAvailableID, domain.VerdictAvailable, 1)

			// Configure global node filter: requires available baseline probe.
			// Global empty pass is explicitly set to false to guarantee strict validation.
			filterRepo := sqlite.NewNodeFilterRepository(db)
			probeKind := domain.ProbeKindBaseline
			falseVal := false
			globalSpec := domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{
						Field:     domain.FilterFieldProbeVerdict,
						Op:        domain.FilterOpEquals,
						Value:     string(domain.VerdictAvailable),
						ProbeKind: &probeKind,
					},
				},
			}
			if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
				Spec:              globalSpec,
				EmptyFallbackPass: &falseVal,
			}); err != nil {
				t.Fatalf("persist global filter: %v", err)
			}
			revRepo := sqlite.NewRevisionRepository(db)
			revision := &domain.ConfigurationRevision{
				ID:            "rev-export-" + domain.MustNewUUIDv7(),
				ContentDigest: "sha256:synthetic-revision",
				State:         domain.RevisionStateActive,
				CreatedAt:     now,
			}
			if err := revRepo.Create(ctx, revision); err != nil {
				t.Fatalf("create active revision: %v", err)
			}
			if err := revRepo.SetActive(ctx, revision.ID); err != nil {
				t.Fatalf("set active revision: %v", err)
			}

			policyRepo := sqlite.NewPolicyRepository(db)
			groupID := domain.MustNewUUIDv7()
			pattern := "(?i)hk"
			group := &domain.NodeGroup{
				ID:                  groupID,
				Name:                "HK selection",
				GroupType:           domain.GroupTypeSelect,
				InheritGlobalFilter: inherit,
				EmptyFallbackPass:   false,
				CreatedAt:           now,
				UpdatedAt:           now,
			}
			if _, err := policyRepo.SaveGroupWithSources(ctx, group, 0, []domain.MemberSource{
				{
					Operation: domain.OperationAdd,
					Kind:      domain.SourceKindRegex,
					Pattern:   &pattern,
				},
			}); err != nil {
				t.Fatalf("persist policy group: %v", err)
			}

			// Route group with a MATCH rule in this revision
			if err := policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
				ID:            domain.MustNewUUIDv7(),
				RevisionID:    revision.ID,
				TargetGroupID: groupID,
				Expression:    "MATCH",
				Enabled:       true,
			}); err != nil {
				t.Fatalf("persist MATCH rule: %v", err)
			}

			// Verify group round-trip using GetGroupByID
			loadedGroup, err := policyRepo.GetGroupByID(ctx, groupID)
			if err != nil {
				t.Fatalf("GetGroupByID failed: %v", err)
			}
			if loadedGroup.InheritGlobalFilter != inherit {
				t.Fatalf("group inherit_global_filter mismatch: got %v, want %v", loadedGroup.InheritGlobalFilter, inherit)
			}

			linkRepo := sqlite.NewSubscriptionLinkRepository(db)
			pubRepo := sqlite.NewPublicationRepository(db)
			factsLoader := sqlite.NewConsistentFactsLoader(db)
			obsRepo := sqlite.NewProbeObservationRepository(db)
			nodeRepo := sqlite.NewNodeRepository(db)
			sourceRepo := sqlite.NewNodeSourceRepository(db)

			svc := publication.NewService(
				pubRepo,
				&dummyAuditRepo{},
				publication.WithPolicyRepository(policyRepo),
				publication.WithRevisionRepository(revRepo),
				publication.WithNodeRepository(nodeRepo),
				publication.WithNodeFilterRepository(filterRepo),
				publication.WithProbeObservationRepository(obsRepo),
				publication.WithNodeSourceRepository(sourceRepo),
				publication.WithSubscriptionLinkRepository(linkRepo),
				publication.WithConsistentFactsLoader(factsLoader),
			)

			if inherit {
				// Inheriting group: candidates are filtered by global filter (which only has US available).
				// Case-insensitive regex (?i)hk matches 0 nodes in the US-only global pool.
				// Since the group is routed and empty, strict preflight and preview must reject with empty routing diagnostic.
				preflight, err := svc.Preflight(ctx, publication.PreflightCommand{
					Target:     domain.TargetMihomo,
					RevisionID: revision.ID,
				})
				if err != nil {
					t.Fatalf("preflight call returned unexpected error: %v", err)
				}
				if preflight.Allowed {
					t.Fatal("preflight should NOT be allowed for empty routed inheriting group")
				}
				foundDiag := false
				for _, d := range preflight.Diagnostics {
					if d.Code == "empty_routed_group" || d.Code == "required_nonempty" {
						foundDiag = true
						break
					}
				}
				if !foundDiag {
					t.Fatalf("expected empty_routed_group or required_nonempty diagnostic, got: %+v", preflight.Diagnostics)
				}

				_, prevErr := svc.Preview(ctx, publication.PreviewQuery{
					Target:     domain.TargetMihomo,
					RevisionID: revision.ID,
					CompatMode: "strict",
				})
				if prevErr == nil {
					t.Fatal("strict preview should fail for empty routed inheriting group")
				}
				var strictErr *publication.StrictCapabilityError
				if !errors.As(prevErr, &strictErr) || strictErr.Code != "required_nonempty" {
					t.Fatalf("expected strict required_nonempty, got %T: %v", prevErr, prevErr)
				}
				return
			}

			// Non-inheriting group (inherit_global_filter = false):
			// 1. Group candidate pool is the full admitted inventory (contains HK unavailable and Hk backup unavailable).
			// 2. Both HK nodes enter the group regardless of lack of available probe observations.
			// 3. Snapshot global union semantics:
			//    Global filter admitted: 1 node (US available)
			//    Group referenced: 2 nodes (HK unavailable, Hk backup unavailable)
			//    Total snapshot nodes: 3 nodes (Union of global + group)
			//    Manifest.NodeCount == 3
			prevRes, err := svc.Preview(ctx, publication.PreviewQuery{
				Target:     domain.TargetMihomo,
				RevisionID: revision.ID,
				CompatMode: "strict",
			})
			if err != nil {
				t.Fatalf("strict preview failed for independent group: %v", err)
			}
			if prevRes.Manifest.NodeCount != 3 {
				t.Fatalf("expected snapshot global union node_count=3, got %d (manifest: %+v)", prevRes.Manifest.NodeCount, prevRes.Manifest)
			}
			if prevRes.Manifest.ExcludedCount != 0 {
				t.Fatalf("expected excluded_count=0, got %d", prevRes.Manifest.ExcludedCount)
			}

			var parsed parsedMihomoConfig
			if err := yaml.Unmarshal(prevRes.Content, &parsed); err != nil {
				t.Fatalf("unmarshal compiled Mihomo YAML failed: %v", err)
			}
			if len(parsed.Proxies) != 3 {
				t.Fatalf("expected 3 proxies in top-level YAML, got %d", len(parsed.Proxies))
			}
			if len(parsed.ProxyGroups) != 1 {
				t.Fatalf("expected 1 proxy group in YAML, got %d", len(parsed.ProxyGroups))
			}
			groupMembers := parsed.ProxyGroups[0].Proxies
			if len(groupMembers) != 2 {
				t.Fatalf("expected exactly 2 HK members in group 'HK selection', got %d: %v", len(groupMembers), groupMembers)
			}
			hasHK := false
			hasHkBackup := false
			for _, m := range groupMembers {
				if m == "HK unavailable" {
					hasHK = true
				}
				if m == "Hk backup unavailable" {
					hasHkBackup = true
				}
				if m == "US available" {
					t.Errorf("group member list must NOT include non-matching 'US available'")
				}
			}
			if !hasHK || !hasHkBackup {
				t.Errorf("group missing expected HK members: hasHK=%v hasHkBackup=%v (members: %v)", hasHK, hasHkBackup, groupMembers)
			}

			// 4. Test Global Zero (Global filter matching 0 nodes):
			// A global filter that matches zero nodes must NOT trigger 'filtered_nodes_empty' when an
			// independent group has resolved valid members from the full inventory.
			zeroSpec := domain.NodeFilterSpec{
				Conditions: []domain.FilterCondition{
					{
						Field: domain.FilterFieldDisplayName,
						Op:    domain.FilterOpContains,
						Value: "non-existent-synthetic-label",
					},
				},
			}
			if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
				Spec:              zeroSpec,
				EmptyFallbackPass: &falseVal,
			}); err != nil {
				t.Fatalf("update global filter to zero-match: %v", err)
			}

			zeroPreview, err := svc.Preview(ctx, publication.PreviewQuery{
				Target:     domain.TargetMihomo,
				RevisionID: revision.ID,
				CompatMode: "strict",
			})
			if err != nil {
				t.Fatalf("preview under zero global matches failed: %v", err)
			}
			// When global matches 0, snapshot.Nodes contains only the group-referenced nodes (2 HK nodes)
			if zeroPreview.Manifest.NodeCount != 2 {
				t.Fatalf("expected manifest node_count=2 under global zero, got %d (manifest: %+v)", zeroPreview.Manifest.NodeCount, zeroPreview.Manifest)
			}

			var parsedZero parsedMihomoConfig
			if err := yaml.Unmarshal(zeroPreview.Content, &parsedZero); err != nil {
				t.Fatalf("unmarshal zero global preview content: %v", err)
			}
			if len(parsedZero.Proxies) != 2 {
				t.Fatalf("expected 2 proxies in top-level YAML under global zero, got %d", len(parsedZero.Proxies))
			}
			if zeroPreview.FilterCounts == nil || zeroPreview.FilterCounts.GlobalFilteredTotal != 0 {
				t.Fatalf("global filter accounting changed: %+v", zeroPreview.FilterCounts)
			}
			for _, target := range []domain.CompilerTarget{domain.TargetSingBox, domain.TargetSurge} {
				if _, err := svc.Preview(ctx, publication.PreviewQuery{Target: target, RevisionID: revision.ID, CompatMode: "strict"}); err == nil {
					t.Errorf("node-only target %s must not use non-inheriting Mihomo group", target)
				}
			}
			if len(parsedZero.ProxyGroups) != 1 || len(parsedZero.ProxyGroups[0].Proxies) != 2 {
				t.Fatalf("expected group 'HK selection' to retain 2 members under global zero, got: %+v", parsedZero.ProxyGroups)
			}

			// 5. Test Direct Target Compilation (GenerateTargetArtifact):
			targetArt, err := svc.GenerateTargetArtifact(ctx, domain.TargetMihomo)
			if err != nil {
				t.Fatalf("GenerateTargetArtifact failed for Mihomo: %v", err)
			}
			if !strings.Contains(string(targetArt.Content), "HK unavailable") || !strings.Contains(string(targetArt.Content), "Hk backup unavailable") {
				t.Errorf("GenerateTargetArtifact missing HK nodes: %s", string(targetArt.Content))
			}

			// 6. Test Live Subscription Link (GenerateLiveArtifact):
			link, err := svc.CreateSubscriptionLink(ctx, publication.CreateSubscriptionLinkCommand{
				Label:     "Mihomo Live Test",
				Target:    domain.TargetMihomo,
				IsDefault: true,
			})
			if err != nil {
				t.Fatalf("CreateSubscriptionLink failed: %v", err)
			}
			liveArt, err := svc.GenerateLiveArtifact(ctx, link.ID, link.Token)
			if err != nil {
				t.Fatalf("GenerateLiveArtifact failed: %v", err)
			}
			if liveArt.Target != domain.TargetMihomo || len(liveArt.Content) == 0 {
				t.Fatalf("invalid live artifact: %+v", liveArt)
			}
			if !strings.Contains(string(liveArt.Content), "HK unavailable") || !strings.Contains(string(liveArt.Content), "Hk backup unavailable") {
				t.Errorf("live artifact missing HK nodes: %s", string(liveArt.Content))
			}

			// 7. Test Publish & ResolveAndServe (immutable published artifact retrieval):
			pubRes, err := svc.Publish(ctx, publication.PublishCommand{
				Target:     domain.TargetMihomo,
				SnapshotID: zeroPreview.SnapshotID,
			})
			if err != nil {
				t.Fatalf("publish snapshot failed: %v", err)
			}
			served, err := svc.ResolveAndServe(ctx, pubRes.Publication.ID, pubRes.RawToken)
			if err != nil {
				t.Fatalf("ResolveAndServe failed: %v", err)
			}
			if string(served.Content) != string(zeroPreview.Content) {
				t.Errorf("served published content diverged from draft preview content")
			}
		})
	}
}

func TestPolicyGroupPreviewRespectsInheritGlobalFilterWithUnavailableNodes(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	now := domain.NowUTC()

	hkID1 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.51", 8388, nil)
	hkID2 := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.52", 8388, nil)
	usID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.53", 8388, nil)

	nodes := []domain.Node{
		{
			LogicalID:          hkID1,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "HK Alpha",
			Server:             "198.51.100.51",
			Port:               8388,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pwd-1"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          hkID2,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "Hk Beta",
			Server:             "198.51.100.52",
			Port:               8388,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pwd-2"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          usID,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "US Charlie",
			Server:             "198.51.100.53",
			Port:               8388,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pwd-3"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	setupSyntheticInventory(t, ctx, db, nodes)
	setupSyntheticProbeObservation(t, ctx, db, hkID1, domain.VerdictError, 1)
	setupSyntheticProbeObservation(t, ctx, db, hkID2, domain.VerdictError, 1)
	setupSyntheticProbeObservation(t, ctx, db, usID, domain.VerdictAvailable, 1)

	filterRepo := sqlite.NewNodeFilterRepository(db)
	probeKind := domain.ProbeKindBaseline
	falseVal := false
	if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
		Spec: domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field:     domain.FilterFieldProbeVerdict,
					Op:        domain.FilterOpEquals,
					Value:     string(domain.VerdictAvailable),
					ProbeKind: &probeKind,
				},
			},
		},
		EmptyFallbackPass: &falseVal,
	}); err != nil {
		t.Fatalf("persist global filter: %v", err)
	}

	revRepo := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{
		ID:            "rev-policy-preview-" + domain.MustNewUUIDv7(),
		ContentDigest: "sha256:rev-policy-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     now,
	}
	if err := revRepo.Create(ctx, rev); err != nil {
		t.Fatalf("create active revision: %v", err)
	}
	if err := revRepo.SetActive(ctx, rev.ID); err != nil {
		t.Fatalf("set active revision: %v", err)
	}

	policyRepo := sqlite.NewPolicyRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	factsLoader := sqlite.NewConsistentFactsLoader(db)

	pubSvc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(filterRepo),
		publication.WithProbeObservationRepository(obsRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)

	policySvc := policy.NewService(policyRepo, revRepo, nodeRepo, &dummyAuditRepo{}, filterRepo)
	policySvc.SetInputProvider(pubSvc.AssembleResolveInputForPreview)

	patHK := "(?i)hk"
	name := "HK Group Preview"
	gType := domain.GroupTypeSelect

	// Subtest 1: inherit_global_filter = false -> HK nodes enter group preview
	t.Run("inherit_false_yields_members", func(t *testing.T) {
		inheritFalse := false
		resp, err := policySvc.PreviewGroup(ctx, policy.PreviewGroupCommand{
			Name:                &name,
			GroupType:           &gType,
			InheritGlobalFilter: &inheritFalse,
			Sources: []domain.MemberSource{
				{
					Operation: domain.OperationAdd,
					Kind:      domain.SourceKindRegex,
					Position:  0,
					Pattern:   &patHK,
				},
			},
		})
		if err != nil {
			t.Fatalf("PreviewGroup failed with inherit=false: %v", err)
		}
		if resp.MemberCount != 2 {
			t.Fatalf("expected member_count=2, got %d", resp.MemberCount)
		}
		foundAlpha := false
		foundBeta := false
		for _, m := range resp.Members {
			if m.DisplayName == "HK Alpha" {
				foundAlpha = true
			}
			if m.DisplayName == "Hk Beta" {
				foundBeta = true
			}
		}
		if !foundAlpha || !foundBeta {
			t.Errorf("missing members in preview response: foundAlpha=%v, foundBeta=%v", foundAlpha, foundBeta)
		}
	})

	// Subtest 2: inherit_global_filter = true -> HK nodes are excluded by global filter
	t.Run("inherit_true_excludes_members", func(t *testing.T) {
		inheritTrue := true
		resp, err := policySvc.PreviewGroup(ctx, policy.PreviewGroupCommand{
			Name:                &name,
			GroupType:           &gType,
			InheritGlobalFilter: &inheritTrue,
			Sources: []domain.MemberSource{
				{
					Operation: domain.OperationAdd,
					Kind:      domain.SourceKindRegex,
					Position:  0,
					Pattern:   &patHK,
				},
			},
		})
		if err != nil {
			t.Fatalf("PreviewGroup failed with inherit=true: %v", err)
		}
		if resp.MemberCount != 0 {
			t.Fatalf("expected member_count=0 with inherit=true, got %d (members: %+v)", resp.MemberCount, resp.Members)
		}
	})
}

func TestStrictPreflightDiagnostics_RequiredNonemptyAndMixedCapabilities(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	now := domain.NowUTC()

	vlessID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "198.51.100.61", 443, nil)
	ssID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.62", 8388, nil)

	nodes := []domain.Node{
		{
			LogicalID:          vlessID,
			Protocol:           domain.ProtocolVLESS,
			DisplayName:        "HK VLESS Node",
			Server:             "198.51.100.61",
			Port:               443,
			Credentials:        domain.InboundProtocolCredential{UUID: "11111111-2222-3333-4444-555555555555", Transport: map[string]string{"network": "kcp"}},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          ssID,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "Shadowsocks Node",
			Server:             "198.51.100.62",
			Port:               8388,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pwd"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	setupSyntheticInventory(t, ctx, db, nodes)

	revRepo := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{
		ID:            "rev-diag-" + domain.MustNewUUIDv7(),
		ContentDigest: "sha256:diag-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     now,
	}
	if err := revRepo.Create(ctx, rev); err != nil {
		t.Fatalf("create active revision: %v", err)
	}
	if err := revRepo.SetActive(ctx, rev.ID); err != nil {
		t.Fatalf("set active revision: %v", err)
	}

	// 1. Create a group that matches ZERO nodes (pure required_nonempty / empty_routed_group)
	policyRepo := sqlite.NewPolicyRepository(db)
	emptyGroupID := domain.MustNewUUIDv7()
	patTokyo := "(?i)tokyo"
	emptyGroup := &domain.NodeGroup{
		ID:                  emptyGroupID,
		Name:                "Tokyo Group",
		GroupType:           domain.GroupTypeSelect,
		InheritGlobalFilter: true,
		EmptyFallbackPass:   false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if _, err := policyRepo.SaveGroupWithSources(ctx, emptyGroup, 0, []domain.MemberSource{
		{
			Operation: domain.OperationAdd,
			Kind:      domain.SourceKindRegex,
			Pattern:   &patTokyo,
		},
	}); err != nil {
		t.Fatalf("create empty group: %v", err)
	}

	// Route Tokyo with a valid non-terminal rule; the non-inheriting HK group
	// owns the unique terminal MATCH rule.
	if err := policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            domain.MustNewUUIDv7(),
		RevisionID:    rev.ID,
		TargetGroupID: emptyGroupID,
		Expression:    "DOMAIN-SUFFIX,example.test",
		Position:      0,
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create Tokyo route rule: %v", err)
	}
	hkGroupID := domain.MustNewUUIDv7()
	patHK := "(?i)hk"
	if _, err := policyRepo.SaveGroupWithSources(ctx, &domain.NodeGroup{
		ID: hkGroupID, Name: "HK Group", GroupType: domain.GroupTypeSelect,
		InheritGlobalFilter: false, EmptyFallbackPass: false, CreatedAt: now, UpdatedAt: now,
	}, 0, []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindRegex, Pattern: &patHK}}); err != nil {
		t.Fatalf("create non-inheriting HK group: %v", err)
	}
	if err := policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID: domain.MustNewUUIDv7(), RevisionID: rev.ID, TargetGroupID: hkGroupID,
		Expression: "MATCH", Position: 1, Enabled: true,
	}); err != nil {
		t.Fatalf("create HK MATCH rule: %v", err)
	}

	factsLoader := sqlite.NewConsistentFactsLoader(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	filterRepo := sqlite.NewNodeFilterRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	svc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(filterRepo),
		publication.WithProbeObservationRepository(obsRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)
	globalEmptyPass := false
	if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
		Spec: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
			Field: domain.FilterFieldDisplayName, Op: domain.FilterOpContains, Value: "no-node-matches-this-filter",
		}}},
		EmptyFallbackPass: &globalEmptyPass,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("set empty global filter for independent empty-route regression: %v", err)
	}

	// Subtest 1: Pure required_nonempty / empty_routed_group on Mihomo target
	t.Run("pure_empty_routed_group_preflight_diagnostic", func(t *testing.T) {
		preflight, err := svc.Preflight(ctx, publication.PreflightCommand{
			Target:     domain.TargetMihomo,
			RevisionID: rev.ID,
		})
		if err != nil {
			t.Fatalf("preflight error: %v", err)
		}
		if preflight.Allowed {
			t.Fatal("preflight should fail for empty routed Tokyo group")
		}
		if len(preflight.Diagnostics) < 2 {
			t.Fatalf("expected Tokyo empty-route and unsupported VLESS transport diagnostics together, got: %+v", preflight.Diagnostics)
		}
		foundTokyoEmptyDiag := false
		foundCapabilityDiag := false
		for _, d := range preflight.Diagnostics {
			if d.Code == "empty_routed_group" && d.Target == emptyGroupID {
				foundTokyoEmptyDiag = true
			}
			if d.Code == "unsupported_target_capability" {
				foundCapabilityDiag = true
			}
		}
		if !foundTokyoEmptyDiag || !foundCapabilityDiag {
			t.Fatalf("expected both Tokyo empty-route and capability diagnostics, got: %+v", preflight.Diagnostics)
		}
		foundEmptyDiag := false
		for _, d := range preflight.Diagnostics {
			if d.Code == "empty_routed_group" || d.Code == "required_nonempty" {
				foundEmptyDiag = true
				if d.Severity != resolver.DiagnosticSeverityError {
					t.Errorf("diagnostic severity: got %s, want error", d.Severity)
				}
				break
			}
		}
		if !foundEmptyDiag {
			t.Fatalf("expected empty_routed_group diagnostic, got: %+v", preflight.Diagnostics)
		}
	})

	// Subtest 2: Strict capability failure on non-Mihomo target. The global
	// filter is cleared so the pure-node Surge snapshot contains the VLESS node;
	// Surge intentionally strips policy groups/rules.
	t.Run("strict_capability_diagnostic_surge_rejects_vless", func(t *testing.T) {
		if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
			Spec: domain.NodeFilterSpec{}, EmptyFallbackPass: &globalEmptyPass, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("clear global filter for Surge capability case: %v", err)
		}
		_, err := svc.Preview(ctx, publication.PreviewQuery{
			Target:     domain.TargetSurge,
			RevisionID: rev.ID,
			CompatMode: "strict",
		})
		if err == nil {
			t.Fatal("expected strict Surge preview to fail on VLESS node, got nil")
		}
		var strictCapErr *publication.StrictCapabilityError
		if !errors.As(err, &strictCapErr) {
			t.Fatalf("expected *publication.StrictCapabilityError, got %T: %v", err, err)
		}
		foundVLESSDiag := false
		for _, cd := range strictCapErr.Diagnostics {
			if cd.Code == "unsupported_target_capability" && cd.NodeID == vlessID {
				foundVLESSDiag = true
				break
			}
		}
		if !foundVLESSDiag {
			t.Fatalf("expected unsupported_target_capability for node %s, got: %+v", vlessID, strictCapErr.Diagnostics)
		}
	})

	// Subtest 3: Direct compiler ValidateTargetCapabilities test
	t.Run("compiler_validate_target_capabilities", func(t *testing.T) {
		snap := &resolver.ResolvedPolicySnapshot{
			RevisionID:      rev.ID,
			SnapshotDigest:  "sha256:test-snap",
			CompilerVersion: "1.0.0",
			Nodes: []resolver.ResolvedNode{
				{LogicalID: vlessID, Protocol: domain.ProtocolVLESS, DisplayName: "VLESS", Active: true},
			},
			Groups: []resolver.ResolvedGroup{
				{
					ID:                 emptyGroupID,
					Name:               "Tokyo Group",
					GroupType:          domain.GroupTypeSelect,
					EmptyFallbackPass:  false,
					EffectiveEmptyPass: false,
					Members:            nil,
					AllNodeLogicalIDs:  nil,
				},
			},
		}

		// Mihomo flags required_nonempty and preserves mixed capability + required diagnostics
		snap.Nodes = append(snap.Nodes, resolver.ResolvedNode{
			LogicalID:   "synthetic-bad-network",
			Protocol:    domain.ProtocolVLESS,
			DisplayName: "Bad VLESS",
			Server:      "198.51.100.63",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{UUID: "11111111-2222-3333-4444-555555555555", Transport: map[string]string{"network": "kcp"}},
			Active:      true,
		})
		mihomoDiags := compiler.ValidateTargetCapabilities(snap, domain.TargetMihomo)
		foundRequiredNonempty := false
		foundCap := false
		for _, d := range mihomoDiags {
			if d.Code == "required_nonempty" {
				foundRequiredNonempty = true
			}
			if d.Code == "unsupported_target_capability" && d.NodeID == "synthetic-bad-network" {
				foundCap = true
			}
		}
		if !foundRequiredNonempty || !foundCap {
			t.Errorf("expected both required_nonempty and unsupported_target_capability in mihomoDiags: %+v", mihomoDiags)
		}
		_, mixedErr := svc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, RevisionID: rev.ID, Snapshot: snap, CompatMode: "strict"})
		var mixedStrict *publication.StrictCapabilityError
		if !errors.As(mixedErr, &mixedStrict) || mixedStrict.Code != "unsupported_target_capability" || len(mixedStrict.Diagnostics) < 2 {
			t.Fatalf("expected mixed strict capability wrapper preserving diagnostics, got: %v", mixedErr)
		}

		// Surge flags unsupported_target_capability for VLESS
		surgeDiags := compiler.ValidateTargetCapabilities(snap, domain.TargetSurge)
		foundSurgeCap := false
		for _, d := range surgeDiags {
			if d.Code == "unsupported_target_capability" && d.NodeID == vlessID {
				foundSurgeCap = true
			}
		}
		if !foundSurgeCap {
			t.Errorf("expected unsupported_target_capability in surgeDiags: %+v", surgeDiags)
		}
	})
}

// TestGlobalFilterTrueEmpty_BothGlobalAndGroupsEmpty verifies that when a global filter
// matches 0 nodes AND all policy groups are also empty (true empty inventory),
// strict Preflight and Preview correctly detect and reject with filtered_nodes_empty.
func TestGlobalFilterTrueEmpty_BothGlobalAndGroupsEmpty(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	now := domain.NowUTC()

	// Seed one node that will be excluded by both global filter and group filter
	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.71", 8388, nil)
	nodes := []domain.Node{
		{
			LogicalID:          nodeID,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "FR Paris",
			Server:             "198.51.100.71",
			Port:               8388,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pwd"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	setupSyntheticInventory(t, ctx, db, nodes)

	// Global filter matches "non-existent-label" (0 nodes admitted)
	filterRepo := sqlite.NewNodeFilterRepository(db)
	falseVal := false
	if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
		Spec: domain.NodeFilterSpec{
			Conditions: []domain.FilterCondition{
				{
					Field: domain.FilterFieldDisplayName,
					Op:    domain.FilterOpContains,
					Value: "non-existent-label",
				},
			},
		},
		EmptyFallbackPass: &falseVal,
	}); err != nil {
		t.Fatalf("persist global filter: %v", err)
	}

	revRepo := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{
		ID:            "rev-true-empty-" + domain.MustNewUUIDv7(),
		ContentDigest: "sha256:true-empty-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     now,
	}
	if err := revRepo.Create(ctx, rev); err != nil {
		t.Fatalf("create active revision: %v", err)
	}
	if err := revRepo.SetActive(ctx, rev.ID); err != nil {
		t.Fatalf("set active revision: %v", err)
	}

	// Group also matches nothing (pattern (?i)tokyo vs "FR Paris") with EmptyFallbackPass=false
	policyRepo := sqlite.NewPolicyRepository(db)
	emptyGroupID := domain.MustNewUUIDv7()
	patTokyo := "(?i)tokyo"
	emptyGroup := &domain.NodeGroup{
		ID:                  emptyGroupID,
		Name:                "Tokyo Group",
		GroupType:           domain.GroupTypeSelect,
		InheritGlobalFilter: false,
		EmptyFallbackPass:   false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if _, err := policyRepo.SaveGroupWithSources(ctx, emptyGroup, 0, []domain.MemberSource{
		{
			Operation: domain.OperationAdd,
			Kind:      domain.SourceKindRegex,
			Pattern:   &patTokyo,
		},
	}); err != nil {
		t.Fatalf("create empty group: %v", err)
	}
	if err := policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
		ID:            domain.MustNewUUIDv7(),
		RevisionID:    rev.ID,
		TargetGroupID: emptyGroupID,
		Expression:    "MATCH",
		Enabled:       true,
	}); err != nil {
		t.Fatalf("create MATCH rule: %v", err)
	}

	factsLoader := sqlite.NewConsistentFactsLoader(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	svc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(filterRepo),
		publication.WithProbeObservationRepository(obsRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)

	// Preflight must reject because both global and group pools are genuinely empty
	preflight, err := svc.Preflight(ctx, publication.PreflightCommand{
		Target:     domain.TargetMihomo,
		RevisionID: rev.ID,
	})
	if err != nil {
		t.Fatalf("preflight error: %v", err)
	}
	if preflight.Allowed {
		t.Fatal("preflight should NOT be allowed when global filter and all groups resolve to 0 nodes")
	}
	foundFilteredEmpty := false
	for _, d := range preflight.Diagnostics {
		if d.Code == "filtered_nodes_empty" || d.Code == "empty_routed_group" || d.Code == "required_nonempty" {
			foundFilteredEmpty = true
			break
		}
	}
	if !foundFilteredEmpty {
		t.Fatalf("expected filtered_nodes_empty or empty_routed_group diagnostic, got: %+v", preflight.Diagnostics)
	}

	// Preview must reject
	_, prevErr := svc.Preview(ctx, publication.PreviewQuery{
		Target:     domain.TargetMihomo,
		RevisionID: rev.ID,
		CompatMode: "strict",
	})
	if prevErr == nil {
		t.Fatal("strict preview should fail when global and all groups are truly empty")
	}
}

// TestNonMihomoPureNodeExportBoundary verifies that non-Mihomo targets (Sing-box, Surge)
// operate on pure node candidate pools (ignoring proxy-group routing structures)
// and properly validate capabilities in strict mode vs compatible mode.
func TestNonMihomoPureNodeExportBoundary(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	now := domain.NowUTC()

	vlessID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "198.51.100.81", 443, nil)
	ssID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.82", 8388, nil)

	nodes := []domain.Node{
		{
			LogicalID:          vlessID,
			Protocol:           domain.ProtocolVLESS,
			DisplayName:        "VLESS Strict Candidate",
			Server:             "198.51.100.81",
			Port:               443,
			Credentials:        domain.InboundProtocolCredential{UUID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		{
			LogicalID:          ssID,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "Shadowsocks Universal",
			Server:             "198.51.100.82",
			Port:               8388,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pwd"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	setupSyntheticInventory(t, ctx, db, nodes)

	revRepo := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{
		ID:            "rev-non-mihomo-" + domain.MustNewUUIDv7(),
		ContentDigest: "sha256:non-mihomo-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     now,
	}
	if err := revRepo.Create(ctx, rev); err != nil {
		t.Fatalf("create active revision: %v", err)
	}
	if err := revRepo.SetActive(ctx, rev.ID); err != nil {
		t.Fatalf("set active revision: %v", err)
	}

	policyRepo := sqlite.NewPolicyRepository(db)
	factsLoader := sqlite.NewConsistentFactsLoader(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	filterRepo := sqlite.NewNodeFilterRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	svc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(filterRepo),
		publication.WithProbeObservationRepository(obsRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)

	// Surge in strict mode rejects VLESS
	t.Run("surge_strict_rejects_vless_pure_node", func(t *testing.T) {
		_, err := svc.Preview(ctx, publication.PreviewQuery{
			Target:     domain.TargetSurge,
			RevisionID: rev.ID,
			CompatMode: "strict",
		})
		if err == nil {
			t.Fatal("expected strict Surge preview to fail on VLESS node")
		}
		var strictCapErr *publication.StrictCapabilityError
		if !errors.As(err, &strictCapErr) {
			t.Fatalf("expected *publication.StrictCapabilityError, got %T: %v", err, err)
		}
		if strictCapErr.Code != "unsupported_target_capability" {
			t.Errorf("expected code unsupported_target_capability, got %s", strictCapErr.Code)
		}
	})

	t.Run("vless_raw_preview_direct_and_live_mihomo_export", func(t *testing.T) {
		ctx := context.Background()
		db := newCleanSQLiteDB(t)
		now := domain.NowUTC()

		rawTransport := map[string]string{
			"network": "raw",
			"tls":     "true",
			"sni":     "sni.synthetic.invalid",
			"flow":    "xtls-rprx-vision",
			"pbk":     "synthetic-public-key",
			"sid":     "abcd",
		}
		rawID := domain.ComputeNodeLogicalID(domain.ProtocolVLESS, "198.51.100.99", 443, rawTransport)
		nodes := []domain.Node{{
			LogicalID:   rawID,
			Protocol:    domain.ProtocolVLESS,
			DisplayName: "HK VLESS Raw",
			Server:      "198.51.100.99",
			Port:        443,
			Credentials: domain.InboundProtocolCredential{
				UUID:      "11111111-2222-4333-8444-555555555555",
				Transport: rawTransport,
			},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		}}
		setupSyntheticInventory(t, ctx, db, nodes)
		setupSyntheticProbeObservation(t, ctx, db, rawID, domain.VerdictError, 1)

		filterRepo := sqlite.NewNodeFilterRepository(db)
		falseVal := false
		probeKind := domain.ProbeKindBaseline
		if err := filterRepo.SetGlobalFilter(ctx, &domain.GlobalNodeFilter{
			Spec: domain.NodeFilterSpec{Conditions: []domain.FilterCondition{{
				Field: domain.FilterFieldProbeVerdict, Op: domain.FilterOpEquals,
				Value: string(domain.VerdictAvailable), ProbeKind: &probeKind,
			}}},
			EmptyFallbackPass: &falseVal,
		}); err != nil {
			t.Fatalf("set global filter: %v", err)
		}

		revRepo := sqlite.NewRevisionRepository(db)
		rev := &domain.ConfigurationRevision{ID: "rev-raw-export-" + domain.MustNewUUIDv7(), ContentDigest: "sha256:raw-export", State: domain.RevisionStateActive, CreatedAt: now}
		if err := revRepo.Create(ctx, rev); err != nil {
			t.Fatalf("create revision: %v", err)
		}
		if err := revRepo.SetActive(ctx, rev.ID); err != nil {
			t.Fatalf("activate revision: %v", err)
		}

		policyRepo := sqlite.NewPolicyRepository(db)
		groupID := domain.MustNewUUIDv7()
		patHK := "(?i)hk"
		if _, err := policyRepo.SaveGroupWithSources(ctx, &domain.NodeGroup{
			ID: groupID, Name: "HK Raw", GroupType: domain.GroupTypeSelect,
			InheritGlobalFilter: false, EmptyFallbackPass: false, CreatedAt: now, UpdatedAt: now,
		}, 0, []domain.MemberSource{{Operation: domain.OperationAdd, Kind: domain.SourceKindRegex, Pattern: &patHK}}); err != nil {
			t.Fatalf("save HK group: %v", err)
		}
		if err := policyRepo.CreatePolicyRule(ctx, &domain.PolicyRule{
			ID: domain.MustNewUUIDv7(), RevisionID: rev.ID, TargetGroupID: groupID, Expression: "MATCH", Enabled: true,
		}); err != nil {
			t.Fatalf("create MATCH rule: %v", err)
		}

		linkRepo := sqlite.NewSubscriptionLinkRepository(db)
		rawSvc := publication.NewService(
			sqlite.NewPublicationRepository(db),
			&dummyAuditRepo{},
			publication.WithPolicyRepository(policyRepo),
			publication.WithRevisionRepository(revRepo),
			publication.WithNodeRepository(sqlite.NewNodeRepository(db)),
			publication.WithNodeFilterRepository(filterRepo),
			publication.WithProbeObservationRepository(sqlite.NewProbeObservationRepository(db)),
			publication.WithNodeSourceRepository(sqlite.NewNodeSourceRepository(db)),
			publication.WithSubscriptionLinkRepository(linkRepo),
			publication.WithConsistentFactsLoader(sqlite.NewConsistentFactsLoader(db)),
		)

		prevRes, err := rawSvc.Preview(ctx, publication.PreviewQuery{Target: domain.TargetMihomo, RevisionID: rev.ID, CompatMode: "strict"})
		if err != nil {
			t.Fatalf("strict Mihomo preview rejected VLESS raw in non-inheriting HK group: %v", err)
		}
		directArt, err := rawSvc.GenerateTargetArtifact(ctx, domain.TargetMihomo)
		if err != nil {
			t.Fatalf("direct Mihomo export failed for VLESS raw: %v", err)
		}
		link, err := rawSvc.CreateSubscriptionLink(ctx, publication.CreateSubscriptionLinkCommand{Label: "Raw Live", Target: domain.TargetMihomo, IsDefault: true})
		if err != nil {
			t.Fatalf("create subscription link: %v", err)
		}
		liveArt, err := rawSvc.GenerateLiveArtifact(ctx, link.ID, link.Token)
		if err != nil {
			t.Fatalf("live Mihomo export failed for VLESS raw: %v", err)
		}
		for _, payload := range [][]byte{prevRes.Content, directArt.Content, liveArt.Content} {
			var cfg struct {
				Proxies []map[string]any `yaml:"proxies"`
			}
			if err := yaml.Unmarshal(payload, &cfg); err != nil {
				t.Fatalf("decode Mihomo YAML: %v", err)
			}
			if len(cfg.Proxies) != 1 || cfg.Proxies[0]["network"] != "tcp" || cfg.Proxies[0]["name"] != "HK VLESS Raw" {
				t.Fatalf("unexpected compiled VLESS raw proxy output: %+v", cfg.Proxies)
			}
		}
		if loaded, err := sqlite.NewNodeRepository(db).GetByLogicalID(ctx, rawID); err != nil || loaded == nil || loaded.Credentials.Transport["network"] != "raw" {
			t.Fatalf("stored VLESS transport mutated or ID drifted: err=%v node=%+v", err, loaded)
		}
	})

	// Surge in compatible mode filters out VLESS and exports Shadowsocks
	t.Run("surge_compatible_filters_vless_exports_shadowsocks", func(t *testing.T) {
		prevRes, err := svc.Preview(ctx, publication.PreviewQuery{
			Target:     domain.TargetSurge,
			RevisionID: rev.ID,
			CompatMode: "compatible",
		})
		if err != nil {
			t.Fatalf("compatible Surge preview failed: %v", err)
		}
		if prevRes.Manifest.NodeCount != 1 {
			t.Fatalf("expected 1 node in compatible Surge manifest, got %d", prevRes.Manifest.NodeCount)
		}
		if prevRes.Manifest.ExcludedCount != 1 {
			t.Fatalf("expected 1 excluded node in compatible Surge manifest, got %d", prevRes.Manifest.ExcludedCount)
		}
		content := string(prevRes.Content)
		if !strings.Contains(content, "Shadowsocks Universal") {
			t.Errorf("compatible Surge export missing Shadowsocks node: %s", content)
		}
		if strings.Contains(content, "VLESS Strict Candidate") {
			t.Errorf("compatible Surge export should NOT contain VLESS node: %s", content)
		}
	})

	// Sing-box exports both VLESS and Shadowsocks cleanly
	t.Run("singbox_exports_both_nodes", func(t *testing.T) {
		prevRes, err := svc.Preview(ctx, publication.PreviewQuery{
			Target:     domain.TargetSingBox,
			RevisionID: rev.ID,
			CompatMode: "strict",
		})
		if err != nil {
			t.Fatalf("strict Sing-box preview failed: %v", err)
		}
		if prevRes.Manifest.NodeCount != 2 {
			t.Fatalf("expected 2 nodes in Sing-box manifest, got %d", prevRes.Manifest.NodeCount)
		}
		content := string(prevRes.Content)
		if !strings.Contains(content, "VLESS Strict Candidate") || !strings.Contains(content, "Shadowsocks Universal") {
			t.Errorf("Sing-box export missing nodes: %s", content)
		}
	})
}

// TestLegacyPublicationRecompileFallback verifies the fallback path in ResolveAndServeAuthorized
// for legacy publication records that were persisted without Content (Content = nil or empty).
// The service must recompile the snapshot on-the-fly using the publication's RevisionID.
func TestLegacyPublicationRecompileFallback(t *testing.T) {
	ctx := context.Background()
	db := newCleanSQLiteDB(t)
	now := domain.NowUTC()

	nodeID := domain.ComputeNodeLogicalID(domain.ProtocolSS, "198.51.100.91", 8388, nil)
	nodes := []domain.Node{
		{
			LogicalID:          nodeID,
			Protocol:           domain.ProtocolSS,
			DisplayName:        "Legacy Recompile Node",
			Server:             "198.51.100.91",
			Port:               8388,
			Credentials:        domain.InboundProtocolCredential{Method: "aes-128-gcm", Password: "pwd"},
			Active:             true,
			ConnectionRevision: 1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	setupSyntheticInventory(t, ctx, db, nodes)

	revRepo := sqlite.NewRevisionRepository(db)
	rev := &domain.ConfigurationRevision{
		ID:            "rev-legacy-fallback-" + domain.MustNewUUIDv7(),
		ContentDigest: "sha256:legacy-fallback-digest",
		State:         domain.RevisionStateActive,
		CreatedAt:     now,
	}
	if err := revRepo.Create(ctx, rev); err != nil {
		t.Fatalf("create active revision: %v", err)
	}
	if err := revRepo.SetActive(ctx, rev.ID); err != nil {
		t.Fatalf("set active revision: %v", err)
	}

	policyRepo := sqlite.NewPolicyRepository(db)
	factsLoader := sqlite.NewConsistentFactsLoader(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	pubRepo := sqlite.NewPublicationRepository(db)
	filterRepo := sqlite.NewNodeFilterRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)

	svc := publication.NewService(
		pubRepo,
		&dummyAuditRepo{},
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revRepo),
		publication.WithNodeRepository(nodeRepo),
		publication.WithNodeFilterRepository(filterRepo),
		publication.WithProbeObservationRepository(obsRepo),
		publication.WithNodeSourceRepository(sourceRepo),
		publication.WithConsistentFactsLoader(factsLoader),
	)

	// Persist a publication entity with Content=nil, simulating legacy data
	legacyPubID := "pub-legacy-" + domain.MustNewUUIDv7()
	legacyPub := &domain.Publication{
		ID:              legacyPubID,
		RevisionID:      rev.ID,
		Target:          domain.TargetSingBox,
		SnapshotDigest:  "sha256:legacy-snap-digest",
		ContentDigest:   "sha256:legacy-content-digest",
		ContentType:     "application/json",
		Filename:        "sing-box.json",
		Content:         nil, // Empty content triggers on-the-fly recompile fallback
		CompilerVersion: "1.0.0",
		TokenHash:       "",
		State:           domain.PublicationStateActive,
		CreatedAt:       now,
	}
	if err := pubRepo.Create(ctx, legacyPub); err != nil {
		t.Fatalf("persist legacy publication record: %v", err)
	}

	served, err := svc.ResolveAndServeAuthorized(ctx, legacyPubID, func(pubTokenHash string) bool {
		return true // isolate the recompile fallback from token generation
	})
	if err != nil {
		t.Fatalf("ResolveAndServeAuthorized failed on legacy publication fallback: %v", err)
	}
	if served == nil || len(served.Content) == 0 {
		t.Fatal("expected recompiled content from legacy publication fallback, got empty")
	}
	if !strings.Contains(string(served.Content), "Legacy Recompile Node") {
		t.Errorf("served legacy recompile content missing node: %s", string(served.Content))
	}
}
