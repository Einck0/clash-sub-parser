package sqlite

import (
	"clash-sub-parser/internal/domain"
	"context"
	"database/sql"
	"fmt"
	"time"
)

type RetentionResult struct {
	SubscriptionID    string `json:"subscription_id"`
	DeletedPayloads   int64  `json:"deleted_payloads"`
	ProtectedPayloads int64  `json:"protected_payloads"`
	DeletedFetches    int64  `json:"deleted_fetches"`
	ProtectedFetches  int64  `json:"protected_fetches"`
}
type SubscriptionEntryOverride struct {
	SubscriptionID   string           `json:"subscription_id"`
	OverrideAnchor   string           `json:"override_anchor"`
	SourceKey        string           `json:"source_key"`
	RawName          string           `json:"raw_name"`
	UserKindOverride domain.EntryKind `json:"user_kind_override"`
	OverrideReason   string           `json:"override_reason"`
	OverrideAt       time.Time        `json:"override_at"`
	ActorRef         string           `json:"actor_ref"`
}

// PruneFetchSummariesTx runs inside the same transaction that advances state.
// Descriptive history/evidence fetch IDs intentionally do not require old rows.
func PruneFetchSummariesTx(ctx context.Context,tx *sql.Tx,subscriptionID string)(int64,error){
	res,err:=tx.ExecContext(ctx,`DELETE FROM subscription_fetches WHERE (subscription_id=? OR ?='')
	AND id NOT IN (SELECT applied_fetch_id FROM subscription_current_state WHERE applied_fetch_id IS NOT NULL)
	AND id NOT IN (SELECT last_attempt_fetch_id FROM subscription_current_state WHERE last_attempt_fetch_id IS NOT NULL)
	AND id NOT IN (SELECT fetch_id FROM subscription_original_configs)
	AND id NOT IN (SELECT fetch_id FROM subscription_payloads)
	AND id NOT IN (SELECT last_seen_fetch_id FROM node_sources WHERE last_seen_fetch_id!='')`,subscriptionID,subscriptionID)
	if err!=nil{return 0,err};return res.RowsAffected()
}

// PruneSubscriptionRetention never converts legacy generations. Replacement is
// now atomic in reconcile; standalone retention can only trim unreferenced fetch
// summaries after checking the sealed contract. Pins/publications do not exempt
// an extra parsed payload. maxPayloads/olderThan are retained for caller ABI only.
func PruneSubscriptionRetention(ctx context.Context, db *sql.DB, subscriptionID string, maxPayloads int, olderThan *time.Time) (*RetentionResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	if err := CheckSubscriptionStorageReady(ctx, db); err != nil {
		return nil, err
	}
	result := &RetentionResult{SubscriptionID: subscriptionID}
	err := WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		var invalid int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT subscription_id FROM subscription_payloads GROUP BY subscription_id HAVING COUNT(*)>1)`).Scan(&invalid); err != nil {
			return err
		}
		if invalid != 0 {
			return fmt.Errorf("maintenance_required: multiple parsed generations")
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id=? OR ?=''`, subscriptionID, subscriptionID).Scan(&result.ProtectedPayloads); err != nil {
			return err
		}
		// Refuse unexpected dangling current source references instead of erasing
		// diagnostics needed by an unconverted reader.
		var err error
		result.DeletedFetches,err=PruneFetchSummariesTx(ctx,tx,subscriptionID)
		if err!=nil{return err}
		return tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_fetches WHERE subscription_id=? OR ?=''`, subscriptionID, subscriptionID).Scan(&result.ProtectedFetches)
	})
	return result, err
}
