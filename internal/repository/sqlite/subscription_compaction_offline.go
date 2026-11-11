package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/migrations"
)

var errCompactionSelectionUnresolved = errors.New("maintenance_required: generation or membership unresolved")

// The cut chooses a useful generation, never a later failed attempt. Fetch
// ordering matches existing retention semantics; ties use the captured rowid.
const compactionLatestUseful = `(SELECT f.id FROM subscription_fetches f WHERE f.subscription_id=ns.subscription_id AND f.outcome IN ('success','partial') ORDER BY julianday(f.started_at) DESC,f.rowid DESC LIMIT 1)`
const compactionCurrentSources = `(SELECT ns.* FROM node_sources ns WHERE ns.last_seen_fetch_id=` + compactionLatestUseful + `)`

// Rank once rather than running a correlated scan for every historical entry
// in a multi-million-row schema27 database.
// Sort only the winner's identity and ordering fields, not historical config
// blobs. Join complete rows after ranking, preserving the exact tie-break rule.
// Intentionally scan the narrow rank input sequentially: the noncovering node
// index otherwise causes random lookups into wide historical rows.
const compactionLatestEntries = `(SELECT e.* FROM subscription_entries e JOIN (
 SELECT entry_rowid FROM (SELECT rowid AS entry_rowid,ROW_NUMBER() OVER (
 PARTITION BY node_logical_id,subscription_id ORDER BY julianday(created_at) DESC,rowid DESC
 ) AS compaction_rank FROM subscription_entries NOT INDEXED WHERE node_logical_id IS NOT NULL)
 WHERE compaction_rank=1) winners ON e.rowid=winners.entry_rowid)`
const compactionHistoryAdditions = `SELECT 'observation:'||e.node_logical_id||':sub:'||e.subscription_id AS id,
 e.node_logical_id,e.subscription_id,'sub:'||e.subscription_id AS source_identity,s.name AS source_label,
 NULL AS connection_revision,'unknown' AS relation_state,'unresolved' AS cause,
 e.created_at AS first_observed_at,e.created_at AS last_observed_at,
 'retention_entry_observation' AS evidence_kind,'observation:sub:'||e.subscription_id AS evidence_key,
 '{}' AS evidence_json,e.created_at
 FROM ` + compactionLatestEntries + ` e JOIN subscriptions s ON s.id=e.subscription_id
 WHERE NOT EXISTS(SELECT 1 FROM node_source_history h WHERE h.node_logical_id=e.node_logical_id AND h.source_identity='sub:'||e.subscription_id)`
const compactionProjectedHistory = `(SELECT * FROM node_source_history UNION ALL ` + compactionHistoryAdditions + `)`

// CROSS JOIN alone is insufficient: SQLite can still choose a subscription or
// node index and scan years of entries per source. This schema15 unique index
// (unchanged through schema30) bounds the inner lookup to one payload's entries.
const compactionSourceMembership = `SELECT 1 FROM subscription_payloads p
 CROSS JOIN subscription_entries e INDEXED BY sqlite_autoindex_subscription_entries_2 ON e.payload_id=p.id
 WHERE p.subscription_id=ns.subscription_id AND p.fetch_id=ns.last_seen_fetch_id
 AND e.subscription_id=ns.subscription_id AND e.node_logical_id=ns.node_logical_id`

