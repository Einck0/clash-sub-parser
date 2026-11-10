package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"clash-sub-parser/internal/repository/sqlite"
)

func TestSubscriptionCompactionRefusesUnsupportedMutations(t *testing.T) {
	for _, operation := range []string{"apply", "", "unknown"} {
		t.Run(operation, func(t *testing.T) {
			var out, stderr bytes.Buffer
			if code := runSubscriptionCompaction([]string{operation, "--db", filepath.Join(t.TempDir(), "must-not-exist.db")}, &out, &stderr); code == 0 {
				t.Fatal("unimplemented operation reported success")
			}
			if !strings.Contains(stderr.String(), "maintenance_required") || out.Len() != 0 {
				t.Fatal("unimplemented mutation must fail closed without any success output")
			}
		})
	}
	var out, stderr bytes.Buffer
	if code := runSubscriptionCompaction([]string{"verify", "--db", filepath.Join(t.TempDir(), "must-not-exist.db")}, &out, &stderr); code == 0 {
		t.Fatal("verify without baseline reported success")
	}
}

func TestSubscriptionCompactionPlanPreservesInputAndPrivateReport(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "snapshot.db")
	cfg := sqlite.DefaultConfig(path)
	cfg.WALMode = false
	db, err := sqlite.OpenAndMigrate(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(dir, "plan.json")
	var out, stderr bytes.Buffer
	if code := run([]string{"compact-subscription-storage", "plan", "--db", path, "--report", reportPath}, &out, &stderr); code != 0 {
		t.Fatalf("read-only plan exit=%d stderr=%s", code, stderr.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("plan modified snapshot: %v", err)
	}
	report, err := os.ReadFile(reportPath)
	if err != nil || !bytes.Contains(report, []byte(`"apply_supported":false`)) {
		t.Fatalf("plan must explicitly disclose unsupported apply: %v", err)
	}
	info, err := os.Stat(reportPath)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("report must be private: %v", err)
	}
	out.Reset()
	stderr.Reset()
	if code := runSubscriptionCompaction([]string{"plan", "--db", path, "--report", reportPath}, &out, &stderr); code == 0 {
		t.Fatal("existing report must not be overwritten")
	}
	retained, err := os.ReadFile(reportPath)
	if err != nil || !bytes.Equal(report, retained) {
		t.Fatalf("existing report changed: %v", err)
	}
	alias := filepath.Join(dir, "alias.db")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if code := runSubscriptionCompaction([]string{"plan", "--db", alias, "--report", filepath.Join(dir, "alias-plan.json")}, &out, &stderr); code == 0 {
		t.Fatal("snapshot symlink must not be accepted")
	}
	out.Reset()
	stderr.Reset()
	if code := run([]string{"compact-subscription-storage", "verify", "--db", path, "--baseline", reportPath}, &out, &stderr); code != 0 {
		t.Fatalf("read-only verify failed: exit=%d stderr=%s", code, stderr.String())
	}
	after, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("verify modified database: %v", err)
	}
	mutated, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutated.ExecContext(context.Background(), `INSERT INTO nodes(logical_id,protocol,display_name,server,port,config_json,active,created_at,updated_at,connection_revision)VALUES('tampered','ss','Tampered','192.0.2.1',8388,'{}',1,'2026-11-10T00:00:00Z','2026-11-10T00:00:00Z',1)`); err != nil {
		mutated.Close()
		t.Fatal(err)
	}
	mutated.Close()
	out.Reset()
	stderr.Reset()
	if code := run([]string{"compact-subscription-storage", "verify", "--db", path, "--baseline", reportPath}, &out, &stderr); code == 0 {
		t.Fatal("verify accepted protected asset change")
	}
}
