package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// compactionInspectionError exposes only a trusted, fixed phase. The original
// error remains available to errors.Is/As, but is never interpolated in output.
type compactionInspectionError struct {
	stage string
	cause error
}

func (e *compactionInspectionError) Error() string { return "compaction inspection failed: " + e.stage }
func (e *compactionInspectionError) Unwrap() error { return e.cause }

// SubscriptionCompactionInspectionStage returns a phase assigned by the
// inspector, never a table name or an untrusted error's string or Stage method.
func SubscriptionCompactionInspectionStage(err error) string {
	var inspection *compactionInspectionError
	if errors.As(err, &inspection) {
		return inspection.stage
	}
	return "unknown"
}

// InspectSubscriptionCompaction runs on one read snapshot. It never invokes a
// migration runner, enables WAL, creates tables, selects/deletes survivors or
// marks a database ready. Use OpenReadOnly for file-backed callers. Full-table
// hashing is intentionally an explicit offline operation, not a health check.
func InspectSubscriptionCompaction(ctx context.Context, db *sql.DB) (*domain.SubscriptionStoragePlan, error) {
	if db == nil {
		return nil, &compactionInspectionError{"begin_snapshot", errors.New("database unavailable")}
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, &compactionInspectionError{"begin_snapshot", err}
	}
	defer tx.Rollback()
	return inspectSubscriptionCompactionTx(ctx, tx)
}

