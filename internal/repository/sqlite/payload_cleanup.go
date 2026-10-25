package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// PayloadStorageReport captures storage metrics for raw subscription payload bodies,
// including total payloads, non-empty body counts, byte sizes, cleared counts,
// and row counts of critical retained business tables.
type PayloadStorageReport struct {
	PayloadCount      int64 `json:"payload_count"`
	NonemptyBodyCount int64 `json:"nonempty_body_count"`
	BodyBytes         int64 `json:"body_bytes"`
	ClearedBodyCount  int64 `json:"cleared_body_count"`
	ClearedBodyBytes  int64 `json:"cleared_body_bytes"`

	// Retained counts for critical business tables
	RetainedEntries       int64 `json:"retained_entries"`
	RetainedNodes         int64 `json:"retained_nodes"`
	RetainedSubscriptions int64 `json:"retained_subscriptions"`
	RetainedPublications  int64 `json:"retained_publications"`
	RetainedRefs          int64 `json:"retained_refs"`
}

var requiredPayloadStorageTables = []string{
	"subscription_payloads",
	"subscription_entries",
	"nodes",
	"subscriptions",
	"publications",
	"publication_payload_refs",
}

type storageCounts struct {
	payloadCount      int64
	nonemptyBodyCount int64
	bodyBytes         int64
	entryCount        int64
	nodeCount         int64
	subCount          int64
	pubCount          int64
	refCount          int64
}

func validateRequiredTables(ctx context.Context, q queryExecutor) error {
	for _, tbl := range requiredPayloadStorageTables {
		if !tableExists(ctx, q, tbl) {
			return fmt.Errorf("required table %q does not exist", tbl)
		}
	}
	return nil
}

func queryStorageCounts(ctx context.Context, q queryExecutor) (storageCounts, error) {
	var sc storageCounts

	const payloadSQL = `
	SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN body_blob IS NOT NULL AND LENGTH(body_blob) > 0 THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN body_blob IS NOT NULL AND LENGTH(body_blob) > 0 THEN LENGTH(body_blob) ELSE 0 END), 0)
	FROM subscription_payloads;`

	if err := q.QueryRowContext(ctx, payloadSQL).Scan(&sc.payloadCount, &sc.nonemptyBodyCount, &sc.bodyBytes); err != nil {
		return sc, fmt.Errorf("failed to query subscription_payloads storage stats: %w", err)
	}

	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_entries;").Scan(&sc.entryCount); err != nil {
		return sc, fmt.Errorf("failed to count subscription_entries: %w", err)
	}
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes;").Scan(&sc.nodeCount); err != nil {
		return sc, fmt.Errorf("failed to count nodes: %w", err)
	}
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscriptions;").Scan(&sc.subCount); err != nil {
		return sc, fmt.Errorf("failed to count subscriptions: %w", err)
	}
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM publications;").Scan(&sc.pubCount); err != nil {
		return sc, fmt.Errorf("failed to count publications: %w", err)
	}
	if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM publication_payload_refs;").Scan(&sc.refCount); err != nil {
		return sc, fmt.Errorf("failed to count publication_payload_refs: %w", err)
	}

	return sc, nil
}

func buildReport(payloadCount, nonemptyBodyCount, bodyBytes, clearedCount, clearedBytes int64, sc storageCounts) *PayloadStorageReport {
	return &PayloadStorageReport{
		PayloadCount:          payloadCount,
		NonemptyBodyCount:     nonemptyBodyCount,
		BodyBytes:             bodyBytes,
		ClearedBodyCount:      clearedCount,
		ClearedBodyBytes:      clearedBytes,
		RetainedEntries:       sc.entryCount,
		RetainedNodes:         sc.nodeCount,
		RetainedSubscriptions: sc.subCount,
		RetainedPublications:  sc.pubCount,
		RetainedRefs:          sc.refCount,
	}
}

// InspectPayloadStorage performs a read-only audit of payload body storage and retained tables.
// It does not modify any rows. If required schema tables are missing, it fails closed with an error.
func InspectPayloadStorage(ctx context.Context, db *sql.DB) (*PayloadStorageReport, error) {
	if db == nil {
		return nil, errors.New("db cannot be nil")
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		tx, err = db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to begin inspect transaction: %w", err)
		}
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := validateRequiredTables(ctx, tx); err != nil {
		return nil, err
	}

	sc, err := queryStorageCounts(ctx, tx)
	if err != nil {
		return nil, err
	}

	return buildReport(sc.payloadCount, sc.nonemptyBodyCount, sc.bodyBytes, 0, 0, sc), nil
}