func checkCompactionMembershipIndex(ctx context.Context, tx *sql.Tx) error {
	var valid int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_index_list('subscription_entries')
 WHERE name='sqlite_autoindex_subscription_entries_2' AND "unique"=1 AND partial=0`).Scan(&valid); err != nil {
		return err
	}
	if valid != 1 {
		return errors.New("compaction membership index unavailable")
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_index_xinfo('sqlite_autoindex_subscription_entries_2')
 WHERE "key"=1 AND cid>=0 AND "desc"=0 AND coll='BINARY'
 AND ((seqno=0 AND name='payload_id') OR (seqno=1 AND name='ordinal'))`).Scan(&valid); err != nil {
		return err
	}
	var keys int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_index_xinfo('sqlite_autoindex_subscription_entries_2') WHERE "key"=1`).Scan(&keys); err != nil {
		return err
	}
	if valid != 2 || keys != 2 {
		return errors.New("compaction membership index unsupported")
	}
	return nil
}

func selectCompactionGenerationsTx(ctx context.Context, tx *sql.Tx, plan *domain.SubscriptionStoragePlan) error {
	if err := checkCompactionMembershipIndex(ctx, tx); err != nil {
		return err
	}
	var invalidTime int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_fetches WHERE julianday(started_at) IS NULL`).Scan(&invalidTime); err != nil {
		return err
	}
	if invalidTime != 0 {
		return errCompactionSelectionUnresolved
	}
	for i := range plan.Subscriptions {
		s := &plan.Subscriptions[i]
		if err := tx.QueryRowContext(ctx, `SELECT revision FROM subscriptions WHERE id=?`, s.SubscriptionID).Scan(&s.SourceRevision); err != nil {
			return err
		}
		if s.SourceRevision == "" {
			return errCompactionSelectionUnresolved
		}
		var fetch string
		err := tx.QueryRowContext(ctx, `SELECT id FROM subscription_fetches WHERE subscription_id=? AND outcome IN ('success','partial') ORDER BY julianday(started_at) DESC,rowid DESC LIMIT 1`, s.SubscriptionID).Scan(&fetch)
		if err == sql.ErrNoRows {
			if s.PayloadCount != 0 {
				return errCompactionSelectionUnresolved
			}
			continue
		}
		if err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(id),'') FROM subscription_payloads WHERE subscription_id=? AND fetch_id=?`, s.SubscriptionID, fetch).Scan(&count, &s.SelectedPayloadID); err != nil {
			return err
		}
		if count != 1 {
			return errCompactionSelectionUnresolved
		}
	}
	// A relation may be removed only if it is demonstrably from an older useful
	// fetch and really has a matching entry. Unknown relations are not guessed.
	var unexplained int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_sources ns WHERE
 NOT EXISTS(SELECT 1 FROM subscription_fetches f WHERE f.id=ns.last_seen_fetch_id AND f.subscription_id=ns.subscription_id AND f.outcome IN ('success','partial'))
 OR NOT EXISTS(`+compactionSourceMembership+`)
 OR NOT EXISTS(SELECT 1 FROM subscription_fetches f WHERE f.id=`+compactionLatestUseful+`)`).Scan(&unexplained); err != nil {
		return err
	}
	if unexplained != 0 {
		return errCompactionSelectionUnresolved
	}
	return nil
}

// PrepareOfflineSubscriptionCompaction is exclusively for an authorized,
// drained, independently backed-up scratch. It never opens files or bypasses
// the normal startup guard. Caller must hold exclusive SQLite ownership.
func PrepareOfflineSubscriptionCompaction(ctx context.Context, db *sql.DB, original *domain.SubscriptionStoragePlan) (*domain.SubscriptionStoragePlan, []SubscriptionCompactionDecision, error) {
	if original == nil || !original.ApplySupported {
		return nil, nil, fmt.Errorf("maintenance_required: supported original plan required")
	}
	current, err := InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	if current.SnapshotDigest != original.SnapshotDigest || !reflect.DeepEqual(current, original) {
		return nil, nil, fmt.Errorf("maintenance_required: original snapshot changed")
	}
	if err := NewMigrationRunner(db, migrations.FS).RunUpTo(ctx, 30); err != nil {
		return nil, nil, err
	}
	prepared, err := InspectSubscriptionCompaction(ctx, db)
	if err != nil {
		return nil, nil, err
	}
	// Migration must not alter any original protected business content.
	for name, digest := range original.ProtectedDigests {
		if prepared.ProtectedDigests[name] != digest {
			return nil, nil, fmt.Errorf("maintenance_required: migration changed protected content")
		}
	}
	decisions := make([]SubscriptionCompactionDecision, 0, len(original.Subscriptions))
	for _, s := range original.Subscriptions {
		decisions = append(decisions, SubscriptionCompactionDecision{SubscriptionID: s.SubscriptionID, SourceRevision: s.SourceRevision, PayloadID: s.SelectedPayloadID})
	}
	return prepared, decisions, nil
}

func reconcileCompactionSourcesTx(ctx context.Context, tx *sql.Tx, baseline *domain.SubscriptionStoragePlan) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO node_source_history `+compactionHistoryAdditions); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM node_sources AS ns WHERE ns.last_seen_fetch_id!=`+compactionLatestUseful); err != nil {
		return err
	}
	for _, item := range []struct {
		table, digest string
		count         int64
	}{
		{"node_sources", baseline.ReconciledSourcesDigest, baseline.ReconciledSourcesCount},
		{"node_source_history", baseline.ReconciledHistoryDigest, baseline.ReconciledHistoryCount},
	} {
		count, digest, err := digestCompactionTable(ctx, tx, item.table)
		if err != nil {
			return err
		}
		if item.digest == "" || digest != item.digest || count != item.count {
			return fmt.Errorf("maintenance_required: reconciliation projection mismatch")
		}
	}
	return nil
}

func compactionProtectedMatch(result, baseline *domain.SubscriptionStoragePlan) bool {
	for name, want := range baseline.ProtectedDigests {
		if name == "node_sources" {
			want = baseline.ReconciledSourcesDigest
		}
		if name == "node_source_history" {
			want = baseline.ReconciledHistoryDigest
		}
		if result.ProtectedDigests[name] != want {
			return false
		}
	}
	return len(result.ProtectedDigests) == len(baseline.ProtectedDigests)
}
