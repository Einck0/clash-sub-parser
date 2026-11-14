package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestProbePolicySnapshotIncludesDisabledAndUnpagedSubscriptions(t *testing.T) {
	db, _ := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 105; i++ {
		config := `{"probe_policy":{"mode":"custom","interval_seconds":180}}`
		if i == 0 {
			config = `{"probe_policy":{"mode":"disabled"}}`
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscriptions (id,name,source_url_secret_ref,enabled,revision,config_json,created_at,updated_at) VALUES (?,?,'synthetic',?,'1',?,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`, fmt.Sprintf("policy-%03d", i), fmt.Sprintf("policy-%03d", i), i%2, config); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	reader := sqlite.NewSubscriptionRepository(db).(domain.ProbePolicySubscriptionReader)
	rows, err := reader.ListProbePolicies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 105 {
		t.Fatalf("rows=%d want 105", len(rows))
	}
	for i, row := range rows {
		if row.ID != fmt.Sprintf("policy-%03d", i) || row.Enabled != (i%2 == 1) {
			t.Fatalf("policy identity/enable mismatch: %+v", row)
		}
		policy := row.Config.EffectiveProbePolicy()
		if i == 0 {
			if policy.Mode != domain.ProbePolicyModeDisabled {
				t.Fatalf("disabled policy lost: %+v", policy)
			}
		} else if policy.Mode != domain.ProbePolicyModeCustom || policy.IntervalSeconds == nil || *policy.IntervalSeconds != 180 {
			t.Fatalf("custom policy lost: %+v", policy)
		}
	}
	// Malformed stored policy must not silently schedule using the global default.
	if _, err := db.ExecContext(ctx, `UPDATE subscriptions SET config_json='{' WHERE id='policy-000'`); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ListProbePolicies(ctx); err == nil {
		t.Fatal("malformed policy accepted")
	}
}
