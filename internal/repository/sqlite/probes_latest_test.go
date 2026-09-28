package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestListLatestByNodes_WindowRankingTieBreakChunkingAndQueryPlan(t *testing.T) {
	ctx := context.Background()
	db, _ := setupTestDB(t)
	defer db.Close()

	runRepo := sqlite.NewProbeRunRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)

	// 1. Verify idempotent composite index idx_probe_obs_node_kind_observed_id exists on probe_observations
	var idxCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'index'
		  AND tbl_name = 'probe_observations'
		  AND name = 'idx_probe_obs_node_kind_observed_id';
	`).Scan(&idxCount); err != nil {
		t.Fatalf("failed to query sqlite_master for idx_probe_obs_node_kind_observed_id: %v", err)
	}
	if idxCount != 1 {
		t.Fatalf("expected idx_probe_obs_node_kind_observed_id index to exist, got count=%d", idxCount)
	}

	// 2. Empty input returns empty map without error
	emptyNil, err := obsRepo.ListLatestByNodes(ctx, nil, nil)
	if err != nil {
		t.Fatalf("ListLatestByNodes(nil, nil) failed: %v", err)
	}
	if len(emptyNil) != 0 {
		t.Fatalf("expected empty map for nil nodeLogicalIDs, got %d entries", len(emptyNil))
	}

	emptySlice, err := obsRepo.ListLatestByNodes(ctx, []string{}, []domain.ProbeKind{domain.ProbeKindBaseline})
	if err != nil {
		t.Fatalf("ListLatestByNodes([], [baseline]) failed: %v", err)
	}
	if len(emptySlice) != 0 {
		t.Fatalf("expected empty map for empty nodeLogicalIDs, got %d entries", len(emptySlice))
	}

	// 3. Seed 235 nodes (> 100 chunk boundary: 100 + 100 + 35) and multiple probe runs
	baseTime := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)
	const totalNodes = 235
	nodes := make([]domain.Node, 0, totalNodes)
	nodeIDs := make([]string, 0, totalNodes)
	for i := 1; i <= totalNodes; i++ {
		nid := fmt.Sprintf("node-latest-%03d", i)
		nodeIDs = append(nodeIDs, nid)
		nodes = append(nodes, domain.Node{
			LogicalID:   nid,
			Protocol:    domain.ProtocolSS,
			DisplayName: fmt.Sprintf("LatestNode-%03d", i),
			Server:      "198.51.100.10",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		})
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		t.Fatalf("UpsertBatch nodes failed: %v", err)
	}

	runID := "0195c800-0000-7000-8000-000000000001"
	if err := runRepo.Create(ctx, &domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "idem-latest-test",
		ActorScope:     "test",
		ConfigRevision: "rev-1",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     baseTime.Add(time.Hour),
		CreatedAt:      baseTime,
		UpdatedAt:      baseTime,
	}); err != nil {
		t.Fatalf("Create probe run failed: %v", err)
	}

	kinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindGeo,
		domain.ProbeKindSpeed,
		domain.ProbeKindStreaming,
	}

	// Seed deep history + same-second tie-break observations inside a transaction for speed
	expectedID := make(map[string]map[domain.ProbeKind]string, totalNodes)
	expectedLatency := make(map[string]map[domain.ProbeKind]int64, totalNodes)

	err = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO probe_observations (
				id, probe_run_id, node_logical_id, kind, verdict,
				evidence_digest, observed_at, latency_ms, redacted_summary
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for i, nid := range nodeIDs {
			expectedID[nid] = make(map[domain.ProbeKind]string, len(kinds))
			expectedLatency[nid] = make(map[domain.ProbeKind]int64, len(kinds))

			// Leave last 5 nodes with zero observations to verify empty map preservation across chunks
			if i >= totalNodes-5 {
				continue
			}

			for kIdx, kind := range kinds {
				// 6 older historical observations per (node, kind)
				// Give older observations a lexicographically huge ID ("zzzz-old-...") to prove observed_at DESC takes precedence over id DESC.
				for h := 6; h >= 1; h-- {
					histAt := baseTime.Add(-time.Duration(h*10) * time.Minute).Format(time.RFC3339)
					obsID := fmt.Sprintf("zzzz-old-%s-%s-h%02d", nid, kind, h)
					if _, err := stmt.ExecContext(ctx,
						obsID, runID, nid, string(kind), string(domain.VerdictError),
						"digest-old", histAt, 900+h, "historical",
					); err != nil {
						return err
					}
				}

				latestAt := baseTime.Add(time.Duration(kIdx) * time.Minute).Format(time.RFC3339)
				if (i+kIdx)%2 == 0 {
					// Same-second tie-break scenario: insert multiple rows with identical observed_at in non-sorted order.
					// Winner must be "-tie-z" (highest lexicographical ID).
					tieCandidates := []struct {
						suffix  string
						verdict domain.ProbeVerdict
						latency int64
					}{
						{suffix: "tie-m", verdict: domain.VerdictUnknown, latency: 210},
						{suffix: "tie-z", verdict: domain.VerdictAvailable, latency: int64(42 + (i % 50))},
						{suffix: "tie-a", verdict: domain.VerdictRestricted, latency: 330},
					}
					for _, tc := range tieCandidates {
						obsID := fmt.Sprintf("obs-%s-%s-%s", nid, kind, tc.suffix)
						if _, err := stmt.ExecContext(ctx,
							obsID, runID, nid, string(kind), string(tc.verdict),
							"digest-"+tc.suffix, latestAt, tc.latency, "summary-"+tc.suffix,
						); err != nil {
							return err
						}
					}
					expectedID[nid][kind] = fmt.Sprintf("obs-%s-%s-tie-z", nid, kind)
					expectedLatency[nid][kind] = int64(42 + (i % 50))
				} else {
					// Distinct latest timestamp scenario
					winnerID := fmt.Sprintf("aaaa-latest-%s-%s", nid, kind)
					winLatency := int64(55 + (i % 40))
					if _, err := stmt.ExecContext(ctx,
						winnerID, runID, nid, string(kind), string(domain.VerdictAvailable),
						"digest-latest", latestAt, winLatency, "summary-latest",
					); err != nil {
						return err
					}
					expectedID[nid][kind] = winnerID
					expectedLatency[nid][kind] = winLatency
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to seed observations: %v", err)
	}

	// 4. Verify ListLatestByNodes across >100 node chunks with nil kinds (all kinds)
	queryNodeIDs := append(append([]string{}, nodeIDs...), "node-nonexistent-999")
	allLatest, err := obsRepo.ListLatestByNodes(ctx, queryNodeIDs, nil)
	if err != nil {
		t.Fatalf("ListLatestByNodes(all) failed: %v", err)
	}
	if len(allLatest) != len(queryNodeIDs) {
		t.Fatalf("expected %d node keys in result, got %d", len(queryNodeIDs), len(allLatest))
	}

	for i, nid := range nodeIDs {
		byKind, ok := allLatest[nid]
		if !ok || byKind == nil {
			t.Fatalf("missing result map for node %s", nid)
		}
		if i >= totalNodes-5 {
			if len(byKind) != 0 {
				t.Fatalf("expected 0 observations for unseeded node %s, got %d", nid, len(byKind))
			}
			continue
		}
		if len(byKind) != len(kinds) {
			t.Fatalf("node %s expected %d kinds, got %d", nid, len(kinds), len(byKind))
		}
		for _, kind := range kinds {
			gotObs, exists := byKind[kind]
			if !exists {
				t.Fatalf("node %s missing kind %s", nid, kind)
			}
			wantID := expectedID[nid][kind]
			wantLatency := expectedLatency[nid][kind]
			if gotObs.ID != wantID {
				t.Fatalf("node %s kind %s: expected latest ID %q, got %q", nid, kind, wantID, gotObs.ID)
			}
			if gotObs.LatencyMS != wantLatency {
				t.Fatalf("node %s kind %s: expected latency %d, got %d", nid, kind, wantLatency, gotObs.LatencyMS)
			}
			if gotObs.Verdict != domain.VerdictAvailable {
				t.Fatalf("node %s kind %s: expected verdict available, got %s", nid, kind, gotObs.Verdict)
			}
		}
	}
	if len(allLatest["node-nonexistent-999"]) != 0 {
		t.Fatalf("expected empty map for nonexistent node, got %+v", allLatest["node-nonexistent-999"])
	}

	// 5. Verify ListLatestByNodes with subset of kinds across chunks
	subsetKinds := []domain.ProbeKind{domain.ProbeKindBaseline, domain.ProbeKindSpeed}
	subsetLatest, err := obsRepo.ListLatestByNodes(ctx, queryNodeIDs, subsetKinds)
	if err != nil {
		t.Fatalf("ListLatestByNodes(subset) failed: %v", err)
	}
	for i, nid := range nodeIDs {
		byKind := subsetLatest[nid]
		if i >= totalNodes-5 {
			if len(byKind) != 0 {
				t.Fatalf("expected 0 observations for unseeded node %s, got %d", nid, len(byKind))
			}
			continue
		}
		if len(byKind) != len(subsetKinds) {
			t.Fatalf("node %s expected %d filtered kinds, got %d", nid, len(subsetKinds), len(byKind))
		}
		for _, kind := range subsetKinds {
			if byKind[kind].ID != expectedID[nid][kind] {
				t.Fatalf("node %s kind %s: expected ID %q, got %q", nid, kind, expectedID[nid][kind], byKind[kind].ID)
			}
		}
		if _, hasGeo := byKind[domain.ProbeKindGeo]; hasGeo {
			t.Fatalf("node %s should not contain filtered-out geo observation", nid)
		}
	}

	// 6. Verify DB query returns bounded rows (only 1 row per (node, kind)) and uses composite index in EXPLAIN QUERY PLAN
	t.Run("bounded_rows_and_explain_query_plan", func(t *testing.T) {
		var totalStoredRows int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM probe_observations;").Scan(&totalStoredRows); err != nil {
			t.Fatalf("count probe_observations: %v", err)
		}
		if totalStoredRows <= (totalNodes-5)*len(kinds) {
			t.Fatalf("expected deep history rows > %d, got %d", (totalNodes-5)*len(kinds), totalStoredRows)
		}

		// Verify a 100-node chunk window query returns at most 100 * len(subsetKinds) rows from SQLite
		chunkNodes := nodeIDs[:100]
		placeholders := make([]string, len(chunkNodes))
		args := make([]any, 0, len(chunkNodes)+len(subsetKinds))
		for i, nid := range chunkNodes {
			placeholders[i] = "?"
			args = append(args, nid)
		}
		kindPlaceholders := []string{"?", "?"}
		args = append(args, string(domain.ProbeKindBaseline), string(domain.ProbeKindSpeed))

		windowSQL := fmt.Sprintf(`
		WITH ranked_observations AS (
			SELECT id, probe_run_id, node_logical_id, kind, verdict,
			       evidence_digest, observed_at, latency_ms, redacted_summary,
			       ROW_NUMBER() OVER (
			           PARTITION BY node_logical_id, kind
			           ORDER BY observed_at DESC, id DESC
			       ) AS rn
			FROM probe_observations
			WHERE node_logical_id IN (%s) AND kind IN (%s)
		)
		SELECT id, probe_run_id, node_logical_id, kind, verdict,
		       evidence_digest, observed_at, latency_ms, redacted_summary
		FROM ranked_observations
		WHERE rn = 1;`, strings.Join(placeholders, ", "), strings.Join(kindPlaceholders, ", "))

		rows, err := db.QueryContext(ctx, windowSQL, args...)
		if err != nil {
			t.Fatalf("window query failed: %v", err)
		}
		returnedRows := 0
		for rows.Next() {
			returnedRows++
		}
		rows.Close()

		wantRows := len(chunkNodes) * len(subsetKinds)
		if returnedRows != wantRows {
			t.Fatalf("expected SQLite window query to return exactly %d latest rows (1 per node+kind), got %d", wantRows, returnedRows)
		}

		// Verify EXPLAIN QUERY PLAN uses composite index on probe_observations and avoids full table scan
		explainRows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+windowSQL, args...)
		if err != nil {
			t.Fatalf("EXPLAIN QUERY PLAN failed: %v", err)
		}
		defer explainRows.Close()

		var planDetails []string
		for explainRows.Next() {
			var id, parent, notused int
			var detail string
			if err := explainRows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatalf("scan explain row: %v", err)
			}
			planDetails = append(planDetails, detail)
		}
		planText := strings.Join(planDetails, " | ")
		if !strings.Contains(planText, "idx_probe_obs_node_kind_observed_id") && !strings.Contains(planText, "idx_probe_obs_latest_lookup") {
			t.Fatalf("expected EXPLAIN QUERY PLAN to use composite (node_logical_id, kind, observed_at DESC, id DESC) index, got: %s", planText)
		}
		if strings.Contains(planText, "SCAN probe_observations") {
			t.Fatalf("expected EXPLAIN QUERY PLAN to avoid full table scan on probe_observations, got: %s", planText)
		}
		if strings.Contains(planText, "USE TEMP B-TREE") {
			t.Fatalf("expected EXPLAIN QUERY PLAN to avoid temp B-tree sort for window partition/order, got: %s", planText)
		}

		// Also verify EXPLAIN QUERY PLAN when kinds filter is omitted (all kinds)
		allKindsSQL := fmt.Sprintf(`
		WITH ranked_observations AS (
			SELECT id, probe_run_id, node_logical_id, kind, verdict,
			       evidence_digest, observed_at, latency_ms, redacted_summary,
			       ROW_NUMBER() OVER (
			           PARTITION BY node_logical_id, kind
			           ORDER BY observed_at DESC, id DESC
			       ) AS rn
			FROM probe_observations
			WHERE node_logical_id IN (%s)
		)
		SELECT id, probe_run_id, node_logical_id, kind, verdict,
		       evidence_digest, observed_at, latency_ms, redacted_summary
		FROM ranked_observations
		WHERE rn = 1;`, strings.Join(placeholders, ", "))

		explainAllRows, err := db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+allKindsSQL, args[:len(chunkNodes)]...)
		if err != nil {
			t.Fatalf("EXPLAIN QUERY PLAN (all kinds) failed: %v", err)
		}
		defer explainAllRows.Close()

		var allPlanDetails []string
		for explainAllRows.Next() {
			var id, parent, notused int
			var detail string
			if err := explainAllRows.Scan(&id, &parent, &notused, &detail); err != nil {
				t.Fatalf("scan explain (all kinds) row: %v", err)
			}
			allPlanDetails = append(allPlanDetails, detail)
		}
		allPlanText := strings.Join(allPlanDetails, " | ")
		if !strings.Contains(allPlanText, "idx_probe_obs_node_kind_observed_id") && !strings.Contains(allPlanText, "idx_probe_obs_latest_lookup") {
			t.Fatalf("expected all-kinds EXPLAIN QUERY PLAN to use composite index, got: %s", allPlanText)
		}
		if strings.Contains(allPlanText, "SCAN probe_observations") {
			t.Fatalf("expected all-kinds EXPLAIN QUERY PLAN to avoid full table scan, got: %s", allPlanText)
		}
	})
}

func BenchmarkListLatestByNodes(b *testing.B) {
	ctx := context.Background()
	cfg := sqlite.Config{
		Path:        fmt.Sprintf("file:bench_latest_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
		WALMode:     false,
	}
	db, err := sqlite.Open(cfg)
	if err != nil {
		b.Fatalf("failed to open benchmark sqlite db: %v", err)
	}
	defer db.Close()

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		b.Fatalf("failed to apply migrations on benchmark db: %v", err)
	}

	runRepo := sqlite.NewProbeRunRepository(db)
	obsRepo := sqlite.NewProbeObservationRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)

	baseTime := time.Date(2026, 9, 28, 3, 0, 0, 0, time.UTC)
	const (
		benchNodes     = 150
		historyPerKind = 25
	)
	kinds := []domain.ProbeKind{
		domain.ProbeKindBaseline,
		domain.ProbeKindGeo,
		domain.ProbeKindSpeed,
		domain.ProbeKindStreaming,
	}

	nodes := make([]domain.Node, 0, benchNodes)
	nodeIDs := make([]string, 0, benchNodes)
	for i := 1; i <= benchNodes; i++ {
		nid := fmt.Sprintf("bench-node-%03d", i)
		nodeIDs = append(nodeIDs, nid)
		nodes = append(nodes, domain.Node{
			LogicalID:   nid,
			Protocol:    domain.ProtocolSS,
			DisplayName: fmt.Sprintf("BenchNode-%03d", i),
			Server:      "198.51.100.20",
			Port:        8388,
			Credentials: domain.InboundProtocolCredential{Method: "aes-256-gcm", Password: "secret"},
			Active:      true,
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
		})
	}
	if err := nodeRepo.UpsertBatch(ctx, nodes); err != nil {
		b.Fatalf("UpsertBatch failed: %v", err)
	}

	runID := "0195c800-0000-7000-8000-000000000099"
	if err := runRepo.Create(ctx, &domain.ProbeRun{
		ID:             runID,
		IdempotencyKey: "idem-bench-latest",
		ActorScope:     "bench",
		ConfigRevision: "rev-bench",
		State:          domain.ProbeRunStateSucceeded,
		DeadlineAt:     baseTime.Add(time.Hour),
		CreatedAt:      baseTime,
		UpdatedAt:      baseTime,
	}); err != nil {
		b.Fatalf("Create probe run failed: %v", err)
	}

	// Seed 150 nodes * 4 kinds * 25 historical observations = 15,000 rows
	err = sqlite.WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO probe_observations (
				id, probe_run_id, node_logical_id, kind, verdict,
				evidence_digest, observed_at, latency_ms, redacted_summary
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, nid := range nodeIDs {
			for _, kind := range kinds {
				for h := 0; h < historyPerKind; h++ {
					obsAt := baseTime.Add(-time.Duration(h) * time.Minute).Format(time.RFC3339)
					obsID := fmt.Sprintf("obs-%s-%s-%02d", nid, kind, h)
					if _, err := stmt.ExecContext(ctx,
						obsID, runID, nid, string(kind), string(domain.VerdictAvailable),
						"sha256:1111111111111111111111111111111111111111111111111111111111111111",
						obsAt, 80+h, "status=204 reason=contract_matched",
					); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		b.Fatalf("seed benchmark observations failed: %v", err)
	}

	b.Run("window_rn1_db_latest", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res, err := obsRepo.ListLatestByNodes(ctx, nodeIDs, kinds)
			if err != nil {
				b.Fatalf("ListLatestByNodes failed: %v", err)
			}
			if len(res) != benchNodes {
				b.Fatalf("expected %d nodes, got %d", benchNodes, len(res))
			}
		}
	})

	b.Run("legacy_full_history_scan", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res, err := legacyListLatestByNodesScanAll(ctx, db, nodeIDs, kinds)
			if err != nil {
				b.Fatalf("legacyListLatestByNodesScanAll failed: %v", err)
			}
			if len(res) != benchNodes {
				b.Fatalf("expected %d nodes, got %d", benchNodes, len(res))
			}
		}
	})
}

func legacyListLatestByNodesScanAll(ctx context.Context, db *sql.DB, nodeLogicalIDs []string, kinds []domain.ProbeKind) (map[string]map[domain.ProbeKind]domain.ProbeObservation, error) {
	result := make(map[string]map[domain.ProbeKind]domain.ProbeObservation, len(nodeLogicalIDs))
	for _, id := range nodeLogicalIDs {
		result[id] = make(map[domain.ProbeKind]domain.ProbeObservation)
	}
	const chunkSize = 100
	for i := 0; i < len(nodeLogicalIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(nodeLogicalIDs) {
			end = len(nodeLogicalIDs)
		}
		chunk := nodeLogicalIDs[i:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)+len(kinds))
		for j, id := range chunk {
			placeholders[j] = "?"
			args = append(args, id)
		}
		kindPlaceholders := make([]string, len(kinds))
		for j, k := range kinds {
			kindPlaceholders[j] = "?"
			args = append(args, string(k))
		}
		query := fmt.Sprintf(`
		SELECT id, probe_run_id, node_logical_id, kind, verdict, evidence_digest, observed_at, latency_ms, redacted_summary
		FROM probe_observations
		WHERE node_logical_id IN (%s) AND kind IN (%s)
		ORDER BY observed_at DESC, id DESC;`, strings.Join(placeholders, ", "), strings.Join(kindPlaceholders, ", "))

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var obs domain.ProbeObservation
			var kindStr, verdictStr, observedStr string
			if err := rows.Scan(
				&obs.ID,
				&obs.ProbeRunID,
				&obs.NodeLogicalID,
				&kindStr,
				&verdictStr,
				&obs.EvidenceDigest,
				&observedStr,
				&obs.LatencyMS,
				&obs.RedactedSummary,
			); err != nil {
				rows.Close()
				return nil, err
			}
			obs.Kind = domain.ProbeKind(kindStr)
			obs.Verdict = domain.ProbeVerdict(verdictStr)
			obs.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)
			nodeObs := result[obs.NodeLogicalID]
			if _, exists := nodeObs[obs.Kind]; !exists {
				nodeObs[obs.Kind] = obs
			}
		}
		rows.Close()
	}
	return result, nil
}
