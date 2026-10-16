package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestConsistentFactsLoader_Comprehensive(t *testing.T) {
	db, _ := setupTestDB(t)
	loader := sqlite.NewConsistentFactsLoader(db)
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Initially no active revision -> should fail with conflict
	_, err := loader.LoadConsistentFacts(ctx, "", domain.TargetMihomo, now)
	if err == nil {
		t.Fatal("expected conflict error when no active revision exists")
	}

	// 2. Insert configuration revision
	revID := "rev-facts-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-1', 'active', ?);
	`, revID, now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert revision failed: %v", err)
	}

	// 3. Insert admission rules
	_, err = db.ExecContext(ctx, `
		INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
		VALUES ('ar-1', ?, 'Allow All', 'true', 'allow', 0);
	`, revID)
	if err != nil {
		t.Fatalf("insert admission rule failed: %v", err)
	}

	// 4. Insert node groups & policy group sources
	groupID := "grp-facts-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy Group', 'select', ?, ?, 0, 1);
	`, groupID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert node group failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO policy_group_sources (id, group_id, operation, kind, position, node_logical_id, created_at, updated_at)
		VALUES ('pgs-1', ?, 'add', 'manual_node', 0, 'node-facts-1', ?, ?);
	`, groupID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert policy group source failed: %v", err)
	}

	// 5. Insert policy rules
	_, err = db.ExecContext(ctx, `
		INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES ('pr-1', ?, ?, 'MATCH', 0);
	`, revID, groupID)
	if err != nil {
		t.Fatalf("insert policy rule failed: %v", err)
	}

	// 6. Insert enabled subscription and active node
	subID := "sub-facts-1"
	fetchID := "fetch-facts-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Test Sub', 'secret://url', 1, '1', ?, ?);
	`, subID, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert subscription failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'hash-1', 1, 1);
	`, fetchID, subID, now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert subscription_fetch failed: %v", err)
	}

	nodeID := "node-facts-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'HK Node', '1.2.3.4', 8388, '{"method":"aes-128-gcm","password":"secret"}', 1, ?, ?, 1);
	`, nodeID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert node failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, nodeID, subID, fetchID)
	if err != nil {
		t.Fatalf("insert node_sources failed: %v", err)
	}

	// 7. Insert global node filter
	_, err = db.ExecContext(ctx, `
		UPDATE global_node_filters
		SET filter_spec = '{"conditions":[{"field":"display_name","op":"contains","value":"HK"}]}', updated_at = ?
		WHERE id = 1;
	`, now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("update global_node_filters failed: %v", err)
	}

	// 8. Insert active risk policy with normalized schema (000002)
	riskPolID := "rp-facts-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO risk_policy_revisions (
			id, fusion_mode, max_observation_age_seconds, minimum_confidence,
			unknown_action, conflict_action, review_action, active, created_at, activated_at, deactivated_at, rules_digest
		) VALUES (?, 'single_provider', 86400, 50, 'review', 'block', 'review', 1, ?, ?, NULL, 'digest-rp');
	`, riskPolID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert risk_policy_revision failed: %v", err)
	}

	// Provider deployment setting and policy provider
	_, err = db.ExecContext(ctx, `
		INSERT INTO ip_risk_provider_settings (
			provider, schema_version, enabled, secret_reference, max_concurrency,
			requests_per_minute, daily_request_budget, per_request_timeout_ms, max_response_bytes, created_at, updated_at
		) VALUES ('scamalytics', 'v1', 1, 'secret://test', 5, 60, 1000, 3000, 1048576, ?, ?);
	`, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("insert ip_risk_provider_settings failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO risk_policy_providers (revision_id, provider, schema_version, position)
		VALUES (?, 'scamalytics', 'v1', 0);
	`, riskPolID)
	if err != nil {
		t.Fatalf("insert risk_policy_providers failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO risk_policy_score_bands (revision_id, min_score, max_score, band, action, position)
		VALUES (?, 0, 50, 'low', 'allow', 0),
		       (?, 51, 100, 'high', 'block', 1);
	`, riskPolID, riskPolID)
	if err != nil {
		t.Fatalf("insert risk_policy_score_bands failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO risk_policy_trait_rules (revision_id, trait, action)
		VALUES (?, 'tor', 'block');
	`, riskPolID)
	if err != nil {
		t.Fatalf("insert risk_policy_trait_rules failed: %v", err)
	}

	// Insert IP risk observation for the node
	obsID := "obs-facts-1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO ip_risk_observations (
			id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
			observed_at, expires_at, status, score, confidence, network_class, anonymizer_traits, evidence_digest, redacted_summary
		) VALUES (
			?, ?, 'exit-digest-1', 'scamalytics', 'v1',
			?, ?, 'available', 10, 80, 'datacenter', '[]', 'ev-digest-1', 'Test observation'
		);
	`, obsID, nodeID, now.Format(time.RFC3339Nano), now.Add(24*time.Hour).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert ip_risk_observations failed: %v", err)
	}

	// 9. Load ConsistentFacts for TargetMihomo (includes graph)
	factsMihomo, err := loader.LoadConsistentFacts(ctx, revID, domain.TargetMihomo, now)
	if err != nil {
		t.Fatalf("LoadConsistentFacts(Mihomo) failed: %v", err)
	}

	if factsMihomo.Revision == nil || factsMihomo.Revision.ID != revID {
		t.Fatalf("expected revision %s, got %+v", revID, factsMihomo.Revision)
	}
	if len(factsMihomo.AdmissionRules) != 1 {
		t.Fatalf("expected 1 admission rule, got %d", len(factsMihomo.AdmissionRules))
	}
	if len(factsMihomo.Groups) != 1 || factsMihomo.Groups[0].ID != groupID {
		t.Fatalf("expected group %s, got %+v", groupID, factsMihomo.Groups)
	}
	if len(factsMihomo.PolicyRules) != 1 {
		t.Fatalf("expected 1 policy rule, got %d", len(factsMihomo.PolicyRules))
	}
	if factsMihomo.GlobalFilter == nil || len(factsMihomo.GlobalFilter.Conditions) != 1 {
		t.Fatalf("expected global filter loaded, got %+v", factsMihomo.GlobalFilter)
	}
	if len(factsMihomo.Nodes) != 1 || factsMihomo.Nodes[0].LogicalID != nodeID {
		t.Fatalf("expected node %s, got %+v", nodeID, factsMihomo.Nodes)
	}
	if factsMihomo.ActiveRiskPolicy == nil || factsMihomo.ActiveRiskPolicy.RevisionID != riskPolID {
		t.Fatalf("expected active risk policy %s, got %+v", riskPolID, factsMihomo.ActiveRiskPolicy)
	}
	if len(factsMihomo.ActiveRiskPolicy.ProviderSelection.Providers) != 1 {
		t.Fatalf("expected 1 provider in risk policy, got %d", len(factsMihomo.ActiveRiskPolicy.ProviderSelection.Providers))
	}
	if len(factsMihomo.ActiveRiskPolicy.ScoreBands) != 2 {
		t.Fatalf("expected 2 score bands, got %d", len(factsMihomo.ActiveRiskPolicy.ScoreBands))
	}
	if len(factsMihomo.ActiveRiskPolicy.TraitRules) != 1 {
		t.Fatalf("expected 1 trait rule, got %d", len(factsMihomo.ActiveRiskPolicy.TraitRules))
	}
	if len(factsMihomo.RiskObservations) != 1 || factsMihomo.RiskObservations[0].ID != obsID {
		t.Fatalf("expected 1 risk observation with ID %s, got %+v", obsID, factsMihomo.RiskObservations)
	}

	// 10. Load ConsistentFacts for TargetSingBox (non-Mihomo does not load graph)
	factsSingBox, err := loader.LoadConsistentFacts(ctx, revID, domain.TargetSingBox, now)
	if err != nil {
		t.Fatalf("LoadConsistentFacts(SingBox) failed: %v", err)
	}
	if len(factsSingBox.Groups) != 0 {
		t.Fatalf("expected 0 groups for non-Mihomo target, got %d", len(factsSingBox.Groups))
	}
	if len(factsSingBox.PolicyRules) != 0 {
		t.Fatalf("expected 0 policy rules for non-Mihomo target, got %d", len(factsSingBox.PolicyRules))
	}
	// But nodes, global filter, and risk facts ARE loaded for non-Mihomo
	if len(factsSingBox.Nodes) != 1 {
		t.Fatalf("expected 1 node for SingBox target, got %d", len(factsSingBox.Nodes))
	}
	if factsSingBox.ActiveRiskPolicy == nil {
		t.Fatal("expected active risk policy loaded for SingBox target")
	}
}

func TestConsistentFactsLoader_ConcurrentReads(t *testing.T) {
	db, _ := setupTestDB(t)
	loader := sqlite.NewConsistentFactsLoader(db)
	ctx := context.Background()
	now := time.Now().UTC()

	// Insert revision and node
	revID := "rev-concurrent-1"
	_, _ = db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-c1', 'active', ?);
	`, revID, now.Format(time.RFC3339Nano))

	nodeID := "node-concurrent-1"
	_, _ = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Concurrent Node', '1.1.1.1', 8388, '{"method":"aes-128-gcm","password":"p"}', 1, ?, ?, 1);
	`, nodeID, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))

	const goroutines = 10
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			facts, err := loader.LoadConsistentFacts(ctx, revID, domain.TargetSingBox, now)
			if err != nil {
				errCh <- fmt.Errorf("goroutine %d failed: %w", idx, err)
				return
			}
			if len(facts.Nodes) != 1 || facts.Nodes[0].LogicalID != nodeID {
				errCh <- fmt.Errorf("goroutine %d unexpected nodes count: %d", idx, len(facts.Nodes))
				return
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}
}

func TestConsistentFactsLoader_CrossDomainWriteTransactionConsistency(t *testing.T) {
	db, _ := setupTestDB(t)
	loader := sqlite.NewConsistentFactsLoader(db)
	ctx := context.Background()
	t0 := time.Now().UTC()

	// 1. Seed initial Version 1 across all domain tables
	revV1 := "rev-v1"
	_, err := db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-v1', 'active', ?);
	`, revV1, t0.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("seed v1 revision failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
		VALUES ('ar-v1', ?, 'Allow V1', 'true', 'allow', 0);
	`, revV1)
	if err != nil {
		t.Fatalf("seed v1 admission rule failed: %v", err)
	}

	grpV1 := "grp-v1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy V1', 'select', ?, ?, 0, 1);
	`, grpV1, t0.Format(time.RFC3339), t0.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed v1 group failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES ('pr-v1', ?, ?, 'MATCH', 0);
	`, revV1, grpV1)
	if err != nil {
		t.Fatalf("seed v1 policy rule failed: %v", err)
	}

	subID := "sub-v1"
	fetchID := "fetch-v1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Sub V1', 'secret://url', 1, '1', ?, ?);
	`, subID, t0.Format(time.RFC3339), t0.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed v1 subscription failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'hash-v1', 1, 1);
	`, fetchID, subID, t0.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("seed v1 fetch failed: %v", err)
	}

	nodeID := "node_0123456789abcdef"
	_, err = db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Node V1', '1.1.1.1', 8388, '{"method":"aes-128-gcm","password":"p"}', 1, ?, ?, 1);
	`, nodeID, t0.Format(time.RFC3339Nano), t0.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("seed v1 node failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, nodeID, subID, fetchID)
	if err != nil {
		t.Fatalf("seed v1 node source failed: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		UPDATE global_node_filters
		SET filter_spec = '{"conditions":[{"field":"display_name","op":"contains","value":"V1"}]}', updated_at = ?
		WHERE id = 1;
	`, t0.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed v1 filter failed: %v", err)
	}

	rpV1 := "rp-v1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO risk_policy_revisions (
			id, fusion_mode, max_observation_age_seconds, minimum_confidence,
			unknown_action, conflict_action, review_action, active, created_at, activated_at, rules_digest
		) VALUES (?, 'single_provider', 1111, 50, 'review', 'block', 'review', 1, ?, ?, 'digest-rp-v1');
	`, rpV1, t0.Format(time.RFC3339Nano), t0.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("seed v1 risk policy failed: %v", err)
	}

	// 2. Concurrency barrier: readers continuously read, writer executes atomic cross-domain transition to V2
	const numReaders = 6
	const readsPerReader = 10

	var v1Seen atomic.Int64
	var v2Seen atomic.Int64

	writerStart := make(chan struct{})
	writerDone := make(chan struct{})
	errCh := make(chan error, numReaders*readsPerReader*2+20)
	var readersWg sync.WaitGroup
	var preTransitionWg sync.WaitGroup

	for r := 0; r < numReaders; r++ {
		readersWg.Add(1)
		preTransitionWg.Add(1)
		go func(readerIdx int) {
			defer readersWg.Done()

			// Pre-transition reads: must observe V1
			for i := 0; i < readsPerReader/2; i++ {
				facts, err := loader.LoadConsistentFacts(ctx, "", domain.TargetMihomo, t0)
				if err != nil {
					errCh <- fmt.Errorf("reader %d pre-transition iteration %d failed: %w", readerIdx, i, err)
					preTransitionWg.Done()
					return
				}
				if facts.Revision.ID != "rev-v1" {
					errCh <- fmt.Errorf("expected rev-v1 before transition, got %s", facts.Revision.ID)
					preTransitionWg.Done()
					return
				}
				if len(facts.AdmissionRules) != 1 || facts.AdmissionRules[0].Name != "Allow V1" {
					errCh <- fmt.Errorf("inconsistent V1 state: admission rule mismatch: %+v", facts.AdmissionRules)
					preTransitionWg.Done()
					return
				}
				if len(facts.Nodes) != 1 || facts.Nodes[0].DisplayName != "Node V1" {
					errCh <- fmt.Errorf("inconsistent V1 state: node mismatch: %+v", facts.Nodes)
					preTransitionWg.Done()
					return
				}
				if facts.GlobalFilter == nil || len(facts.GlobalFilter.Conditions) != 1 || facts.GlobalFilter.Conditions[0].Value != "V1" {
					errCh <- fmt.Errorf("inconsistent V1 state: filter mismatch: %+v", facts.GlobalFilter)
					preTransitionWg.Done()
					return
				}
				if facts.ActiveRiskPolicy == nil || facts.ActiveRiskPolicy.RevisionID != "rp-v1" {
					errCh <- fmt.Errorf("inconsistent V1 state: risk policy mismatch: %+v", facts.ActiveRiskPolicy)
					preTransitionWg.Done()
					return
				}
				v1Seen.Add(1)
			}

			// Signal completion of pre-transition phase
			preTransitionWg.Done()

			// Wait for writer to complete atomic transaction
			<-writerDone

			// Post-transition reads: must observe V2 cleanly, never partial/hybrid
			for i := 0; i < readsPerReader/2; i++ {
				facts, err := loader.LoadConsistentFacts(ctx, "", domain.TargetMihomo, t0)
				if err != nil {
					errCh <- fmt.Errorf("reader %d post-transition iteration %d failed: %w", readerIdx, i, err)
					return
				}
				if facts.Revision.ID != "rev-v2" {
					errCh <- fmt.Errorf("expected rev-v2 after transition, got %s", facts.Revision.ID)
					return
				}
				if len(facts.AdmissionRules) != 1 || facts.AdmissionRules[0].Name != "Allow V2" {
					errCh <- fmt.Errorf("inconsistent V2 state: admission rule mismatch: %+v", facts.AdmissionRules)
					return
				}
				if len(facts.Nodes) != 1 || facts.Nodes[0].DisplayName != "Node V2" {
					errCh <- fmt.Errorf("inconsistent V2 state: node mismatch: %+v", facts.Nodes)
					return
				}
				if facts.GlobalFilter == nil || len(facts.GlobalFilter.Conditions) != 1 || facts.GlobalFilter.Conditions[0].Value != "V2" {
					errCh <- fmt.Errorf("inconsistent V2 state: filter mismatch: %+v", facts.GlobalFilter)
					return
				}
				if facts.ActiveRiskPolicy == nil || facts.ActiveRiskPolicy.RevisionID != "rp-v2" {
					errCh <- fmt.Errorf("inconsistent V2 state: risk policy mismatch: %+v", facts.ActiveRiskPolicy)
					return
				}
				v2Seen.Add(1)
			}
		}(r)
	}

	// Coordinator: Wait for all readers to complete V1 checks before releasing writer
	go func() {
		preTransitionWg.Wait()
		close(writerStart)
	}()

	// Writer transitions all domains to V2 in one atomic transaction, checking EVERY statement error
	var writerWg sync.WaitGroup
	writerWg.Add(1)
	go func() {
		defer writerWg.Done()
		<-writerStart

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			errCh <- fmt.Errorf("writer BeginTx failed: %w", err)
			return
		}
		defer tx.Rollback()

		t1 := t0.Add(1 * time.Minute)
		revV2 := "rev-v2"

		// 1. Archive v1 revision
		if _, err := tx.ExecContext(ctx, `UPDATE configuration_revisions SET state = 'archived' WHERE id = 'rev-v1';`); err != nil {
			errCh <- fmt.Errorf("archive v1 revision failed: %w", err)
			return
		}

		// 2. Insert v2 active revision
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
			VALUES (?, 'rev-v1', 'digest-v2', 'active', ?);
		`, revV2, t1.Format(time.RFC3339Nano)); err != nil {
			errCh <- fmt.Errorf("insert v2 revision failed: %w", err)
			return
		}

		// 3. Insert v2 admission rule
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
			VALUES ('ar-v2', ?, 'Allow V2', 'true', 'allow', 0);
		`, revV2); err != nil {
			errCh <- fmt.Errorf("insert v2 admission rule failed: %w", err)
			return
		}

		// 4. Insert v2 group & policy rule
		grpV2 := "grp-v2"
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
			VALUES (?, 'Proxy V2', 'select', ?, ?, 0, 1);
		`, grpV2, t1.Format(time.RFC3339), t1.Format(time.RFC3339)); err != nil {
			errCh <- fmt.Errorf("insert v2 node group failed: %w", err)
			return
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
			VALUES ('pr-v2', ?, ?, 'MATCH', 0);
		`, revV2, grpV2); err != nil {
			errCh <- fmt.Errorf("insert v2 policy rule failed: %w", err)
			return
		}

		// 5. Update node display name to Node V2
		if _, err := tx.ExecContext(ctx, `UPDATE nodes SET display_name = 'Node V2', updated_at = ? WHERE logical_id = ?;`, t1.Format(time.RFC3339Nano), nodeID); err != nil {
			errCh <- fmt.Errorf("update node to v2 failed: %w", err)
			return
		}

		// 6. Update global filter to V2
		if _, err := tx.ExecContext(ctx, `UPDATE global_node_filters SET filter_spec = '{"conditions":[{"field":"display_name","op":"contains","value":"V2"}]}', updated_at = ? WHERE id = 1;`, t1.Format(time.RFC3339)); err != nil {
			errCh <- fmt.Errorf("update global filter to v2 failed: %w", err)
			return
		}

		// 7. Deactivate rp-v1 and activate rp-v2
		if _, err := tx.ExecContext(ctx, `UPDATE risk_policy_revisions SET active = 0 WHERE id = 'rp-v1';`); err != nil {
			errCh <- fmt.Errorf("deactivate rp-v1 failed: %w", err)
			return
		}
		rpV2 := "rp-v2"
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO risk_policy_revisions (
				id, fusion_mode, max_observation_age_seconds, minimum_confidence,
				unknown_action, conflict_action, review_action, active, created_at, activated_at, rules_digest
			) VALUES (?, 'single_provider', 2222, 50, 'review', 'block', 'review', 1, ?, ?, 'digest-rp-v2');
		`, rpV2, t1.Format(time.RFC3339Nano), t1.Format(time.RFC3339Nano)); err != nil {
			errCh <- fmt.Errorf("insert rp-v2 failed: %w", err)
			return
		}

		// Commit transaction
		if err := tx.Commit(); err != nil {
			errCh <- fmt.Errorf("writer commit failed: %w", err)
			return
		}
		close(writerDone)
	}()

	writerWg.Wait()
	readersWg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

	if v1Seen.Load() == 0 {
		t.Fatal("expected at least one reader to observe V1 state before transition")
	}
	if v2Seen.Load() == 0 {
		t.Fatal("expected at least one reader to observe V2 state after transition")
	}
}

func TestConsistentFactsLoader_MidTransactionConcurrentWriteRepeatableRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbPath := filepath.Join(t.TempDir(), "consistent_facts_wal.db")
	cfg := sqlite.Config{
		Path:         dbPath,
		BusyTimeout:  5 * time.Second,
		ForeignKeys:  true,
		WALMode:      true,
		MaxOpenConns: 5,
		MaxIdleConns: 5,
	}
	db, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("OpenAndMigrate file-backed WAL DB failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	loader := sqlite.NewConsistentFactsLoader(db)
	t0 := time.Now().UTC()

	// 1. Seed initial V1 facts
	revV1 := "rev-tx-v1"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, NULL, 'digest-tx-v1', 'active', ?);
	`, revV1, t0.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed revision failed: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
		VALUES ('ar-tx-v1', ?, 'Allow V1', 'true', 'allow', 0);
	`, revV1); err != nil {
		t.Fatalf("seed admission rule failed: %v", err)
	}

	grpV1 := "grp-tx-v1"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy V1', 'select', ?, ?, 0, 1);
	`, grpV1, t0.Format(time.RFC3339), t0.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed node group failed: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES ('pr-tx-v1', ?, ?, 'MATCH', 0);
	`, revV1, grpV1); err != nil {
		t.Fatalf("seed policy rule failed: %v", err)
	}

	subID := "sub-tx-v1"
	fetchID := "fetch-tx-v1"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO subscriptions (id, name, source_url_secret_ref, enabled, revision, created_at, updated_at)
		VALUES (?, 'Sub V1', 'secret://url', 1, '1', ?, ?);
	`, subID, t0.Format(time.RFC3339), t0.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed subscription failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO subscription_fetches (id, subscription_id, started_at, outcome, content_digest, nodes_parsed, nodes_valid)
		VALUES (?, ?, ?, 'success', 'hash-v1', 1, 1);
	`, fetchID, subID, t0.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed fetch failed: %v", err)
	}

	nodeID := "node_0123456789abcdef"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, 'ss', 'Node V1', '1.1.1.1', 8388, '{"method":"aes-128-gcm","password":"p"}', 1, ?, ?, 1);
	`, nodeID, t0.Format(time.RFC3339Nano), t0.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed node failed: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?);
	`, nodeID, subID, fetchID); err != nil {
		t.Fatalf("seed node source failed: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		UPDATE global_node_filters
		SET filter_spec = '{"conditions":[{"field":"display_name","op":"contains","value":"V1"}]}', updated_at = ?
		WHERE id = 1;
	`, t0.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed filter failed: %v", err)
	}

	rpV1 := "rp-tx-v1"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO risk_policy_revisions (
			id, fusion_mode, max_observation_age_seconds, minimum_confidence,
			unknown_action, conflict_action, review_action, active, created_at, activated_at, rules_digest
		) VALUES (?, 'single_provider', 1111, 50, 'review', 'block', 'review', 1, ?, ?, 'digest-rp-tx-v1');
	`, rpV1, t0.Format(time.RFC3339Nano), t0.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("seed risk policy failed: %v", err)
	}

	// 2. Open a dedicated read-only transaction for the reader
	readerTx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("reader BeginTx failed: %v", err)
	}
	defer readerTx.Rollback()

	// Step A: Reader reads domain 1 (Revision) from its transaction
	revRead, err := sqlite.LoadRevisionInternal(ctx, readerTx, "")
	if err != nil {
		t.Fatalf("reader step A load revision failed: %v", err)
	}
	if revRead.ID != "rev-tx-v1" {
		t.Fatalf("reader step A expected rev-tx-v1, got %s", revRead.ID)
	}

	// Step B: Concurrently on another connection/transaction, writer completely modifies and commits V2 across all domains
	writerTx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("writer BeginTx failed: %v", err)
	}
	defer writerTx.Rollback()

	t1 := t0.Add(1 * time.Minute)
	revV2 := "rev-tx-v2"

	if _, err := writerTx.ExecContext(ctx, `UPDATE configuration_revisions SET state = 'archived' WHERE id = 'rev-tx-v1';`); err != nil {
		t.Fatalf("writer archive v1 failed: %v", err)
	}
	if _, err := writerTx.ExecContext(ctx, `
		INSERT INTO configuration_revisions (id, parent_id, content_digest, state, created_at)
		VALUES (?, 'rev-tx-v1', 'digest-tx-v2', 'active', ?);
	`, revV2, t1.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("writer insert v2 revision failed: %v", err)
	}
	if _, err := writerTx.ExecContext(ctx, `
		INSERT INTO admission_rules (id, revision_id, name, expression, action, position)
		VALUES ('ar-tx-v2', ?, 'Allow V2', 'true', 'allow', 0);
	`, revV2); err != nil {
		t.Fatalf("writer insert v2 admission rule failed: %v", err)
	}
	grpV2 := "grp-tx-v2"
	if _, err := writerTx.ExecContext(ctx, `
		INSERT INTO node_groups (id, name, group_type, created_at, updated_at, empty_fallback_pass, revision)
		VALUES (?, 'Proxy V2', 'select', ?, ?, 0, 1);
	`, grpV2, t1.Format(time.RFC3339), t1.Format(time.RFC3339)); err != nil {
		t.Fatalf("writer insert v2 group failed: %v", err)
	}
	if _, err := writerTx.ExecContext(ctx, `
		INSERT INTO policy_rules (id, revision_id, target_group_id, expression, position)
		VALUES ('pr-tx-v2', ?, ?, 'MATCH', 0);
	`, revV2, grpV2); err != nil {
		t.Fatalf("writer insert v2 policy rule failed: %v", err)
	}
	if _, err := writerTx.ExecContext(ctx, `UPDATE nodes SET display_name = 'Node V2', updated_at = ? WHERE logical_id = ?;`, t1.Format(time.RFC3339Nano), nodeID); err != nil {
		t.Fatalf("writer update node to v2 failed: %v", err)
	}
	if _, err := writerTx.ExecContext(ctx, `UPDATE global_node_filters SET filter_spec = '{"conditions":[{"field":"display_name","op":"contains","value":"V2"}]}', updated_at = ? WHERE id = 1;`, t1.Format(time.RFC3339)); err != nil {
		t.Fatalf("writer update filter to v2 failed: %v", err)
	}
	if _, err := writerTx.ExecContext(ctx, `UPDATE risk_policy_revisions SET active = 0 WHERE id = 'rp-tx-v1';`); err != nil {
		t.Fatalf("writer deactivate rp-tx-v1 failed: %v", err)
	}
	rpV2 := "rp-tx-v2"
	if _, err := writerTx.ExecContext(ctx, `
		INSERT INTO risk_policy_revisions (
			id, fusion_mode, max_observation_age_seconds, minimum_confidence,
			unknown_action, conflict_action, review_action, active, created_at, activated_at, rules_digest
		) VALUES (?, 'single_provider', 2222, 50, 'review', 'block', 'review', 1, ?, ?, 'digest-rp-tx-v2');
	`, rpV2, t1.Format(time.RFC3339Nano), t1.Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("writer insert rp-tx-v2 failed: %v", err)
	}

	// Commit writer's transaction completely
	if err := writerTx.Commit(); err != nil {
		t.Fatalf("writer Commit failed: %v", err)
	}

	// Step C: Reader continues reading remaining domains (Rules, Nodes, Filters, Risk) from the ALREADY OPEN readerTx
	// In SQLite WAL mode, readerTx has snapshot isolation at start of read transaction.
	// Every subsequent query within readerTx MUST strictly return V1 data, proving ZERO partial/hybrid leak!
	factsFromTx, err := sqlite.LoadConsistentFactsFromTx(ctx, readerTx, revRead.ID, domain.TargetMihomo, t0)
	if err != nil {
		t.Fatalf("reader step C LoadConsistentFactsFromTx failed: %v", err)
	}

	if factsFromTx.Revision.ID != "rev-tx-v1" {
		t.Fatalf("reader step C violated repeatable read: got revision %s", factsFromTx.Revision.ID)
	}
	if len(factsFromTx.AdmissionRules) != 1 || factsFromTx.AdmissionRules[0].Name != "Allow V1" {
		t.Fatalf("reader step C violated repeatable read: got admission rule %+v", factsFromTx.AdmissionRules)
	}
	if len(factsFromTx.Nodes) != 1 || factsFromTx.Nodes[0].DisplayName != "Node V1" {
		t.Fatalf("reader step C violated repeatable read: got node display name %s", factsFromTx.Nodes[0].DisplayName)
	}
	if factsFromTx.GlobalFilter == nil || len(factsFromTx.GlobalFilter.Conditions) != 1 || factsFromTx.GlobalFilter.Conditions[0].Value != "V1" {
		t.Fatalf("reader step C violated repeatable read: got filter %+v", factsFromTx.GlobalFilter)
	}
	if factsFromTx.ActiveRiskPolicy == nil || factsFromTx.ActiveRiskPolicy.RevisionID != "rp-tx-v1" {
		t.Fatalf("reader step C violated repeatable read: got risk policy %+v", factsFromTx.ActiveRiskPolicy)
	}

	// Close readerTx
	_ = readerTx.Rollback()

	// Step D: A fresh LoadConsistentFacts started AFTER writer commit MUST load complete V2 across all domains
	factsFresh, err := loader.LoadConsistentFacts(ctx, "", domain.TargetMihomo, t1)
	if err != nil {
		t.Fatalf("fresh loader after commit failed: %v", err)
	}
	if factsFresh.Revision.ID != "rev-tx-v2" {
		t.Fatalf("fresh loader expected rev-tx-v2, got %s", factsFresh.Revision.ID)
	}
	if len(factsFresh.AdmissionRules) != 1 || factsFresh.AdmissionRules[0].Name != "Allow V2" {
		t.Fatalf("fresh loader expected Allow V2, got %+v", factsFresh.AdmissionRules)
	}
	if len(factsFresh.Nodes) != 1 || factsFresh.Nodes[0].DisplayName != "Node V2" {
		t.Fatalf("fresh loader expected Node V2, got %+v", factsFresh.Nodes)
	}
	if factsFresh.GlobalFilter == nil || len(factsFresh.GlobalFilter.Conditions) != 1 || factsFresh.GlobalFilter.Conditions[0].Value != "V2" {
		t.Fatalf("fresh loader expected filter V2, got %+v", factsFresh.GlobalFilter)
	}
	if factsFresh.ActiveRiskPolicy == nil || factsFresh.ActiveRiskPolicy.RevisionID != "rp-tx-v2" {
		t.Fatalf("fresh loader expected rp-tx-v2, got %+v", factsFresh.ActiveRiskPolicy)
	}
}
