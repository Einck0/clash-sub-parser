package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCompactionInspectionStageWrapPreservesCauseWithoutLeaking(t *testing.T) {
	secret := "secret://sentinel?password=do-not-print SQL /private/snapshot.db"
	for _, cause := range []error{context.DeadlineExceeded, context.Canceled, errors.New(secret)} {
		wrapped := &compactionInspectionError{"reconcile_history", fmt.Errorf("%s: %w", secret, cause)}
		if !errors.Is(wrapped, cause) {
			t.Fatal("inspection wrapping lost error identity")
		}
		if stage := SubscriptionCompactionInspectionStage(fmt.Errorf("outer: %w", wrapped)); stage != "reconcile_history" {
			t.Fatalf("stage=%q", stage)
		}
		if strings.Contains(wrapped.Error(), secret) {
			t.Fatal("safe stage wrapper leaked cause")
		}
	}
	if stage := SubscriptionCompactionInspectionStage(errors.New(secret)); stage != "unknown" {
		t.Fatalf("untrusted error assigned stage=%q", stage)
	}
}

func TestCompactionInspectionFailureStagesAndNoPartialPlan(t *testing.T) {
	db := compactionOrderDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan, err := InspectSubscriptionCompaction(ctx, db)
	if plan != nil || !errors.Is(err, context.Canceled) || SubscriptionCompactionInspectionStage(err) != "begin_snapshot" {
		t.Fatalf("cancelled snapshot plan=%+v stage=%s err=%v", plan, SubscriptionCompactionInspectionStage(err), err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations(version TEXT); INSERT INTO schema_migrations VALUES('secret://sentinel-password')`); err != nil {
		t.Fatal(err)
	}
	plan, err = InspectSubscriptionCompaction(context.Background(), db)
	if plan != nil || err == nil || SubscriptionCompactionInspectionStage(err) != "validate_schema" {
		t.Fatalf("invalid schema plan=%+v stage=%s err=%v", plan, SubscriptionCompactionInspectionStage(err), err)
	}
	if strings.Contains(err.Error(), "sentinel-password") {
		t.Fatal("scan error content leaked through inspection wrapper")
	}
}
