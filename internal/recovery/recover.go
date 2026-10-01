package recovery

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultProductionDBPath identifies the standard production SQLite database path.
const DefaultProductionDBPath = "/data/csp-v1.db"

// IsProductionTarget returns true if the target database path matches known production database locations.
func IsProductionTarget(path string) bool {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return false
	}
	if clean == DefaultProductionDBPath || strings.HasSuffix(clean, "/csp-v1.db") {
		return true
	}
	if prodEnv := os.Getenv("CSP_DB_PATH"); prodEnv != "" && clean == filepath.Clean(prodEnv) {
		return true
	}
	if dbEnv := os.Getenv("DB_PATH"); dbEnv != "" && clean == filepath.Clean(dbEnv) {
		return true
	}
	if strings.HasPrefix(clean, "/etc/csp/") || strings.HasPrefix(clean, "/var/lib/csp/") {
		return true
	}
	return false
}

// IsVerifiableRehearsalSandbox returns true only if the database path is demonstrably a test replica or sandbox.
func IsVerifiableRehearsalSandbox(path string) bool {
	if IsProductionTarget(path) {
		return false
	}
	lower := strings.ToLower(filepath.Clean(strings.TrimSpace(path)))
	if lower == "" {
		return false
	}
	// Strong sandbox detection: must contain replica/test/sandbox/backup/tmp marker
	sandboxIndicators := []string{
		"test", "rehearsal", "replica", "sandbox", "backup", "tmp", "temp", "scratch",
	}
	for _, ind := range sandboxIndicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
}

// DefaultMisdeactivationBatchPrefix identifies the timestamp prefix of the 2026-09-29 mass deactivation.
const DefaultMisdeactivationBatchPrefix = "2026-09-29T00:52"

