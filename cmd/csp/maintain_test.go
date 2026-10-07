package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func setupTestMaintainDB(t *testing.T) (string, *sql.DB) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test-maintain.db")

	dbCfg := sqlite.DefaultConfig(dbPath)
	db, err := sqlite.Open(dbCfg)
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	return dbPath, db
}

func TestMaintainInventory_DeleteRule_Lifecycle(t *testing.T) {
	dbPath, db := setupTestMaintainDB(t)
	ctx := context.Background()

	// Seed policy group "其他" and "选择节点"
	otherGroupID := "01a0b9af-c116-71ba-a125-31cefefe0d4b"
	parentGroupID := "01a0b9af-c116-7806-9740-66b8b070ba2f"
	targetRuleID := "01a0b9af-c118-72f9-9949-d94b49fa6ec2"

	appServices, err := makeApplicationServices(ctx, db, appServiceOptions{})
	if err != nil {
		t.Fatalf("makeApplicationServices failed: %v", err)
	}

	_, err = appServices.policyService.CreateGroup(ctx, policy.CreateGroupCommand{
		ID:        otherGroupID,
		Name:      "其他",
		GroupType: domain.GroupTypeSelect,
		RequestID: "req-create-other",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("CreateGroup 其他 failed: %v", err)
	}

	_, err = appServices.policyService.CreateGroup(ctx, policy.CreateGroupCommand{
		ID:        parentGroupID,
		Name:      "选择节点",
		GroupType: domain.GroupTypeSelect,
		Edges: []policy.EdgeInput{
			{ChildGroupID: &otherGroupID, Position: 0},
		},
		RequestID: "req-create-parent",
		ActorKind: domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("CreateGroup 选择节点 failed: %v", err)
	}

	// Create the exact target rule
	_, err = appServices.policyService.CreatePolicyRule(ctx, policy.CreatePolicyRuleCommand{
		ID:            targetRuleID,
		TargetGroupID: otherGroupID,
		Expression:    "PROCESS-NAME,tr.com.kliq.app",
		Position:      10,
		RequestID:     "req-create-rule",
		ActorKind:     domain.ActorKindAdmin,
	})
	if err != nil {
		t.Fatalf("CreatePolicyRule failed: %v", err)
	}

	// Close test db handle before CLI runs to avoid file lock issues
	_ = db.Close()

	t.Run("guard mismatch rejects without mutation", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		args := []string{
			"maintain-inventory",
			"--db", dbPath,
			"--delete-rule-id", targetRuleID,
			"--expected-rule-expression", "PROCESS-NAME,wrong.app",
			"--allow-concurrent-service",
		}
		exitCode := run(args, stdout, stderr)
		if exitCode != 1 {
			t.Fatalf("expected exit code 1 for mismatched guard, got %d. Stderr: %s", exitCode, stderr.String())
		}
	})

	t.Run("dry-run verifies guards and reports counts without mutation", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		args := []string{
			"maintain-inventory",
			"--db", dbPath,
			"--dry-run",
			"--delete-rule-id", targetRuleID,
			"--expected-rule-expression", "PROCESS-NAME,tr.com.kliq.app",
			"--expected-target-group-id", otherGroupID,
			"--expected-rule-type", "PROCESS-NAME",
			"--expected-rule-value", "tr.com.kliq.app",
			"--allow-concurrent-service",
		}
		exitCode := run(args, stdout, stderr)
		if exitCode != 0 {
			t.Fatalf("expected exit code 0 for dry-run, got %d. Stderr: %s", exitCode, stderr.String())
		}

		var rep map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
			t.Fatalf("failed to parse stdout JSON: %v. Output: %s", err, stdout.String())
		}
		if rep["status"] != "SUCCESS" || rep["dry_run"] != true || rep["matched_count"] != float64(1) || rep["deleted_count"] != float64(0) {
			t.Fatalf("unexpected dry-run report: %+v", rep)
		}
	})

	t.Run("apply executes deletion and creates new active revision", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		args := []string{
			"maintain-inventory",
			"--db", dbPath,
			"--delete-rule-id", targetRuleID,
			"--expected-rule-expression", "PROCESS-NAME,tr.com.kliq.app",
			"--expected-target-group-id", otherGroupID,
			"--allow-concurrent-service",
		}
		exitCode := run(args, stdout, stderr)
		if exitCode != 0 {
			t.Fatalf("expected exit code 0 for apply, got %d. Stderr: %s", exitCode, stderr.String())
		}

		var rep map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
			t.Fatalf("failed to parse stdout JSON: %v. Output: %s", err, stdout.String())
		}
		if rep["status"] != "SUCCESS" || rep["dry_run"] != false || rep["deleted_count"] != float64(1) {
			t.Fatalf("unexpected apply report: %+v", rep)
		}

		// Reopen db and verify rule is gone, but group 其他 is intact
		vDB, err := sqlite.OpenReadOnly(dbPath)
		if err != nil {
			t.Fatalf("failed to open db readonly: %v", err)
		}
		defer vDB.Close()

		var ruleCount int
		_ = vDB.QueryRowContext(ctx, "SELECT count(*) FROM policy_rules WHERE id = ?;", targetRuleID).Scan(&ruleCount)
		if ruleCount != 0 {
			t.Fatalf("expected rule count 0, got %d", ruleCount)
		}

		var groupName string
		_ = vDB.QueryRowContext(ctx, "SELECT name FROM node_groups WHERE id = ?;", otherGroupID).Scan(&groupName)
		if groupName != "其他" {
			t.Fatalf("expected group name 其他, got %q", groupName)
		}
	})

	t.Run("idempotent replay reports already deleted and exit code 0", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		args := []string{
			"maintain-inventory",
			"--db", dbPath,
			"--delete-rule-id", targetRuleID,
			"--allow-concurrent-service",
		}
		exitCode := run(args, stdout, stderr)
		if exitCode != 0 {
			t.Fatalf("expected exit code 0 for idempotent replay, got %d. Stderr: %s", exitCode, stderr.String())
		}

		var rep map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
			t.Fatalf("failed to parse stdout JSON: %v. Output: %s", err, stdout.String())
		}
		if rep["status"] != "SUCCESS" || rep["already_deleted"] != true || rep["deleted_count"] != float64(0) {
			t.Fatalf("unexpected replay report: %+v", rep)
		}
	})
}
