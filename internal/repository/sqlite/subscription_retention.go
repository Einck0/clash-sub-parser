package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

// RetentionResult summarizes observable counts of pruned and protected payloads and fetches.
type RetentionResult struct {
	SubscriptionID    string `json:"subscription_id"`
	DeletedPayloads   int64  `json:"deleted_payloads"`
	ProtectedPayloads int64  `json:"protected_payloads"`
	DeletedFetches    int64  `json:"deleted_fetches"`
	ProtectedFetches  int64  `json:"protected_fetches"`
}

// SubscriptionEntryOverride represents a durable user classification override intent.
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

// PruneSubscriptionRetention executes reference-safe transactional payload and fetch retention.
// It keeps:
//   - Newest two useful success/partial payloads per subscription (deterministic created_at + rowid order)
//   - Newest payload of any outcome per subscription
//   - All explicit pins (pinned = 1)
//   - All publication payload references (publication_payload_refs)
//   - Any payload containing a node_connection_versions.source_entry_id
//   - Current node sources fetch references (node_sources.last_seen_fetch_id)
//   - Payloads whose entries are the only provenance preventing orphan nodes from becoming implicitly manual
//   - Latest 100 fetch summaries per subscription plus referenced fetches
//
// Old unprotected payloads are deleted in bounded batches (at most maxPayloads, e.g. 20).
// If subscriptionID is empty, it evaluates across all subscriptions.
// If olderThan is non-nil and non-zero, only candidate payloads created before olderThan are considered.
// Any error encountered when reading protection sets fails the transaction to prevent data loss.
func PruneSubscriptionRetention(
	ctx context.Context,
	db *sql.DB,
	subscriptionID string,
	maxPayloads int,
	olderThan *time.Time,
) (*RetentionResult, error) {
	if db == nil {
		return nil, errors.New("db cannot be nil")
	}

	cutoffStr := ""
	if olderThan != nil && !olderThan.IsZero() {
		cutoffStr = olderThan.Format(time.RFC3339)
	}

	res := &RetentionResult{
		SubscriptionID: subscriptionID,
	}

	err := WithTx(ctx, db, func(ctx context.Context, tx *sql.Tx) error {
		// 1. Collect protected payload IDs (exact deterministic protection rules)
		protectedSet := make(map[string]bool)

		// 1.1 Pins
		pinQuery := "SELECT id FROM subscription_payloads WHERE pinned = 1 AND (subscription_id = ? OR ? = '');"
		pinRows, err := tx.QueryContext(ctx, pinQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query pinned payloads: %w", err)
		}
		for pinRows.Next() {
			var id string
			if err := pinRows.Scan(&id); err != nil {
				pinRows.Close()
				return fmt.Errorf("failed to scan pinned payload: %w", err)
			}
			protectedSet[id] = true
		}
		if err := pinRows.Err(); err != nil {
			pinRows.Close()
			return fmt.Errorf("error reading pinned payloads: %w", err)
		}
		pinRows.Close()

		// 1.2 Publication references
		pubQuery := `
		SELECT ppr.payload_id
		FROM publication_payload_refs ppr
		JOIN subscription_payloads p ON ppr.payload_id = p.id
		WHERE (p.subscription_id = ? OR ? = '');`
		pubRows, err := tx.QueryContext(ctx, pubQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query publication payload refs: %w", err)
		}
		for pubRows.Next() {
			var id string
			if err := pubRows.Scan(&id); err != nil {
				pubRows.Close()
				return fmt.Errorf("failed to scan publication payload ref: %w", err)
			}
			protectedSet[id] = true
		}
		if err := pubRows.Err(); err != nil {
			pubRows.Close()
			return fmt.Errorf("error reading publication payload refs: %w", err)
		}
		pubRows.Close()

		// 1.3 Current node sources fetch references
		nsQuery := `
		SELECT DISTINCT p.id
		FROM subscription_payloads p
		JOIN node_sources ns ON p.fetch_id = ns.last_seen_fetch_id
		WHERE ns.last_seen_fetch_id != ''
		  AND (p.subscription_id = ? OR ? = '');`
		nsRows, err := tx.QueryContext(ctx, nsQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query node sources fetch payloads: %w", err)
		}
		for nsRows.Next() {
			var id string
			if err := nsRows.Scan(&id); err != nil {
				nsRows.Close()
				return fmt.Errorf("failed to scan node sources fetch payload: %w", err)
			}
			protectedSet[id] = true
		}
		if err := nsRows.Err(); err != nil {
			nsRows.Close()
			return fmt.Errorf("error reading node sources fetch payloads: %w", err)
		}
		nsRows.Close()

		// 1.4 Newest payload of ANY outcome per subscription (deterministic created_at DESC, rowid DESC)
		newestAnyQuery := `
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY subscription_id
				ORDER BY created_at DESC, rowid DESC
			) AS rn
			FROM subscription_payloads
			WHERE (subscription_id = ? OR ? = '')
		) ranked WHERE ranked.rn = 1;`
		anyRows, err := tx.QueryContext(ctx, newestAnyQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query newest payload of any outcome: %w", err)
		}
		for anyRows.Next() {
			var id string
			if err := anyRows.Scan(&id); err != nil {
				anyRows.Close()
				return fmt.Errorf("failed to scan newest payload of any outcome: %w", err)
			}
			protectedSet[id] = true
		}
		if err := anyRows.Err(); err != nil {
			anyRows.Close()
			return fmt.Errorf("error reading newest payload of any outcome: %w", err)
		}
		anyRows.Close()

		// 1.5 Newest TWO payloads of useful SUCCESS or PARTIAL outcome per subscription
		// Uses mandatory migrated facts (JOIN subscription_fetches sf ON p.fetch_id = sf.id) without weak fallback.
		newestSuccessQuery := `
		SELECT id FROM (
			SELECT p.id, ROW_NUMBER() OVER (
				PARTITION BY p.subscription_id
				ORDER BY p.created_at DESC, p.rowid DESC
			) AS rn
			FROM subscription_payloads p
			JOIN subscription_fetches sf ON p.fetch_id = sf.id
			WHERE (p.subscription_id = ? OR ? = '')
			  AND sf.outcome IN ('success', 'partial')
			  AND (sf.nodes_valid > 0 OR EXISTS (
			      SELECT 1 FROM subscription_entries se_check WHERE se_check.payload_id = p.id
			  ))
		) ranked WHERE ranked.rn <= 2;`
		succRows, err := tx.QueryContext(ctx, newestSuccessQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query newest useful success payloads: %w", err)
		}
		for succRows.Next() {
			var id string
			if err := succRows.Scan(&id); err != nil {
				succRows.Close()
				return fmt.Errorf("failed to scan newest useful success payload: %w", err)
			}
			protectedSet[id] = true
		}
		if err := succRows.Err(); err != nil {
			succRows.Close()
			return fmt.Errorf("error reading newest useful success payloads: %w", err)
		}
		succRows.Close()

		// 1.6 Payloads containing a node_connection_versions.source_entry_id
		// Indexes: idx_node_conn_ver_source_entry and subscription_entries PK.
		ncvQuery := `
		SELECT DISTINCT se.payload_id
		FROM node_connection_versions ncv
		JOIN subscription_entries se ON ncv.source_entry_id = se.id
		WHERE (se.subscription_id = ? OR ? = '')
		  AND ncv.source_entry_id IS NOT NULL;`
		ncvRows, err := tx.QueryContext(ctx, ncvQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query connection version source entry payloads: %w", err)
		}
		for ncvRows.Next() {
			var id string
			if err := ncvRows.Scan(&id); err != nil {
				ncvRows.Close()
				return fmt.Errorf("failed to scan connection version payload: %w", err)
			}
			protectedSet[id] = true
		}
		if err := ncvRows.Err(); err != nil {
			ncvRows.Close()
			return fmt.Errorf("error reading connection version payloads: %w", err)
		}
		ncvRows.Close()

		// 1.7 Orphan node provenance protection
		// Payloads whose entries are the only provenance preventing orphan nodes from becoming implicitly manual.
		orphanQuery := `
		SELECT DISTINCT se.payload_id
		FROM nodes n
		JOIN subscription_entries se ON se.node_logical_id = n.logical_id
		WHERE (se.subscription_id = ? OR ? = '')
		  AND NOT EXISTS (SELECT 1 FROM node_sources ns WHERE ns.node_logical_id = n.logical_id)
		  AND NOT EXISTS (SELECT 1 FROM node_source_history h WHERE h.node_logical_id = n.logical_id);`
		orphanRows, err := tx.QueryContext(ctx, orphanQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query orphan provenance payloads: %w", err)
		}
		for orphanRows.Next() {
			var id string
			if err := orphanRows.Scan(&id); err != nil {
				orphanRows.Close()
				return fmt.Errorf("failed to scan orphan provenance payload: %w", err)
			}
			protectedSet[id] = true
		}
		if err := orphanRows.Err(); err != nil {
			orphanRows.Close()
			return fmt.Errorf("error reading orphan provenance payloads: %w", err)
		}
		orphanRows.Close()

		// 2. Select candidates to delete using SQL safe candidate anti-ref checks and LIMIT.
		// Excludes all protected categories in SQL and orders oldest first (created_at ASC, rowid ASC).
		// Bounded by maxPayloads (e.g. 20) to prevent unbounded memory load and whole-backlog scans.
		// Makes forward progress even when the oldest snapshots are protected (no starvation).
		limitVal := maxPayloads
		if limitVal <= 0 {
			limitVal = -1
		}

		candidateSQL := `
		SELECT p.id
		FROM subscription_payloads p
		WHERE (p.subscription_id = ? OR ? = '')
		  AND (? = '' OR p.created_at < ?)
		  AND p.pinned = 0
		  AND p.id NOT IN (
		      SELECT id FROM (
		          SELECT id, ROW_NUMBER() OVER (
		              PARTITION BY subscription_id
		              ORDER BY created_at DESC, rowid DESC
		          ) AS rn
		          FROM subscription_payloads
		          WHERE (subscription_id = ? OR ? = '')
		      ) ranked_any WHERE ranked_any.rn = 1
		  )
		  AND p.id NOT IN (
		      SELECT id FROM (
		          SELECT p2.id, ROW_NUMBER() OVER (
		              PARTITION BY p2.subscription_id
		              ORDER BY p2.created_at DESC, p2.rowid DESC
		          ) AS rn
		          FROM subscription_payloads p2
		          JOIN subscription_fetches sf ON p2.fetch_id = sf.id
		          WHERE (p2.subscription_id = ? OR ? = '')
		            AND sf.outcome IN ('success', 'partial')
		            AND (sf.nodes_valid > 0 OR EXISTS (
		                SELECT 1 FROM subscription_entries se_check WHERE se_check.payload_id = p2.id
		            ))
		      ) ranked_succ WHERE ranked_succ.rn <= 2
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM publication_payload_refs ppr WHERE ppr.payload_id = p.id
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM node_sources ns
		      WHERE ns.last_seen_fetch_id = p.fetch_id AND ns.last_seen_fetch_id != ''
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM subscription_entries se
		      JOIN node_connection_versions ncv ON ncv.source_entry_id = se.id
		      WHERE se.payload_id = p.id AND ncv.source_entry_id IS NOT NULL
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM subscription_entries se
		      WHERE se.payload_id = p.id
		        AND se.node_logical_id IS NOT NULL
		        AND NOT EXISTS (SELECT 1 FROM node_sources ns WHERE ns.node_logical_id = se.node_logical_id)
		        AND NOT EXISTS (SELECT 1 FROM node_source_history h WHERE h.node_logical_id = se.node_logical_id)
		  )
		ORDER BY p.created_at ASC, p.rowid ASC
		LIMIT ?;`

		candRows, err := tx.QueryContext(ctx, candidateSQL,
			subscriptionID, subscriptionID,
			cutoffStr, cutoffStr,
			subscriptionID, subscriptionID,
			subscriptionID, subscriptionID,
			limitVal,
		)
		if err != nil {
			return fmt.Errorf("failed to query candidate payloads for deletion: %w", err)
		}

		var toDeletePayloads []string
		for candRows.Next() {
			var id string
			if err := candRows.Scan(&id); err != nil {
				candRows.Close()
				return fmt.Errorf("failed to scan candidate payload: %w", err)
			}
			if !protectedSet[id] {
				toDeletePayloads = append(toDeletePayloads, id)
			}
		}
		if err := candRows.Err(); err != nil {
			candRows.Close()
			return fmt.Errorf("error reading candidate payloads: %w", err)
		}
		candRows.Close()

		// 3. Delete candidate payloads and their entries in bounded chunks
		const deleteChunkSize = 50
		for i := 0; i < len(toDeletePayloads); i += deleteChunkSize {
			end := i + deleteChunkSize
			if end > len(toDeletePayloads) {
				end = len(toDeletePayloads)
			}
			chunk := toDeletePayloads[i:end]
			placeholders := make([]string, len(chunk))
			args := make([]any, len(chunk))
			for j, id := range chunk {
				placeholders[j] = "?"
				args[j] = id
			}
			inClause := strings.Join(placeholders, ",")

			delEntriesSQL := fmt.Sprintf("DELETE FROM subscription_entries WHERE payload_id IN (%s);", inClause)
			if _, err := tx.ExecContext(ctx, delEntriesSQL, args...); err != nil {
				return fmt.Errorf("failed to delete subscription entries during retention prune: %w", err)
			}

			delPayloadsSQL := fmt.Sprintf("DELETE FROM subscription_payloads WHERE id IN (%s);", inClause)
			if _, err := tx.ExecContext(ctx, delPayloadsSQL, args...); err != nil {
				return fmt.Errorf("failed to delete subscription payloads during retention prune: %w", err)
			}
		}

		res.DeletedPayloads = int64(len(toDeletePayloads))
		res.ProtectedPayloads = int64(len(protectedSet))

		// 4. Fetch retention: keep latest 100 fetches plus referenced fetches
		protectedFetches := make(map[string]bool)

		// 4.1 Newest 100 fetches per subscription
		topFetchesQuery := `
		SELECT id FROM (
			SELECT id, ROW_NUMBER() OVER (
				PARTITION BY subscription_id
				ORDER BY started_at DESC, rowid DESC
			) AS rn
			FROM subscription_fetches
			WHERE (subscription_id = ? OR ? = '')
		) ranked WHERE ranked.rn <= 100;`
		fRows, err := tx.QueryContext(ctx, topFetchesQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query newest 100 fetches: %w", err)
		}
		for fRows.Next() {
			var id string
			if err := fRows.Scan(&id); err != nil {
				fRows.Close()
				return fmt.Errorf("failed to scan top fetch: %w", err)
			}
			protectedFetches[id] = true
		}
		if err := fRows.Err(); err != nil {
			fRows.Close()
			return fmt.Errorf("error reading top fetches: %w", err)
		}
		fRows.Close()

		// 4.2 Fetches referenced by ANY surviving subscription_payloads
		survivingPayloadFetchesQuery := `
		SELECT DISTINCT fetch_id FROM subscription_payloads
		WHERE fetch_id IS NOT NULL AND fetch_id != ''
		  AND (subscription_id = ? OR ? = '');`
		spfRows, err := tx.QueryContext(ctx, survivingPayloadFetchesQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query surviving payload fetches: %w", err)
		}
		for spfRows.Next() {
			var id string
			if err := spfRows.Scan(&id); err != nil {
				spfRows.Close()
				return fmt.Errorf("failed to scan surviving payload fetch: %w", err)
			}
			protectedFetches[id] = true
		}
		if err := spfRows.Err(); err != nil {
			spfRows.Close()
			return fmt.Errorf("error reading surviving payload fetches: %w", err)
		}
		spfRows.Close()

		// 4.3 Fetches referenced by ANY node_sources
		nsFetchesQuery := `
		SELECT DISTINCT last_seen_fetch_id FROM node_sources
		WHERE last_seen_fetch_id IS NOT NULL AND last_seen_fetch_id != ''
		  AND (subscription_id = ? OR ? = '');`
		nsfRows, err := tx.QueryContext(ctx, nsFetchesQuery, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query node sources fetches: %w", err)
		}
		for nsfRows.Next() {
			var id string
			if err := nsfRows.Scan(&id); err != nil {
				nsfRows.Close()
				return fmt.Errorf("failed to scan node sources fetch: %w", err)
			}
			protectedFetches[id] = true
		}
		if err := nsfRows.Err(); err != nil {
			nsfRows.Close()
			return fmt.Errorf("error reading node sources fetches: %w", err)
		}
		nsfRows.Close()

		// 4.4 Candidate fetches to delete using SQL anti-ref + newest100 + LIMIT 100
		candidateFetchesSQL := `
		SELECT f.id
		FROM subscription_fetches f
		WHERE (f.subscription_id = ? OR ? = '')
		  AND f.id NOT IN (
		      SELECT id FROM (
		          SELECT id, ROW_NUMBER() OVER (
		              PARTITION BY subscription_id
		              ORDER BY started_at DESC, rowid DESC
		          ) AS rn
		          FROM subscription_fetches
		          WHERE (subscription_id = ? OR ? = '')
		      ) ranked WHERE ranked.rn <= 100
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM subscription_payloads p WHERE p.fetch_id = f.id
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM node_sources ns WHERE ns.last_seen_fetch_id = f.id
		  )
		ORDER BY f.started_at ASC, f.rowid ASC
		LIMIT 100;`

		cfRows, err := tx.QueryContext(ctx, candidateFetchesSQL, subscriptionID, subscriptionID, subscriptionID, subscriptionID)
		if err != nil {
			return fmt.Errorf("failed to query candidate fetches: %w", err)
		}
		var toDeleteFetches []string
		for cfRows.Next() {
			var id string
			if err := cfRows.Scan(&id); err != nil {
				cfRows.Close()
				return fmt.Errorf("failed to scan candidate fetch: %w", err)
			}
			if !protectedFetches[id] {
				toDeleteFetches = append(toDeleteFetches, id)
			}
		}
		if err := cfRows.Err(); err != nil {
			cfRows.Close()
			return fmt.Errorf("error reading candidate fetches: %w", err)
		}
		cfRows.Close() // Explicitly close rows BEFORE DELETE

		for i := 0; i < len(toDeleteFetches); i += deleteChunkSize {
			end := i + deleteChunkSize
			if end > len(toDeleteFetches) {
				end = len(toDeleteFetches)
			}
			chunk := toDeleteFetches[i:end]
			placeholders := make([]string, len(chunk))
			args := make([]any, len(chunk))
			for j, id := range chunk {
				placeholders[j] = "?"
				args[j] = id
			}
			delFetchesSQL := fmt.Sprintf("DELETE FROM subscription_fetches WHERE id IN (%s);", strings.Join(placeholders, ","))
			if _, err := tx.ExecContext(ctx, delFetchesSQL, args...); err != nil {
				return fmt.Errorf("failed to delete subscription fetches during retention prune: %w", err)
			}
		}

		res.DeletedFetches = int64(len(toDeleteFetches))
		res.ProtectedFetches = int64(len(protectedFetches))

		return nil
	})

	if err != nil {
		return nil, err
	}

	return res, nil
}
