package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
)

type subscriptionRepository struct {
	db *sql.DB
}

// NewSubscriptionRepository constructs a SQLite implementation of domain.SubscriptionRepository.
func NewSubscriptionRepository(db *sql.DB) domain.SubscriptionRepository {
	return &subscriptionRepository{db: db}
}

func (r *subscriptionRepository) GetByID(ctx context.Context, id string) (*domain.Subscription, error) {
	const query = `
	SELECT id, name, source_url_secret_ref, enabled,
	       refresh_interval_seconds, refresh_user_agent_policy, refresh_fetch_proxy_ref,
	       refresh_timeout_seconds, refresh_max_response_bytes,
	       config_json, revision, created_at, updated_at
	FROM subscriptions
	WHERE id = ?;`

	var sub domain.Subscription
	var enabledInt int
	var configJSON sql.NullString
	var createdAtStr, updatedAtStr string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&sub.ID,
		&sub.Name,
		&sub.SourceURLSecretRef,
		&enabledInt,
		&sub.RefreshPolicy.IntervalSeconds,
		&sub.RefreshPolicy.UserAgentPolicy,
		&sub.RefreshPolicy.FetchProxyRef,
		&sub.RefreshPolicy.TimeoutSeconds,
		&sub.RefreshPolicy.MaxResponseBytes,
		&configJSON,
		&sub.Revision,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("subscription_not_found", fmt.Sprintf("subscription %s not found", id))
		}
		return nil, fmt.Errorf("failed to query subscription %s: %w", id, err)
	}

	sub.Enabled = enabledInt == 1
	if configJSON.Valid && strings.TrimSpace(configJSON.String) != "" {
		_ = json.Unmarshal([]byte(configJSON.String), &sub.Config)
	}
	sub.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	sub.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

	if err := r.attachMetadata(ctx, []*domain.Subscription{&sub}); err != nil {
		return nil, err
	}

	return &sub, nil
}

func (r *subscriptionRepository) List(ctx context.Context, filter domain.SubscriptionFilter) ([]domain.Subscription, int, error) {
	whereClauses := make([]string, 0)
	args := make([]interface{}, 0)

	if filter.EnabledOnly {
		whereClauses = append(whereClauses, "enabled = 1")
	}
	if filter.SearchText != "" {
		whereClauses = append(whereClauses, "name LIKE ?")
		args = append(args, "%"+filter.SearchText+"%")
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM subscriptions %s;", whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count subscriptions: %w", err)
	}

	pageSize := filter.Pagination.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	page := filter.Pagination.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	selectQuery := fmt.Sprintf(`
	SELECT id, name, source_url_secret_ref, enabled,
	       refresh_interval_seconds, refresh_user_agent_policy, refresh_fetch_proxy_ref,
	       refresh_timeout_seconds, refresh_max_response_bytes,
	       config_json, revision, created_at, updated_at
	FROM subscriptions
	%s
	ORDER BY created_at DESC
	LIMIT ? OFFSET ?;`, whereSQL)

	queryArgs := append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list subscriptions: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Subscription, 0)
	for rows.Next() {
		var sub domain.Subscription
		var enabledInt int
		var configJSON sql.NullString
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&sub.ID,
			&sub.Name,
			&sub.SourceURLSecretRef,
			&enabledInt,
			&sub.RefreshPolicy.IntervalSeconds,
			&sub.RefreshPolicy.UserAgentPolicy,
			&sub.RefreshPolicy.FetchProxyRef,
			&sub.RefreshPolicy.TimeoutSeconds,
			&sub.RefreshPolicy.MaxResponseBytes,
			&configJSON,
			&sub.Revision,
			&createdAtStr,
			&updatedAtStr,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan subscription: %w", err)
		}

		sub.Enabled = enabledInt == 1
		if configJSON.Valid && strings.TrimSpace(configJSON.String) != "" {
			_ = json.Unmarshal([]byte(configJSON.String), &sub.Config)
		}
		sub.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		sub.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

		items = append(items, sub)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating subscriptions: %w", err)
	}

	if len(items) > 0 {
		subPtrs := make([]*domain.Subscription, len(items))
		for i := range items {
			subPtrs[i] = &items[i]
		}
		if err := r.attachMetadata(ctx, subPtrs); err != nil {
			return nil, 0, err
		}
	}

	return items, total, nil
}

func (r *subscriptionRepository) attachMetadata(ctx context.Context, subs []*domain.Subscription) error {
	if len(subs) == 0 {
		return nil
	}

	if err := r.attachLatestFetches(ctx, subs); err != nil {
		return err
	}
	return r.attachNodeCounts(ctx, subs)
}

