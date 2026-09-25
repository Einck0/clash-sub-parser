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

	return items, total, nil
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
	res, err := r.db.ExecContext(ctx, "DELETE FROM subscriptions WHERE id = ?;", id)
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

	return nil
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