func inspectSubscriptionCompactionTx(ctx context.Context, tx *sql.Tx) (_ *domain.SubscriptionStoragePlan, err error) {
	stage := "list_tables"
	defer func() {
		if err != nil {
			err = &compactionInspectionError{stage, err}
		}
	}()
	plan := &domain.SubscriptionStoragePlan{
		Version: 1, Counts: make(map[string]int64), ProtectedDigests: make(map[string]string),
		Subscriptions:   []domain.SubscriptionStorageStatus{},
		Blockers:        []string{"unsupported_schema", "version_consumers_not_closed"},
		VerifySupported: true,
	}
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("compaction plan: list tables: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	present := make(map[string]bool, len(tables))
	whole := sha256.New()
	// A sealed plan must bind schema definitions as well as row content.
	stage = "fingerprint_schema"
	_, schemaDigest, err := digestCompactionTable(ctx, tx, "sqlite_schema")
	if err != nil {
		return nil, fmt.Errorf("compaction plan: fingerprint schema: %w", err)
	}
	if err := writeCompactionDigest(whole, schemaDigest); err != nil {
		return nil, err
	}
	stage = "fingerprint_tables"
	for _, name := range tables {
		present[name] = true
		count, digest, err := digestCompactionTable(ctx, tx, name)
		if err != nil {
			return nil, fmt.Errorf("compaction plan: fingerprint table %s: %w", name, err)
		}
		plan.Counts[name] = count
		// Only these tables may have data transformed under the frozen design.
		// Every other table is protected by content, not merely row count.
		switch name {
		case "schema_migrations", "subscription_payloads", "subscription_entries", "subscription_fetches",
			"node_connection_versions", "node_connection_heads", "subscription_entry_overrides",
			"subscription_payload_evidence", "node_connection_provenance", "node_source_observations",
			"subscription_current_state", "node_connection_version_leases", "subscription_storage_readiness":
		default:
			plan.ProtectedDigests[name] = digest
		}
		if err := writeCompactionDigest(whole, []any{name, count, digest}); err != nil {
			return nil, err
		}
	}
	plan.SnapshotDigest = hex.EncodeToString(whole.Sum(nil))
	stage = "validate_schema"
	if present["schema_migrations"] {
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&plan.SchemaVersion); err != nil {
			return nil, err
		}
	}
	for _, required := range []string{"subscriptions", "subscription_payloads", "subscription_fetches", "subscription_entries", "node_sources", "node_connection_versions", "node_connection_heads", "publication_payload_refs"} {
		if !present[required] {
			return nil, fmt.Errorf("compaction plan: required table missing: %s", required)
		}
	}
	rawExpr := `''`
	if present["subscription_original_configs"] {
		rawExpr = `COALESCE((SELECT c.fetch_id FROM subscription_original_configs c WHERE c.subscription_id=s.id),'')`
	}
	stage = "subscription_inventory"
	query := `SELECT s.id,
		(SELECT COUNT(*) FROM subscription_payloads p WHERE p.subscription_id=s.id),
		(SELECT COUNT(*) FROM subscription_entries e WHERE e.subscription_id=s.id),
		(SELECT COUNT(*) FROM subscription_payloads p JOIN subscription_fetches f ON f.id=p.fetch_id WHERE p.subscription_id=s.id AND f.outcome IN ('success','partial')),
		COALESCE((SELECT f.id FROM subscription_fetches f WHERE f.subscription_id=s.id ORDER BY julianday(f.started_at) DESC,f.rowid DESC LIMIT 1),''),
		COALESCE((SELECT f.outcome FROM subscription_fetches f WHERE f.subscription_id=s.id ORDER BY julianday(f.started_at) DESC,f.rowid DESC LIMIT 1),''), ` + rawExpr + ` FROM subscriptions s ORDER BY s.id`
	rows, err = tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status domain.SubscriptionStorageStatus
		if err := rows.Scan(&status.SubscriptionID, &status.PayloadCount, &status.EntryCount, &status.CandidateCount, &status.LastAttemptID, &status.LastAttemptStatus, &status.RawFetchID); err != nil {
			rows.Close()
			return nil, err
		}
		plan.Subscriptions = append(plan.Subscriptions, status)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if plan.SchemaVersion >= 27 && plan.SchemaVersion <= 30 {
		stage = "generation_selection"
		selectionErr := selectCompactionGenerationsTx(ctx, tx, plan)
		if selectionErr != nil && !errors.Is(selectionErr, errCompactionSelectionUnresolved) {
			return nil, selectionErr
		}
		stage = "reconcile_sources"
		count, digest, err := digestCompactionSource(ctx, tx, "node_sources", compactionCurrentSources)
		if err != nil {
			return nil, err
		}
		plan.ReconciledSourcesCount, plan.ReconciledSourcesDigest = count, digest
		stage = "reconcile_history"
		count, digest, err = digestCompactionSource(ctx, tx, "node_source_history", compactionProjectedHistory)
		if err != nil {
			return nil, err
		}
		plan.ReconciledHistoryCount, plan.ReconciledHistoryDigest = count, digest
		stage = "readiness"
		var sealed int
		if present["subscription_storage_readiness"] {
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_storage_readiness`).Scan(&sealed); err != nil {
				return nil, err
			}
		}
		// Read-only inspection of already sealed runtime databases compares
		// existing business content, not hypothetical legacy observation rows.
		if sealed != 0 {
			plan.ReconciledSourcesDigest, plan.ReconciledSourcesCount = plan.ProtectedDigests["node_sources"], plan.Counts["node_sources"]
			plan.ReconciledHistoryDigest, plan.ReconciledHistoryCount = plan.ProtectedDigests["node_source_history"], plan.Counts["node_source_history"]
		}
		plan.ApplySupported = selectionErr == nil && sealed == 0
		plan.Blockers = []string{"operator_offline_authorization_required", "version_consumers_not_closed"}
		if selectionErr != nil {
			plan.Blockers = append(plan.Blockers, "legacy_generation_or_membership_unresolved")
		}
		if sealed != 0 {
			plan.Blockers = append(plan.Blockers, "already_sealed")
		}
	}
	return plan, nil
}

// InitializeEmptySubscriptionStorageTx is for brand new databases only. It
// cannot silently convert a legacy database that already contains business rows.
func InitializeEmptySubscriptionStorageTx(ctx context.Context, tx *sql.Tx) error {
	for _, table := range []string{"subscriptions", "nodes", "subscription_payloads", "publications"} {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+quoteCompactionIdentifier(table)).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("maintenance_required: initialization requires an empty database")
		}
	}
	if err := redirectPublicationEvidenceTx(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_subscription_single_payload ON subscription_payloads(subscription_id)`); err != nil {
		return err
	}
	for _, trigger := range subscriptionStorageTriggers {
		if _, err := tx.ExecContext(ctx, trigger); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO subscription_storage_readiness (singleton,contract_version,consumers_closed,sealed_at) VALUES (1,1,0,?)`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// Trigger definitions are shared between empty initialization and read-only
// readiness validation. A same-named inert trigger must not satisfy the seal.
var subscriptionStorageTriggers = map[string]string{
	"connection_version_configuration_immutable": `CREATE TRIGGER connection_version_configuration_immutable BEFORE UPDATE OF effective_config_json,config_fingerprint ON node_connection_versions WHEN NEW.effective_config_json IS NOT OLD.effective_config_json OR NEW.config_fingerprint IS NOT OLD.config_fingerprint BEGIN SELECT RAISE(ABORT,'connection version configuration is immutable'); END`,
	"subscription_payload_empty_body_insert":     `CREATE TRIGGER subscription_payload_empty_body_insert BEFORE INSERT ON subscription_payloads WHEN length(NEW.body_blob)!=0 BEGIN SELECT RAISE(ABORT,'payload body must remain empty'); END`,
	"subscription_payload_empty_body_update":     `CREATE TRIGGER subscription_payload_empty_body_update BEFORE UPDATE OF body_blob ON subscription_payloads WHEN length(NEW.body_blob)!=0 BEGIN SELECT RAISE(ABORT,'payload body must remain empty'); END`,
	"subscription_entry_current_source":          `CREATE TRIGGER subscription_entry_current_source BEFORE INSERT ON subscription_entries WHEN NOT EXISTS(SELECT 1 FROM subscription_payloads p WHERE p.id=NEW.payload_id AND p.subscription_id=NEW.subscription_id) BEGIN SELECT RAISE(ABORT,'entry must belong to current subscription payload'); END`,
	"subscription_entry_current_source_update":   `CREATE TRIGGER subscription_entry_current_source_update BEFORE UPDATE OF payload_id,subscription_id ON subscription_entries WHEN NOT EXISTS(SELECT 1 FROM subscription_payloads p WHERE p.id=NEW.payload_id AND p.subscription_id=NEW.subscription_id) BEGIN SELECT RAISE(ABORT,'entry must belong to current subscription payload'); END`,
}

// CheckSubscriptionStorageReady is read-only and verifies the actual constraint
// definition, not merely a migration number or a forgeable ready flag.
func CheckSubscriptionStorageReady(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return checkSubscriptionStorageReadyTx(ctx, tx)
}

func checkSubscriptionStorageReadyTx(ctx context.Context, tx *sql.Tx) error {
	var schemaVersion int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&schemaVersion); err != nil || schemaVersion != 30 {
		return fmt.Errorf("maintenance_required: unsupported storage schema")
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT contract_version FROM subscription_storage_readiness WHERE singleton=1`).Scan(&version); err != nil || version != 1 {
		return fmt.Errorf("maintenance_required: subscription storage is not sealed")
	}
	var unique, partial int
	if err := tx.QueryRowContext(ctx, `SELECT "unique",partial FROM pragma_index_list('subscription_payloads') WHERE name='idx_subscription_single_payload'`).Scan(&unique, &partial); err != nil || unique != 1 || partial != 0 {
		return fmt.Errorf("maintenance_required: single-payload constraint missing")
	}
	var count int
	var column string
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(name),'') FROM pragma_index_info('idx_subscription_single_payload')`).Scan(&count, &column); err != nil || count != 1 || column != "subscription_id" {
		return fmt.Errorf("maintenance_required: invalid single-payload constraint")
	}
	var refs int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_list('publication_payload_refs') WHERE "table"='subscription_payload_evidence' AND "from"='payload_id' AND "to"='payload_id' AND on_delete='RESTRICT'`).Scan(&refs); err != nil || refs != 1 {
		return fmt.Errorf("maintenance_required: publication evidence foreign key missing")
	}
	for name, expected := range subscriptionStorageTriggers {
		var actual string
		if err := tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_schema WHERE type='trigger' AND name=?`, name).Scan(&actual); err != nil || strings.TrimSpace(actual) != expected {
			return fmt.Errorf("maintenance_required: storage enforcement trigger missing or changed")
		}
	}
	return nil
}

// VerifySubscriptionCompaction is strictly read-only. It verifies sealed
// constraints, PRAGMA integrity/foreign_key_check, single-generation invariants
// and exact content digests for every protected table against the baseline plan.
func VerifySubscriptionCompaction(ctx context.Context, db *sql.DB, baseline *domain.SubscriptionStoragePlan) (*domain.SubscriptionStorageVerification, error) {
	if db == nil || baseline == nil || baseline.Version != 1 || strings.TrimSpace(baseline.SnapshotDigest) == "" || len(baseline.ProtectedDigests) == 0 {
		return nil, fmt.Errorf("compaction verify: valid baseline plan required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := inspectSubscriptionCompactionTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	report := &domain.SubscriptionStorageVerification{
		Version: 1, BaselineDigest: baseline.SnapshotDigest, CandidateDigest: current.SnapshotDigest,
		SchemaVersion: current.SchemaVersion, Counts: current.Counts, Failures: []string{},
	}
	if err := checkSubscriptionStorageReadyTx(ctx, tx); err != nil {
		report.Failures = append(report.Failures, "storage_not_ready")
	} else {
		var closed int
		if err := tx.QueryRowContext(ctx, `SELECT consumers_closed FROM subscription_storage_readiness WHERE singleton=1`).Scan(&closed); err != nil {
			return nil, err
		}
		report.ConsumerClosureSealed = closed == 1
	}
	var integrity string
	if err := tx.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&integrity); err != nil {
		return nil, err
	}
	if integrity != "ok" {
		report.Failures = append(report.Failures, "integrity_check_failed")
	}
	fkRows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, err
	}
	if fkRows.Next() {
		report.Failures = append(report.Failures, "foreign_key_violation")
	}
	fkErr := fkRows.Err()
	fkRows.Close()
	if fkErr != nil {
		return nil, fkErr
	}
	for table, want := range baseline.ProtectedDigests {
		wantCount := baseline.Counts[table]
		if table == "node_sources" && baseline.ReconciledSourcesDigest != "" {
			want, wantCount = baseline.ReconciledSourcesDigest, baseline.ReconciledSourcesCount
		}
		if table == "node_source_history" && baseline.ReconciledHistoryDigest != "" {
			want, wantCount = baseline.ReconciledHistoryDigest, baseline.ReconciledHistoryCount
		}
		got, ok := current.ProtectedDigests[table]
		if !ok || got != want || current.Counts[table] != wantCount {
			report.Failures = append(report.Failures, "protected_table_changed:"+table)
		}
	}
	for table := range current.ProtectedDigests {
		if _, ok := baseline.ProtectedDigests[table]; !ok {
			// Schema27/28 never had a raw slot. Migration29 creates an
			// empty one, not invented historical raw content.
			if table == "subscription_original_configs" && baseline.SchemaVersion < 29 && current.Counts[table] == 0 {
				continue
			}
			report.Failures = append(report.Failures, "unexpected_protected_table:"+table)
		}
	}
	// Compare against the pre-deletion plan, not just the surviving rows. A
	// self-consistent candidate can still have lost the actual latest attempt.
	for _, expected := range baseline.Subscriptions {
		var attempts int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_current_state
			WHERE subscription_id=? AND COALESCE(last_attempt_fetch_id,'')=? AND last_attempt_status=?`,
			expected.SubscriptionID, expected.LastAttemptID, expected.LastAttemptStatus).Scan(&attempts); err != nil {
			return nil, err
		}
		if attempts != 1 {
			report.Failures = append(report.Failures, "last_attempt_changed:"+expected.SubscriptionID)
		}
	}
	for name, q := range map[string]string{
		"multiple_payloads_per_subscription": `SELECT COUNT(*) FROM (SELECT subscription_id FROM subscription_payloads GROUP BY subscription_id HAVING COUNT(*)>1)`,
		"nonempty_payload_body":              `SELECT COUNT(*) FROM subscription_payloads WHERE length(body_blob)!=0`,
		"orphan_subscription_entries":        `SELECT COUNT(*) FROM subscription_entries e WHERE NOT EXISTS(SELECT 1 FROM subscription_payloads p WHERE p.id=e.payload_id AND p.subscription_id=e.subscription_id)`,
		"missing_node_head":                  `SELECT COUNT(*) FROM nodes n WHERE NOT EXISTS(SELECT 1 FROM node_connection_heads h JOIN node_connection_versions v ON v.node_logical_id=h.logical_id AND v.connection_revision=h.connection_revision WHERE h.logical_id=n.logical_id AND h.connection_revision=n.connection_revision)`,
		"unleased_nonhead_version": `SELECT COUNT(*) FROM node_connection_versions v
			WHERE NOT EXISTS(SELECT 1 FROM node_connection_heads h WHERE h.logical_id=v.node_logical_id AND h.connection_revision=v.connection_revision)
			AND NOT EXISTS(SELECT 1 FROM node_connection_version_leases l WHERE l.node_logical_id=v.node_logical_id AND l.connection_revision=v.connection_revision)`,
		"missing_publication_evidence":  `SELECT COUNT(*) FROM publication_payload_refs r WHERE NOT EXISTS(SELECT 1 FROM subscription_payload_evidence e WHERE e.payload_id=r.payload_id)`,
		"dangling_version_source_entry": `SELECT COUNT(*) FROM node_connection_versions v WHERE v.source_entry_id IS NOT NULL AND v.source_entry_id!='' AND NOT EXISTS(SELECT 1 FROM subscription_entries e WHERE e.id=v.source_entry_id)`,
		"invalid_applied_state": `SELECT COUNT(*) FROM subscription_current_state c LEFT JOIN subscriptions s ON s.id=c.subscription_id
			WHERE s.id IS NULL OR (c.applied_payload_id IS NULL) != (c.applied_fetch_id IS NULL)
			OR (c.applied_payload_id IS NOT NULL AND (c.source_revision != s.revision OR NOT EXISTS (
				SELECT 1 FROM subscription_payloads p JOIN subscription_fetches f ON f.id=p.fetch_id
				WHERE p.id=c.applied_payload_id AND p.subscription_id=c.subscription_id AND f.id=c.applied_fetch_id
				AND f.subscription_id=c.subscription_id AND f.outcome IN ('success','partial'))))`,
		"invalid_last_attempt": `SELECT COUNT(*) FROM subscription_current_state c WHERE
			(c.last_attempt_fetch_id IS NULL AND c.last_attempt_status != '') OR
			(c.last_attempt_fetch_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM subscription_fetches f
			 WHERE f.id=c.last_attempt_fetch_id AND f.subscription_id=c.subscription_id AND f.outcome=c.last_attempt_status))`,
		"invalid_node_source_generation": `SELECT COUNT(*) FROM node_sources n WHERE NOT EXISTS (
			SELECT 1 FROM subscription_current_state c JOIN subscription_entries e
			ON e.payload_id=c.applied_payload_id AND e.subscription_id=c.subscription_id
			WHERE c.subscription_id=n.subscription_id AND c.applied_fetch_id=n.last_seen_fetch_id
			AND e.node_logical_id=n.node_logical_id)`,
		"unrepresented_current_payload": `SELECT COUNT(*) FROM subscription_payloads p WHERE NOT EXISTS (
			SELECT 1 FROM subscription_current_state c WHERE c.subscription_id=p.subscription_id AND c.applied_payload_id=p.id)`,
	} {
		var bad int64
		if err := tx.QueryRowContext(ctx, q).Scan(&bad); err != nil {
			return nil, err
		}
		if bad != 0 {
			report.Failures = append(report.Failures, name)
		}
	}
	report.Verified = len(report.Failures) == 0
	return report, nil
}

func redirectPublicationEvidenceTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE publication_payload_refs_evidence (
		publication_id TEXT NOT NULL REFERENCES publications(id) ON DELETE CASCADE,
		payload_id TEXT NOT NULL REFERENCES subscription_payload_evidence(payload_id) ON DELETE RESTRICT,
		PRIMARY KEY(publication_id,payload_id));
		INSERT INTO publication_payload_refs_evidence SELECT publication_id,payload_id FROM publication_payload_refs;
		DROP TABLE publication_payload_refs;
		ALTER TABLE publication_payload_refs_evidence RENAME TO publication_payload_refs;
		CREATE INDEX idx_pub_payload_refs_payload ON publication_payload_refs(payload_id);`)
	return err
}

func quoteCompactionIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// Hash canonical ordered complete rows, retaining SQLite value types. No row
// values are returned to the caller or included in error messages.
func digestCompactionTable(ctx context.Context, tx *sql.Tx, name string) (int64, string, error) {
	return digestCompactionSource(ctx, tx, name, quoteCompactionIdentifier(name))
}

func digestCompactionSource(ctx context.Context, tx *sql.Tx, name, table string) (int64, string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT * FROM "+table+" LIMIT 0")
	if err != nil {
		return 0, "", err
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return 0, "", err
	}
	quoted := make([]string, len(columns))
	for i, col := range columns {
		quoted[i] = quoteCompactionIdentifier(col) + " COLLATE BINARY"
	}
	// A non-NULL BINARY unique prefix already fixes the complete canonical
	// ordering. Truncating only a proven prefix lets SQLite stream wide rows
	// from its index instead of sorting their entire historical config bodies.
	if table == quoteCompactionIdentifier(name) {
		prefix, err := compactionUniqueOrderPrefix(ctx, tx, name, columns)
		if err != nil {
			return 0, "", err
		}
		if prefix > 0 {
			quoted = quoted[:prefix]
		}
	}
	rows, err = tx.QueryContext(ctx, "SELECT * FROM "+table+" ORDER BY "+strings.Join(quoted, ","))
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	h := sha256.New()
	if err := writeCompactionDigest(h, columns); err != nil {
		return 0, "", err
	}
	var count int64
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return 0, "", err
		}
		// JSON alone would encode a BLOB and a TEXT containing its base64
		// identically. Tag driver value types so those changes are detected.
		typed := make([]any, len(values))
		for i, value := range values {
			typed[i] = []any{fmt.Sprintf("%T", value), value}
		}
		if err := writeCompactionDigest(h, typed); err != nil {
			return 0, "", err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	return count, hex.EncodeToString(h.Sum(nil)), nil
}

func writeCompactionDigest(h hash.Hash, value any) error {
	// JSON's newline framing distinguishes row/column boundaries and nulls.
	return json.NewEncoder(h).Encode(value)
}