// ClearPayloadBodies atomically zeroes out non-empty payload bodies (SET body_blob = X”)
// within a single transaction. It validates table existence, updates non-empty bodies, verifies that
// no non-empty bodies remain, ensures payload row count and retained business table counts
// remain unchanged, and runs a PRAGMA foreign_key_check before committing.
// If any check fails, the transaction rolls back. It does not perform VACUUM.
func ClearPayloadBodies(ctx context.Context, db *sql.DB) (*PayloadStorageReport, error) {
	if db == nil {
		return nil, errors.New("db cannot be nil")
	}

	tx, err := db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to begin clear transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := validateRequiredTables(ctx, tx); err != nil {
		return nil, err
	}

	preCounts, err := queryStorageCounts(ctx, tx)
	if err != nil {
		return nil, err
	}

	// Only update if there are non-empty bodies to clear
	if preCounts.nonemptyBodyCount > 0 {
		const updateSQL = `
		UPDATE subscription_payloads
		SET body_blob = X''
		WHERE body_blob IS NOT NULL AND LENGTH(body_blob) > 0;`

		if _, err := tx.ExecContext(ctx, updateSQL); err != nil {
			return nil, fmt.Errorf("failed to update subscription payload bodies: %w", err)
		}
	}

	// Post-validation: ensure non-empty body count is 0
	var postNonemptyCount int64
	const checkNonemptySQL = `
	SELECT COUNT(*) FROM subscription_payloads
	WHERE body_blob IS NOT NULL AND LENGTH(body_blob) > 0;`
	if err := tx.QueryRowContext(ctx, checkNonemptySQL).Scan(&postNonemptyCount); err != nil {
		return nil, fmt.Errorf("failed to verify post-cleanup body count: %w", err)
	}
	if postNonemptyCount != 0 {
		return nil, fmt.Errorf("post-cleanup verification failed: %d non-empty payload bodies remain", postNonemptyCount)
	}

	// Post-validation: ensure total payload count unchanged (no rows deleted)
	var postPayloadCount int64
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_payloads;").Scan(&postPayloadCount); err != nil {
		return nil, fmt.Errorf("failed to count post-cleanup payloads: %w", err)
	}
	if postPayloadCount != preCounts.payloadCount {
		return nil, fmt.Errorf("post-cleanup verification failed: payload count changed from %d to %d", preCounts.payloadCount, postPayloadCount)
	}

	// Post-validation: ensure retained business table counts unchanged
	postCounts, err := queryStorageCounts(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to query post-cleanup table counts: %w", err)
	}
	if postCounts.entryCount != preCounts.entryCount {
		return nil, fmt.Errorf("post-cleanup verification failed: subscription_entries count changed from %d to %d", preCounts.entryCount, postCounts.entryCount)
	}
	if postCounts.nodeCount != preCounts.nodeCount {
		return nil, fmt.Errorf("post-cleanup verification failed: nodes count changed from %d to %d", preCounts.nodeCount, postCounts.nodeCount)
	}
	if postCounts.subCount != preCounts.subCount {
		return nil, fmt.Errorf("post-cleanup verification failed: subscriptions count changed from %d to %d", preCounts.subCount, postCounts.subCount)
	}
	if postCounts.pubCount != preCounts.pubCount {
		return nil, fmt.Errorf("post-cleanup verification failed: publications count changed from %d to %d", preCounts.pubCount, postCounts.pubCount)
	}
	if postCounts.refCount != preCounts.refCount {
		return nil, fmt.Errorf("post-cleanup verification failed: publication_payload_refs count changed from %d to %d", preCounts.refCount, postCounts.refCount)
	}

	// Post-validation: foreign key integrity check within transaction
	fkRows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return nil, fmt.Errorf("failed to run foreign_key_check in cleanup transaction: %w", err)
	}
	defer fkRows.Close()

	var violations []string
	for fkRows.Next() {
		var table, parent sql.NullString
		var rowid sql.NullInt64
		var fkid sql.NullInt64
		if scanErr := fkRows.Scan(&table, &rowid, &parent, &fkid); scanErr != nil {
			violations = append(violations, fmt.Sprintf("unscannable foreign key violation row: %v", scanErr))
			continue
		}
		rowidStr := "NULL"
		if rowid.Valid {
			rowidStr = fmt.Sprintf("%d", rowid.Int64)
		}
		violations = append(violations, fmt.Sprintf("table %s (rowid %s) references parent %s (fkid %d)", table.String, rowidStr, parent.String, fkid.Int64))
	}
	if err := fkRows.Err(); err != nil {
		return nil, fmt.Errorf("error reading foreign_key_check results: %w", err)
	}
	if len(violations) > 0 {
		return nil, fmt.Errorf("foreign key integrity violation after payload cleanup: %s", strings.Join(violations, "; "))
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit payload cleanup transaction: %w", err)
	}

	return buildReport(
		preCounts.payloadCount,
		0,
		0,
		preCounts.nonemptyBodyCount,
		preCounts.bodyBytes,
		preCounts,
	), nil
}