func (r *subscriptionRepository) attachNodeCounts(ctx context.Context, subs []*domain.Subscription) error {
	if len(subs) == 0 {
		return nil
	}

	subMap := make(map[string]*domain.Subscription, len(subs))
	placeholders := make([]string, len(subs))
	args := make([]interface{}, len(subs))
	for i, s := range subs {
		s.CountsScope = "enabled_subscriptions"
		s.NodeCount = 0
		s.SourceNodeCount = 0
		subMap[s.ID] = s
		placeholders[i] = "?"
		args[i] = s.ID
	}

	var entriesTableExists int
	_ = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='subscription_entries';").Scan(&entriesTableExists)

	noticeExclusion := ""
	if entriesTableExists > 0 {
		noticeExclusion = `AND ns.node_logical_id NOT IN (
			SELECT se1.node_logical_id FROM subscription_entries se1
			WHERE se1.node_logical_id IS NOT NULL
			  AND COALESCE(se1.user_kind_override, se1.entry_kind) = 'notice'
			  AND NOT EXISTS (
			      SELECT 1 FROM subscription_entries se2
			      WHERE se2.node_logical_id = se1.node_logical_id
			        AND se2.user_kind_override = 'proxy'
			  )
		)`
	}

	query := fmt.Sprintf(`
	WITH latest_good_fetches AS (
		SELECT sf.subscription_id, sf.id AS fetch_id
		FROM subscription_fetches sf
		WHERE sf.outcome IN ('success', 'partial')
		  AND sf.id = (
		      SELECT sf2.id FROM subscription_fetches sf2
		      WHERE sf2.subscription_id = sf.subscription_id
		        AND sf2.outcome IN ('success', 'partial')
		      ORDER BY sf2.started_at DESC, sf2.id DESC
		      LIMIT 1
		  )
	)
	SELECT s.id,
	       COUNT(DISTINCT CASE 
	           WHEN ns.node_logical_id IS NOT NULL %s
	           THEN ns.node_logical_id 
	       END) AS source_node_count
	FROM subscriptions s
	LEFT JOIN latest_good_fetches lgf ON s.id = lgf.subscription_id
	LEFT JOIN node_sources ns ON s.id = ns.subscription_id AND ns.last_seen_fetch_id = lgf.fetch_id
	WHERE s.id IN (%s)
	GROUP BY s.id;`, noticeExclusion, strings.Join(placeholders, ", "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to query subscription node counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var subID string
		var sourceNodeCount int
		if err := rows.Scan(&subID, &sourceNodeCount); err != nil {
			return fmt.Errorf("failed to scan subscription node counts: %w", err)
		}
		sub, ok := subMap[subID]
		if !ok {
			continue
		}
		sub.SourceNodeCount = sourceNodeCount
		if sub.Enabled {
			sub.NodeCount = sourceNodeCount
		} else {
			sub.NodeCount = 0
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating subscription node counts: %w", err)
	}

	return nil
}

func (r *subscriptionRepository) attachLatestFetches(ctx context.Context, subs []*domain.Subscription) error {
	if len(subs) == 0 {
		return nil
	}

	subMap := make(map[string]*domain.Subscription, len(subs))
	placeholders := make([]string, len(subs))
	args := make([]interface{}, len(subs))
	for i, s := range subs {
		subMap[s.ID] = s
		placeholders[i] = "?"
		args[i] = s.ID
	}

	query := fmt.Sprintf(`
	WITH ranked_fetches AS (
		SELECT subscription_id, finished_at, outcome,
		       ROW_NUMBER() OVER (
		           PARTITION BY subscription_id
		           ORDER BY started_at DESC, id DESC
		       ) AS rn
		FROM subscription_fetches
		WHERE subscription_id IN (%s)
		  AND finished_at IS NOT NULL
		  AND trim(finished_at) != ''
	)
	SELECT subscription_id, finished_at, outcome
	FROM ranked_fetches
	WHERE rn = 1;`, strings.Join(placeholders, ", "))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to query latest subscription fetches: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var subID, finishedAtStr, outcomeStr string
		if err := rows.Scan(&subID, &finishedAtStr, &outcomeStr); err != nil {
			return fmt.Errorf("failed to scan latest subscription fetch: %w", err)
		}
		sub, ok := subMap[subID]
		if !ok {
			continue
		}
		trimmedTime := strings.TrimSpace(finishedAtStr)
		if trimmedTime != "" {
			t, err := time.Parse(time.RFC3339, trimmedTime)
			if err != nil {
				t, err = time.Parse(time.RFC3339Nano, trimmedTime)
			}
			if err == nil {
				sub.LastRefreshedAt = &t
			}
		}
		outcomeVal := domain.FetchOutcome(strings.TrimSpace(outcomeStr))
		if outcomeVal != "" {
			sub.LastRefreshOutcome = &outcomeVal
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating latest subscription fetches: %w", err)
	}

	return nil
}

func (r *subscriptionRepository) Create(ctx context.Context, sub *domain.Subscription) error {
	const query = `
	INSERT INTO subscriptions (
		id, name, source_url_secret_ref, enabled,
		refresh_interval_seconds, refresh_user_agent_policy, refresh_fetch_proxy_ref,
		refresh_timeout_seconds, refresh_max_response_bytes,
		config_json, revision, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	enabledInt := 0
	if sub.Enabled {
		enabledInt = 1
	}

	configBytes, _ := json.Marshal(sub.Config)
	configStr := string(configBytes)
	if configStr == "" || configStr == "null" {
		configStr = "{}"
	}

	nowStr := domain.NowUTC().Format(time.RFC3339)
	createdStr := sub.CreatedAt.Format(time.RFC3339)
	if sub.CreatedAt.IsZero() {
		createdStr = nowStr
	}
	updatedStr := sub.UpdatedAt.Format(time.RFC3339)
	if sub.UpdatedAt.IsZero() {
		updatedStr = nowStr
	}

	_, err := r.db.ExecContext(ctx, query,
		sub.ID,
		sub.Name,
		sub.SourceURLSecretRef,
		enabledInt,
		sub.RefreshPolicy.IntervalSeconds,
		sub.RefreshPolicy.UserAgentPolicy,
		sub.RefreshPolicy.FetchProxyRef,
		sub.RefreshPolicy.TimeoutSeconds,
		sub.RefreshPolicy.MaxResponseBytes,
		configStr,
		sub.Revision,
		createdStr,
		updatedStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert subscription: %w", err)
	}
	return nil
}

func (r *subscriptionRepository) Update(ctx context.Context, sub *domain.Subscription) error {
	const query = `
	UPDATE subscriptions SET
		name = ?,
		source_url_secret_ref = ?,
		enabled = ?,
		refresh_interval_seconds = ?,
		refresh_user_agent_policy = ?,
		refresh_fetch_proxy_ref = ?,
		refresh_timeout_seconds = ?,
		refresh_max_response_bytes = ?,
		config_json = ?,
		revision = ?,
		updated_at = ?
	WHERE id = ?;`

	enabledInt := 0
	if sub.Enabled {
		enabledInt = 1
	}

	configBytes, _ := json.Marshal(sub.Config)
	configStr := string(configBytes)
	if configStr == "" || configStr == "null" {
		configStr = "{}"
	}

	updatedStr := domain.NowUTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, query,
		sub.Name,
		sub.SourceURLSecretRef,
		enabledInt,
		sub.RefreshPolicy.IntervalSeconds,
		sub.RefreshPolicy.UserAgentPolicy,
		sub.RefreshPolicy.FetchProxyRef,
		sub.RefreshPolicy.TimeoutSeconds,
		sub.RefreshPolicy.MaxResponseBytes,
		configStr,
		sub.Revision,
		updatedStr,
		sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("subscription_not_found", fmt.Sprintf("subscription %s not found", sub.ID))
	}

	return nil
}

func (r *subscriptionRepository) Delete(ctx context.Context, id string) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM subscriptions WHERE id = ?;", id)
		if err != nil {
			return fmt.Errorf("failed to delete subscription: %w", err)
		}

		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return domain.NewNotFoundError("subscription_not_found", fmt.Sprintf("subscription %s not found", id))
		}

		if _, err := tx.ExecContext(ctx, "DELETE FROM node_sources WHERE subscription_id = ?;", id); err != nil {
			return fmt.Errorf("failed to delete subscription node sources: %w", err)
		}

		const deactivateOrphansSQL = `
		UPDATE nodes
		SET active = 0, updated_at = ?
		WHERE active = 1
		  AND NOT EXISTS (
			  SELECT 1 FROM node_sources WHERE node_sources.node_logical_id = nodes.logical_id
		  );`

		nowStr := domain.NowUTC().Format(time.RFC3339)
		if _, err := tx.ExecContext(ctx, deactivateOrphansSQL, nowStr); err != nil {
			return fmt.Errorf("failed to deactivate orphan nodes after subscription delete: %w", err)
		}

		return nil
	})
}

// SubscriptionFetchRepository implementation

type subscriptionFetchRepository struct {
	db *sql.DB
}

// NewSubscriptionFetchRepository constructs a SQLite implementation of domain.SubscriptionFetchRepository.
func NewSubscriptionFetchRepository(db *sql.DB) domain.SubscriptionFetchRepository {
	return &subscriptionFetchRepository{db: db}
}

func (r *subscriptionFetchRepository) GetByID(ctx context.Context, id string) (*domain.SubscriptionFetch, error) {
	const query = `
	SELECT id, subscription_id, started_at, finished_at, outcome,
	       content_digest, redacted_error, nodes_parsed, nodes_valid
	FROM subscription_fetches
	WHERE id = ?;`

	var fetch domain.SubscriptionFetch
	var startedAtStr string
	var finishedAtStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&fetch.ID,
		&fetch.SubscriptionID,
		&startedAtStr,
		&finishedAtStr,
		&fetch.Outcome,
		&fetch.ContentDigest,
		&fetch.RedactedError,
		&fetch.NodesParsed,
		&fetch.NodesValid,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("fetch_not_found", fmt.Sprintf("subscription fetch %s not found", id))
		}
		return nil, fmt.Errorf("failed to query subscription fetch %s: %w", id, err)
	}

	fetch.StartedAt, _ = time.Parse(time.RFC3339, startedAtStr)
	if finishedAtStr.Valid && finishedAtStr.String != "" {
		finTime, err := time.Parse(time.RFC3339, finishedAtStr.String)
		if err == nil {
			fetch.FinishedAt = &finTime
		}
	}

	return &fetch, nil
}

func (r *subscriptionFetchRepository) ListBySubscription(ctx context.Context, subID string, limit int) ([]domain.SubscriptionFetch, error) {
	if limit <= 0 {
		limit = 20
	}

	const query = `
	SELECT id, subscription_id, started_at, finished_at, outcome,
	       content_digest, redacted_error, nodes_parsed, nodes_valid
	FROM subscription_fetches
	WHERE subscription_id = ?
	ORDER BY started_at DESC
	LIMIT ?;`

	rows, err := r.db.QueryContext(ctx, query, subID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list fetches for subscription %s: %w", subID, err)
	}
	defer rows.Close()

	items := make([]domain.SubscriptionFetch, 0)
	for rows.Next() {
		var fetch domain.SubscriptionFetch
		var startedAtStr string
		var finishedAtStr sql.NullString

		err := rows.Scan(
			&fetch.ID,
			&fetch.SubscriptionID,
			&startedAtStr,
			&finishedAtStr,
			&fetch.Outcome,
			&fetch.ContentDigest,
			&fetch.RedactedError,
			&fetch.NodesParsed,
			&fetch.NodesValid,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription fetch: %w", err)
		}

		fetch.StartedAt, _ = time.Parse(time.RFC3339, startedAtStr)
		if finishedAtStr.Valid && finishedAtStr.String != "" {
			finTime, err := time.Parse(time.RFC3339, finishedAtStr.String)
			if err == nil {
				fetch.FinishedAt = &finTime
			}
		}

		items = append(items, fetch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating subscription fetches: %w", err)
	}

	return items, nil
}

func (r *subscriptionFetchRepository) Create(ctx context.Context, fetch *domain.SubscriptionFetch) error {
	const query = `
	INSERT INTO subscription_fetches (
		id, subscription_id, started_at, finished_at, outcome,
		content_digest, redacted_error, nodes_parsed, nodes_valid
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`

	startedStr := fetch.StartedAt.Format(time.RFC3339)
	if fetch.StartedAt.IsZero() {
		startedStr = domain.NowUTC().Format(time.RFC3339)
	}

	var finishedStr sql.NullString
	if fetch.FinishedAt != nil && !fetch.FinishedAt.IsZero() {
		finishedStr = sql.NullString{String: fetch.FinishedAt.Format(time.RFC3339), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, query,
		fetch.ID,
		fetch.SubscriptionID,
		startedStr,
		finishedStr,
		string(fetch.Outcome),
		fetch.ContentDigest,
		fetch.RedactedError,
		fetch.NodesParsed,
		fetch.NodesValid,
	)
	if err != nil {
		return fmt.Errorf("failed to insert subscription fetch: %w", err)
	}

	return nil
}
