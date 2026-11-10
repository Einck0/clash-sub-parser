package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"clash-sub-parser/internal/domain"
)

// SubscriptionCompactionDecision is an explicit, snapshot-bound reconciliation
// decision, never an instruction to select whichever row happens to be newest.
// Empty PayloadID is allowed only for a subscription with no useful generation.
// SourceRevision is the snapshot cut fence. Existing saved raw/state must match;
// legacy schema27 absence is allowed and never fabricated as historical proof.
type SubscriptionCompactionDecision struct {
	SubscriptionID string
	SourceRevision string
	PayloadID      string
}

// FinalizeSubscriptionCompaction transforms an already prepared schema-30
// OFFLINE SCRATCH database in one transaction. This is deliberately NOT a CLI
// entry point: the future file orchestrator must prove authorized scratch,
// independent immutable backup, exclusive access, capacity and drain BEFORE
// opening it. It must never be called by startup or against a live database.
//
// Legacy reconciliation is bound to exact planned source/history projections.
// It does not migrate schema, reclaim versions, VACUUM, switch files or approve
// deployment. The returned inventory is not a consumer-closure safety seal.
func FinalizeSubscriptionCompaction(ctx context.Context, db *sql.DB, baseline *domain.SubscriptionStoragePlan, decisions []SubscriptionCompactionDecision) (*domain.SubscriptionStoragePlan, error) {
	if db == nil || baseline == nil || baseline.Version != 1 || baseline.SchemaVersion != 30 || baseline.SnapshotDigest == "" || !baseline.ApplySupported {
		return nil, fmt.Errorf("maintenance_required: prepared scratch baseline required")
	}
	var result *domain.SubscriptionStoragePlan
	err := WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		// Reserve the SQLite write lock before fingerprinting/validating. A
		// competing connection must not change a decision's inputs mid-apply.
		if _, err := tx.ExecContext(ctx, `UPDATE subscription_storage_readiness SET consumers_closed=consumers_closed WHERE singleton=1`); err != nil {
			return err
		}
		var fk, seals, leases int
		if err := tx.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
			return fmt.Errorf("maintenance_required: foreign key enforcement required")
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_storage_readiness`).Scan(&seals); err != nil || seals != 0 {
			return fmt.Errorf("maintenance_required: scratch must be prepared and unsealed")
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_connection_version_leases`).Scan(&leases); err != nil || leases != 0 {
			return fmt.Errorf("maintenance_required: unresolved version consumers")
		}
		before, err := inspectSubscriptionCompactionTx(ctx, tx)
		if err != nil {
			return err
		}
		if before.SchemaVersion != 30 || !reflect.DeepEqual(before, baseline) {
			return fmt.Errorf("maintenance_required: scratch snapshot changed after planning")
		}
		selected := make(map[string]SubscriptionCompactionDecision, len(decisions))
		for _, decision := range decisions {
			if decision.SubscriptionID == "" || decision.SourceRevision == "" {
				return fmt.Errorf("maintenance_required: complete source decisions required")
			}
			if _, exists := selected[decision.SubscriptionID]; exists {
				return fmt.Errorf("maintenance_required: duplicate source decision")
			}
			selected[decision.SubscriptionID] = decision
		}
		if len(selected) != len(before.Subscriptions) {
			return fmt.Errorf("maintenance_required: exact subscription decision coverage required")
		}
		// Resolve every failure branch before detaching or deleting any facts.
		for _, status := range before.Subscriptions {
			decision, ok := selected[status.SubscriptionID]
			if !ok {
				return fmt.Errorf("maintenance_required: missing source decision")
			}
			if err := validateCompactionDecisionTx(ctx, tx, decision); err != nil {
				return err
			}
		}
		if err := requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM pragma_foreign_key_check`, "input foreign key violation"); err != nil {
			return err
		}
		if err := requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM subscription_entries e WHERE NOT EXISTS(SELECT 1 FROM subscription_payloads p WHERE p.id=e.payload_id AND p.subscription_id=e.subscription_id)`, "cross-source input entry"); err != nil {
			return err
		}
		if err := reconcileCompactionSourcesTx(ctx, tx, baseline); err != nil {
			return err
		}
		if err := requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM node_sources ns WHERE NOT EXISTS (SELECT 1 FROM subscription_entries e JOIN subscription_payloads p ON p.id=e.payload_id WHERE e.subscription_id=ns.subscription_id AND e.node_logical_id=ns.node_logical_id AND p.fetch_id=ns.last_seen_fetch_id)`, "unreconciled source membership"); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		// Only actual pin/publication/version consumers get evidence rows.
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_payload_evidence
			(payload_id,subscription_id,source_identity,fetch_id,content_digest,original_created_at,captured_at,http_status,evidence_kind,quality,pinned)
			SELECT p.id,p.subscription_id,'sub:'||p.subscription_id,p.fetch_id,p.content_digest,p.created_at,?,p.http_status,'payload_origin','observed',p.pinned
			FROM subscription_payloads p WHERE p.pinned=1 OR EXISTS(SELECT 1 FROM publication_payload_refs r WHERE r.payload_id=p.id)
			OR EXISTS(SELECT 1 FROM node_connection_versions v JOIN subscription_entries e ON e.id=v.source_entry_id WHERE e.payload_id=p.id)`, now); err != nil {
			// Existing conflicting evidence is not silently ignored.
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_connection_provenance(node_logical_id,connection_revision,origin_payload_id,created_at)
			SELECT v.node_logical_id,v.connection_revision,e.payload_id,v.created_at FROM node_connection_versions v JOIN subscription_entries e ON e.id=v.source_entry_id`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_source_observations(node_logical_id,source_identity,subscription_id,raw_name,protocol,source_key,observed_at)
			SELECT e.node_logical_id,'sub:'||e.subscription_id,e.subscription_id,e.raw_name,e.protocol,e.source_key,e.created_at FROM `+compactionLatestEntries+` e`); err != nil {
			return err
		}
		if err := redirectPublicationEvidenceTx(ctx, tx); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE node_connection_versions SET source_entry_id=NULL WHERE source_entry_id IS NOT NULL`); err != nil {
			return err
		}
		for _, status := range before.Subscriptions {
			decision := selected[status.SubscriptionID]
			var payload, appliedFetch, appliedAt, lastFetch, lastStatus any
			if decision.PayloadID != "" {
				payload = decision.PayloadID
				var fetchID, createdAt string
				if err := tx.QueryRowContext(ctx, `SELECT fetch_id,created_at FROM subscription_payloads WHERE id=?`, decision.PayloadID).Scan(&fetchID, &createdAt); err != nil {
					return err
				}
				appliedFetch, appliedAt = fetchID, createdAt
			}
			if status.LastAttemptID != "" {
				lastFetch = status.LastAttemptID
				var outcome string
				if err := tx.QueryRowContext(ctx, `SELECT outcome FROM subscription_fetches WHERE id=?`, status.LastAttemptID).Scan(&outcome); err != nil {
					return err
				}
				lastStatus = outcome
			} else {
				lastStatus = ""
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_current_state(subscription_id,source_revision,applied_payload_id,applied_fetch_id,last_attempt_fetch_id,last_attempt_status,applied_at)
				VALUES(?,?,?,?,?,?,?) ON CONFLICT(subscription_id) DO UPDATE SET source_revision=excluded.source_revision,applied_payload_id=excluded.applied_payload_id,applied_fetch_id=excluded.applied_fetch_id,last_attempt_fetch_id=excluded.last_attempt_fetch_id,last_attempt_status=excluded.last_attempt_status,applied_at=excluded.applied_at`, decision.SubscriptionID, decision.SourceRevision, payload, appliedFetch, lastFetch, lastStatus, appliedAt); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM subscription_payloads WHERE subscription_id=? AND id!=?`, decision.SubscriptionID, decision.PayloadID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE subscription_payloads SET body_blob=X''`); err != nil {
			return err
		}
		if err := ensureCompactionHeadsTx(ctx, tx); err != nil {
			return err
		}
		if _, err := PruneFetchSummariesTx(ctx, tx, ""); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX idx_subscription_single_payload ON subscription_payloads(subscription_id)`); err != nil {
			return err
		}
		for _, ddl := range subscriptionStorageTriggers {
			if _, err := tx.ExecContext(ctx, ddl); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO subscription_storage_readiness(singleton,contract_version,consumers_closed,sealed_at)VALUES(1,1,0,?)`, now); err != nil {
			return err
		}
		if err := checkSubscriptionStorageReadyTx(ctx, tx); err != nil {
			return err
		}
		var integrity string
		if err := tx.QueryRowContext(ctx, `PRAGMA integrity_check(1)`).Scan(&integrity); err != nil || integrity != "ok" {
			return fmt.Errorf("maintenance_required: scratch integrity failed")
		}
		if err := requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM pragma_foreign_key_check`, "scratch foreign key violation"); err != nil {
			return err
		}
		result, err = inspectSubscriptionCompactionTx(ctx, tx)
		if err != nil {
			return err
		}
		if !compactionProtectedMatch(result, baseline) {
			return fmt.Errorf("maintenance_required: protected asset changed")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func requireCompactionZeroTx(ctx context.Context, tx *sql.Tx, query, reason string, args ...any) error {
	var count int64
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return fmt.Errorf("maintenance_required: %s", reason)
	}
	return nil
}

func validateCompactionDecisionTx(ctx context.Context, tx *sql.Tx, decision SubscriptionCompactionDecision) error {
	var revision string
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM subscriptions WHERE id=?`, decision.SubscriptionID).Scan(&revision); err != nil {
		return err
	}
	if revision != decision.SourceRevision {
		return fmt.Errorf("maintenance_required: source revision changed")
	}
	// Existing applied facts are independent evidence, not a cache that raw
	// proof may overwrite. Even matching raw cannot resolve a contradiction.
	if err := requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM subscription_current_state WHERE subscription_id=? AND
		(source_revision!=? OR (applied_payload_id IS NULL)!=(applied_fetch_id IS NULL) OR
		(applied_payload_id IS NOT NULL AND (applied_payload_id!=? OR NOT EXISTS(SELECT 1 FROM subscription_payloads p WHERE p.id=applied_payload_id AND p.subscription_id=? AND p.fetch_id=applied_fetch_id))))`, "existing applied state contradicts source decision", decision.SubscriptionID, decision.SourceRevision, decision.PayloadID, decision.SubscriptionID); err != nil {
		return err
	}
	if decision.PayloadID == "" {
		return requireCompactionZeroTx(ctx, tx, `SELECT (SELECT COUNT(*) FROM subscription_payloads p JOIN subscription_fetches f ON f.id=p.fetch_id WHERE p.subscription_id=? AND f.outcome IN ('success','partial'))+(SELECT COUNT(*) FROM node_sources WHERE subscription_id=?)+(SELECT COUNT(*) FROM subscription_original_configs WHERE subscription_id=?)`, "empty decision has useful configuration", decision.SubscriptionID, decision.SubscriptionID, decision.SubscriptionID)
	}
	var fetchID string
	if err := tx.QueryRowContext(ctx, `SELECT p.fetch_id FROM subscription_payloads p JOIN subscription_fetches f ON f.id=p.fetch_id WHERE p.id=? AND p.subscription_id=? AND f.subscription_id=p.subscription_id AND f.outcome IN ('success','partial')`, decision.PayloadID, decision.SubscriptionID).Scan(&fetchID); err != nil {
		return fmt.Errorf("maintenance_required: selected generation is not useful")
	}
	// Legacy schema27 has no independent raw/state record. The source revision
	// is a cut fence, not invented historical identity. Existing optional facts
	// must still agree; absence is not a contradiction.
	if err := requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM subscription_original_configs c WHERE c.subscription_id=? AND (c.source_revision!=? OR c.fetch_id!=? OR NOT EXISTS(SELECT 1 FROM subscription_payloads p WHERE p.id=? AND p.content_digest=c.content_digest))`, "saved raw disagrees with selected generation", decision.SubscriptionID, decision.SourceRevision, fetchID, decision.PayloadID); err != nil {
		return err
	}
	// Do not let selecting an earlier successful fetch revive its members after
	// a later partial refresh. A later failure is distinct and may be retained.
	if err := requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM subscription_fetches f JOIN subscription_fetches chosen ON chosen.id=? WHERE f.subscription_id=? AND f.outcome IN ('success','partial') AND (julianday(f.started_at)>julianday(chosen.started_at) OR (julianday(f.started_at)=julianday(chosen.started_at) AND f.rowid>chosen.rowid))`, "newer useful attempt supersedes selected generation", fetchID, decision.SubscriptionID); err != nil {
		return err
	}
	// Stale relations are reconciled against the plan's exact projection only
	// after all decisions validate. Current relations must match selected entries.
	return requireCompactionZeroTx(ctx, tx, `SELECT COUNT(*) FROM node_sources ns WHERE ns.subscription_id=? AND ns.last_seen_fetch_id=? AND NOT EXISTS(SELECT 1 FROM subscription_entries e WHERE e.payload_id=? AND e.subscription_id=ns.subscription_id AND e.node_logical_id=ns.node_logical_id)`, "current membership disagrees with selected generation", decision.SubscriptionID, fetchID, decision.PayloadID)
}

func ensureCompactionHeadsTx(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT logical_id,server,port,config_json,connection_revision,created_at,updated_at FROM nodes ORDER BY logical_id`)
	if err != nil {
		return err
	}
	type current struct {
		id, server, config, created, updated string
		port                                 int
		revision                             int64
	}
	var nodes []current
	for rows.Next() {
		var n current
		if err := rows.Scan(&n.id, &n.server, &n.port, &n.config, &n.revision, &n.created, &n.updated); err != nil {
			rows.Close()
			return err
		}
		nodes = append(nodes, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, n := range nodes {
		creds, err := unmarshalNodeCredentials(n.config)
		// nodes has required positive current revisions since migration 12.
		// Historical version/observation zero remains valid; an invalid current
		// row must be rejected, never renumbered and made spuriously fresh.
		if err != nil || n.revision <= 0 {
			return fmt.Errorf("maintenance_required: invalid current node configuration")
		}
		effective, err := json.Marshal(map[string]any{"server": n.server, "port": n.port, "credentials": creds})
		if err != nil {
			return err
		}
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT effective_config_json FROM node_connection_versions WHERE node_logical_id=? AND connection_revision=?`, n.id, n.revision).Scan(&existing)
		if err == sql.ErrNoRows {
			if _, err = tx.ExecContext(ctx, `INSERT INTO node_connection_versions(node_logical_id,connection_revision,effective_config_json,config_fingerprint,schema_version,created_at)VALUES(?,?,?,?,1,?)`, n.id, n.revision, string(effective), domain.ComputeConnectionFingerprint(n.server, n.port, creds), n.created); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			var stored struct {
				Server      string                           `json:"server"`
				Port        int                              `json:"port"`
				Credentials domain.InboundProtocolCredential `json:"credentials"`
			}
			if json.Unmarshal([]byte(existing), &stored) != nil || stored.Server != n.server || stored.Port != n.port || !reflect.DeepEqual(stored.Credentials, creds) {
				return fmt.Errorf("maintenance_required: current version contradicts node")
			}
		}
		var head int64
		err = tx.QueryRowContext(ctx, `SELECT connection_revision FROM node_connection_heads WHERE logical_id=?`, n.id).Scan(&head)
		if err == sql.ErrNoRows {
			if _, err = tx.ExecContext(ctx, `INSERT INTO node_connection_heads(logical_id,connection_revision,updated_at)VALUES(?,?,?)`, n.id, n.revision, n.updated); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if head != n.revision {
			return fmt.Errorf("maintenance_required: current head contradicts node")
		}
	}
	return nil
}