// CandidateNode represents an attributed or heuristic mis-deactivated node.
type CandidateNode struct {
	LogicalID   string `json:"logical_id"`
	Protocol    string `json:"protocol"`
	DisplayName string `json:"display_name"`
	Server      string `json:"server"`
	Port        int    `json:"port"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// PlanApproval encapsulates explicit authorization from the user or orchestrator.
// The recovery tool is fail-closed and will NEVER apply without non-empty approval evidence.
type PlanApproval struct {
	ApprovedBy       string `json:"approved_by"`        // Actor granting approval (e.g. "orchestrator-simulation", "user:<id>")
	ApprovalEvidence string `json:"approval_evidence"`  // Non-empty evidence token or audit rationale
	ApprovedAt       string `json:"approved_at"`        // Timestamp of approval
	Simulated        bool   `json:"simulated,omitempty"` // Explicitly marks rehearsal/simulation on test replica
}

// RecoveryPlan summarizes the findings of inspection prior to application.
type RecoveryPlan struct {
	TargetDBPath         string          `json:"target_db_path"`
	BatchTimestampPrefix string          `json:"batch_timestamp_prefix"`
	TotalInactiveNodes   int             `json:"total_inactive_nodes"`
	HeuristicCandidates  []CandidateNode `json:"heuristic_candidates"`  // Heuristic matching ONLY; not transactional proof
	AttributedCandidates []CandidateNode `json:"attributed_candidates"` // Confirmed/approved candidates ready for application
	AmbiguousNodes       []CandidateNode `json:"ambiguous_nodes"`        // Ambiguous / unproven nodes
	ManuallyDisabledIDs  []string        `json:"manually_disabled_ids"`
	Approval             *PlanApproval   `json:"approval,omitempty"`
	PlanHash             string          `json:"plan_hash"`
	AppliedTimestamp     string          `json:"applied_timestamp,omitempty"`
}

// ComputePlanHash calculates a SHA-256 digest covering candidate fields and approval evidence.
func ComputePlanHash(candidates []CandidateNode, approval *PlanApproval) string {
	h := sha256.New()
	for _, c := range candidates {
		_, _ = fmt.Fprintf(h, "%s|%s|%s|%s|%d|%s\n", c.LogicalID, c.Protocol, c.Server, c.CreatedAt, c.Port, c.UpdatedAt)
	}
	if approval != nil {
		_, _ = fmt.Fprintf(h, "APPROVAL|%s|%s|%s|%t\n", approval.ApprovedBy, approval.ApprovalEvidence, approval.ApprovedAt, approval.Simulated)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ExecutionReport captures the results of a dry-run, apply, or rollback operation.
type ExecutionReport struct {
	Mode             string    `json:"mode"`
	ExecutedAt       time.Time `json:"executed_at"`
	AppliedTimestamp string    `json:"applied_timestamp,omitempty"`
	PlanHash         string    `json:"plan_hash,omitempty"`
	TargetDBPath     string    `json:"target_db_path"`
	HeuristicCount   int       `json:"heuristic_count"`
	CandidateCount   int       `json:"candidate_count"`
	AmbiguousCount   int       `json:"ambiguous_count"`
	RowsAffected     int64     `json:"rows_affected"`
	RollbackScript   string    `json:"rollback_script,omitempty"`
}

// Inspect queries the database, separates heuristic candidates from confirmed proof, and constructs a RecoveryPlan.
// Heuristic candidates are NOT automatically attributed without explicit transactional/audit proof and user approval.
func Inspect(ctx context.Context, db *sql.DB, targetDBPath string, batchPrefix string) (*RecoveryPlan, error) {
	if batchPrefix == "" {
		batchPrefix = DefaultMisdeactivationBatchPrefix
	}

	plan := &RecoveryPlan{
		TargetDBPath:         targetDBPath,
		BatchTimestampPrefix: batchPrefix,
		HeuristicCandidates:  make([]CandidateNode, 0),
		AttributedCandidates: make([]CandidateNode, 0),
		AmbiguousNodes:       make([]CandidateNode, 0),
		ManuallyDisabledIDs:  make([]string, 0),
	}

	// 1. Identify any manually disabled nodes from audit trail to protect user intent
	disabledMap := make(map[string]bool)
	const manualAuditSQL = `
	SELECT redacted_summary FROM audit_events 
	WHERE action IN ('node.disable', 'node.delete', 'node.deactivate') AND redacted_summary != '';`

	if rows, err := db.QueryContext(ctx, manualAuditSQL); err == nil {
		for rows.Next() {
			var summary string
			if scanErr := rows.Scan(&summary); scanErr == nil && summary != "" {
				fields := strings.Fields(summary)
				for _, f := range fields {
					f = strings.Trim(f, "\"',:;{}[]()")
					if strings.HasPrefix(f, "node_") {
						disabledMap[f] = true
						plan.ManuallyDisabledIDs = append(plan.ManuallyDisabledIDs, f)
					}
				}
			}
		}
		rows.Close()
	}

	// 2. Query all inactive nodes with provenance check (EXISTS node_sources)
	const inactiveNodesSQL = `
	SELECT n.logical_id, n.protocol, n.display_name, n.server, n.port, n.config_json, n.created_at, n.updated_at,
	       EXISTS (SELECT 1 FROM node_sources ns WHERE ns.node_logical_id = n.logical_id) AS has_sources
	FROM nodes n
	WHERE n.active = 0
	ORDER BY n.logical_id ASC;`

	rows, err := db.QueryContext(ctx, inactiveNodesSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to query inactive nodes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, proto, name, server, configJSON, createdAt, updatedAt string
		var port int
		var hasSourcesInt int
		if err := rows.Scan(&id, &proto, &name, &server, &port, &configJSON, &createdAt, &updatedAt, &hasSourcesInt); err != nil {
			return nil, fmt.Errorf("failed to scan inactive node: %w", err)
		}
		plan.TotalInactiveNodes++

		node := CandidateNode{
			LogicalID:   id,
			Protocol:    proto,
			DisplayName: name,
			Server:      server,
			Port:        port,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}

		// User explicit disable protection: NEVER revive
		if disabledMap[id] {
			continue
		}

		// Check valid JSON and configuration sanity
		var parsedJSON any
		isValidJSON := json.Unmarshal([]byte(configJSON), &parsedJSON) == nil && configJSON != "" && configJSON != "{}"

		// Check timestamp attribution to the 2026-09-29T00:52 mis-deactivation window
		isAttributedBatch := strings.HasPrefix(updatedAt, batchPrefix)
		hasValidParams := proto != "" && server != "" && port > 0
		hasNoSources := hasSourcesInt == 0 // Crucial: global orphan bug only deactivated nodes where NOT EXISTS node_sources
		isPreIncidentCreation := createdAt <= batchPrefix

		if isAttributedBatch && hasValidParams && isValidJSON && hasNoSources && isPreIncidentCreation {
			// Separated as HEURISTIC candidate: heuristic features are NOT transactional proof
			plan.HeuristicCandidates = append(plan.HeuristicCandidates, node)
		} else {
			plan.AmbiguousNodes = append(plan.AmbiguousNodes, node)
		}
	}

	plan.PlanHash = ComputePlanHash(plan.AttributedCandidates, plan.Approval)
	return plan, nil
}

// ApproveForRehearsal explicitly approves heuristic candidates for drill/rehearsal on a backup copy.
// It is explicitly marked as simulated and does not constitute causal or production attribution.
func ApproveForRehearsal(plan *RecoveryPlan, approvedBy, evidence string) error {
	if plan == nil {
		return fmt.Errorf("nil recovery plan")
	}
	if strings.TrimSpace(approvedBy) == "" || strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("explicit approved_by and approval_evidence are required for rehearsal")
	}

	plan.Approval = &PlanApproval{
		ApprovedBy:       strings.TrimSpace(approvedBy),
		ApprovalEvidence: strings.TrimSpace(evidence),
		ApprovedAt:       time.Now().UTC().Format(time.RFC3339),
		Simulated:        true,
	}
	// Rehearsal uses heuristic candidates under explicit simulated approval
	plan.AttributedCandidates = append([]CandidateNode(nil), plan.HeuristicCandidates...)
	plan.PlanHash = ComputePlanHash(plan.AttributedCandidates, plan.Approval)
	return nil
}

// Apply executes the atomic, idempotent restoration of confirmed mis-deactivated nodes.
// FAIL-CLOSED: Rejects unproven, unapproved heuristic plans to prevent premature production apply.
func Apply(ctx context.Context, db *sql.DB, plan *RecoveryPlan) (*ExecutionReport, error) {
	if plan == nil {
		return nil, fmt.Errorf("nil recovery plan")
	}

	// Fail-closed gate: explicit approval is strictly mandatory
	if plan.Approval == nil || strings.TrimSpace(plan.Approval.ApprovedBy) == "" || strings.TrimSpace(plan.Approval.ApprovalEvidence) == "" {
		return nil, fmt.Errorf("fail-closed: unconfirmed recovery plan cannot be applied without explicit user or orchestrator approval (heuristic candidates are not transactional proof)")
	}

	if len(plan.AttributedCandidates) == 0 {
		return nil, fmt.Errorf("fail-closed: no confirmed/approved candidates in recovery plan; heuristic candidates alone cannot be applied without explicit approval")
	}

	// Strong isolation protection: simulated approval MUST NEVER be applied to production databases or non-sandbox targets
	if plan.Approval.Simulated {
		if IsProductionTarget(plan.TargetDBPath) || !IsVerifiableRehearsalSandbox(plan.TargetDBPath) {
			return nil, fmt.Errorf("fail-closed: simulated rehearsal approval (simulated=true) cannot be used on production database or unverified target (%s); rehearsal is strictly restricted to verifiable isolated test replicas/sandboxes", plan.TargetDBPath)
		}
	} else {
		// Production apply is strictly forbidden without complete transactional causality proof and historical audit baselines
		return nil, fmt.Errorf("fail-closed: production apply is prohibited without complete transactional causality proof and historical audit baselines; heuristic candidates cannot be attributed or applied to production (at this stage production recovery is strictly disabled)")
	}

	expectedHash := ComputePlanHash(plan.AttributedCandidates, plan.Approval)
	if plan.PlanHash != "" && plan.PlanHash != expectedHash {
		return nil, fmt.Errorf("plan hash mismatch: plan candidate list or approval evidence has been tampered with or corrupted")
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	plan.AppliedTimestamp = nowStr

	mode := "apply"
	if plan.Approval.Simulated {
		mode = "rehearsal_apply"
	}

	report := &ExecutionReport{
		Mode:             mode,
		ExecutedAt:       time.Now().UTC(),
		AppliedTimestamp: nowStr,
		PlanHash:         expectedHash,
		TargetDBPath:     plan.TargetDBPath,
		HeuristicCount:   len(plan.HeuristicCandidates),
		CandidateCount:   len(plan.AttributedCandidates),
		AmbiguousCount:   len(plan.AmbiguousNodes),
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Concurrency protection: WHERE active = 0 AND updated_at = c.UpdatedAt
	stmt, err := tx.PrepareContext(ctx, `
		UPDATE nodes
		SET active = 1, updated_at = ?
		WHERE logical_id = ? AND active = 0 AND updated_at = ?;
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare update: %w", err)
	}
	defer stmt.Close()

	var totalAffected int64
	var restoredIDs []string
	for _, c := range plan.AttributedCandidates {
		res, execErr := stmt.ExecContext(ctx, nowStr, c.LogicalID, c.UpdatedAt)
		if execErr != nil {
			return nil, fmt.Errorf("failed to update node %s: %w", c.LogicalID, execErr)
		}
		aff, _ := res.RowsAffected()
		if aff > 0 {
			totalAffected += aff
			restoredIDs = append(restoredIDs, c.LogicalID)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	report.RowsAffected = totalAffected
	if len(restoredIDs) > 0 {
		var quoted []string
		for _, id := range restoredIDs {
			quoted = append(quoted, fmt.Sprintf("'%s'", id))
		}
		report.RollbackScript = fmt.Sprintf(
			"UPDATE nodes SET active = 0, updated_at = '%s' WHERE logical_id IN (%s);",
			plan.BatchTimestampPrefix+":00Z",
			strings.Join(quoted, ", "),
		)
	}

	return report, nil
}

// Rollback reverses the recovery application by safely deactivating the attributed candidates.
// Safety invariant: only reverts nodes whose updated_at equals the AppliedTimestamp from this tool run,
// preventing accidental overwriting of subsequent user modifications.
func Rollback(ctx context.Context, db *sql.DB, plan *RecoveryPlan) (*ExecutionReport, error) {
	if plan == nil {
		return nil, fmt.Errorf("nil recovery plan")
	}

	report := &ExecutionReport{
		Mode:           "rollback",
		ExecutedAt:     time.Now().UTC(),
		PlanHash:       plan.PlanHash,
		TargetDBPath:   plan.TargetDBPath,
		HeuristicCount: len(plan.HeuristicCandidates),
		CandidateCount: len(plan.AttributedCandidates),
		AmbiguousCount: len(plan.AmbiguousNodes),
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin rollback transaction: %w", err)
	}
	defer tx.Rollback()

	var stmt *sql.Stmt
	if plan.AppliedTimestamp != "" {
		// Strictly protect subsequent user edits: only rollback if updated_at is unchanged since Apply
		stmt, err = tx.PrepareContext(ctx, `
			UPDATE nodes
			SET active = 0, updated_at = ?
			WHERE logical_id = ? AND active = 1 AND updated_at = ?;
		`)
	} else {
		stmt, err = tx.PrepareContext(ctx, `
			UPDATE nodes
			SET active = 0, updated_at = ?
			WHERE logical_id = ? AND active = 1;
		`)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to prepare rollback update: %w", err)
	}
	defer stmt.Close()

	var totalAffected int64
	for _, c := range plan.AttributedCandidates {
		var res sql.Result
		var execErr error
		if plan.AppliedTimestamp != "" {
			res, execErr = stmt.ExecContext(ctx, c.UpdatedAt, c.LogicalID, plan.AppliedTimestamp)
		} else {
			res, execErr = stmt.ExecContext(ctx, c.UpdatedAt, c.LogicalID)
		}
		if execErr != nil {
			return nil, fmt.Errorf("failed to rollback node %s: %w", c.LogicalID, execErr)
		}
		aff, _ := res.RowsAffected()
		totalAffected += aff
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit rollback transaction: %w", err)
	}

	report.RowsAffected = totalAffected
	return report, nil
}
