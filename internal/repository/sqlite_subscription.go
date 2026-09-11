package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"clash-sub-parser/internal/domain"
)

type sqliteSubscriptionRepository struct {
	db *SQLiteDB
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSubscription(s rowScanner) (*domain.Subscription, error) {
	var (
		sub                                              domain.Subscription
		updateInterval                                   sql.NullInt64
		isPrimaryInt, enabledInt                         int64
		nodePrefix, filterRegexRaw                       sql.NullString
		filterMinSpeed                                   sql.NullFloat64
		filterMediaUnlockRaw                             sql.NullString
		includeNodeNamesRaw, excludeNodeNamesRaw         sql.NullString
		nodeRenamesRaw                                   sql.NullString
		sourceNodesRaw, manualNodesRaw, rawNodesRaw      sql.NullString
		lastFetchedAtStr, lastFetchError                 sql.NullString
		fetchFailedCount                                 sql.NullInt64
		fetchCommentsRaw                                 sql.NullString
		subscriptionUserinfo                             sql.NullString
		profileUpdateInterval, profileWebPageURL         sql.NullString
		proxyChain, nodeProxyChainsRaw                   sql.NullString
	)

	err := s.Scan(
		&sub.ID,
		&sub.Name,
		&sub.URL,
		&updateInterval,
		&isPrimaryInt,
		&enabledInt,
		&nodePrefix,
		&filterRegexRaw,
		&filterMinSpeed,
		&filterMediaUnlockRaw,
		&includeNodeNamesRaw,
		&excludeNodeNamesRaw,
		&nodeRenamesRaw,
		&sourceNodesRaw,
		&manualNodesRaw,
		&rawNodesRaw,
		&lastFetchedAtStr,
		&lastFetchError,
		&fetchFailedCount,
		&fetchCommentsRaw,
		&subscriptionUserinfo,
		&profileUpdateInterval,
		&profileWebPageURL,
		&proxyChain,
		&nodeProxyChainsRaw,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan subscription: %w", err)
	}

	if updateInterval.Valid {
		sub.UpdateInterval = int(updateInterval.Int64)
	}
	sub.IsPrimary = intToBool(isPrimaryInt)
	sub.Enabled = intToBool(enabledInt)
	sub.NodePrefix = stringFromNull(nodePrefix)
	sub.FilterMinSpeedMbps = float64PtrFromNull(filterMinSpeed)
	sub.LastFetchedAt = timePtrFromNull(lastFetchedAtStr)
	sub.LastFetchError = stringFromNull(lastFetchError)
	if fetchFailedCount.Valid {
		sub.FetchFailedCount = int(fetchFailedCount.Int64)
	}
	sub.SubscriptionUserinfo = stringFromNull(subscriptionUserinfo)
	sub.ProfileUpdateInterval = stringFromNull(profileUpdateInterval)
	sub.ProfileWebPageURL = stringFromNull(profileWebPageURL)
	sub.ProxyChain = stringFromNull(proxyChain)

	// JSON unmarshals
	_ = unmarshalJSONSafe(stringFromNull(filterRegexRaw), &sub.FilterRegex)
	_ = unmarshalJSONSafe(stringFromNull(filterMediaUnlockRaw), &sub.FilterMediaUnlock)
	_ = unmarshalJSONSafe(stringFromNull(includeNodeNamesRaw), &sub.IncludeNodeNames)
	_ = unmarshalJSONSafe(stringFromNull(excludeNodeNamesRaw), &sub.ExcludeNodeNames)
	_ = unmarshalJSONSafe(stringFromNull(nodeRenamesRaw), &sub.NodeRenames)
	_ = unmarshalJSONSafe(stringFromNull(sourceNodesRaw), &sub.SourceNodes)
	_ = unmarshalJSONSafe(stringFromNull(manualNodesRaw), &sub.ManualNodes)
	_ = unmarshalJSONSafe(stringFromNull(rawNodesRaw), &sub.RawNodes)
	_ = unmarshalJSONSafe(stringFromNull(fetchCommentsRaw), &sub.FetchComments)
	_ = unmarshalJSONSafe(stringFromNull(nodeProxyChainsRaw), &sub.NodeProxyChains)

	return &sub, nil
}

const subscriptionSelectFields = `
	id, name, url, update_interval, is_primary, enabled, node_prefix,
	filter_regex, filter_min_speed_mbps, filter_media_unlock,
	include_node_names, exclude_node_names, node_renames,
	source_nodes, manual_nodes, raw_nodes, last_fetched_at, last_fetch_error,
	fetch_failed_count, fetch_comments, subscription_userinfo,
	profile_update_interval, profile_web_page_url, proxy_chain, node_proxy_chains
`

func (r *sqliteSubscriptionRepository) GetByID(ctx context.Context, id int64) (*domain.Subscription, error) {
	query := fmt.Sprintf("SELECT %s FROM subscriptions WHERE id = ?", subscriptionSelectFields)
	row := r.db.QueryRowContext(ctx, query, id)
	return scanSubscription(row)
}

func (r *sqliteSubscriptionRepository) GetByName(ctx context.Context, name string) (*domain.Subscription, error) {
	query := fmt.Sprintf("SELECT %s FROM subscriptions WHERE name = ?", subscriptionSelectFields)
	row := r.db.QueryRowContext(ctx, query, name)
	return scanSubscription(row)
}

func (r *sqliteSubscriptionRepository) List(ctx context.Context, enabledOnly bool) ([]*domain.Subscription, error) {
	query := fmt.Sprintf("SELECT %s FROM subscriptions", subscriptionSelectFields)
	var args []any
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY id"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()

	var result []*domain.Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}
	return result, nil
}

func (r *sqliteSubscriptionRepository) Create(ctx context.Context, sub *domain.Subscription) error {
	if sub == nil {
		return ErrNilEntity
	}

	if err := sub.Validate(); err != nil {
		return fmt.Errorf("validate subscription: %w", err)
	}

	query := `
		INSERT INTO subscriptions (
			name, url, update_interval, is_primary, enabled, node_prefix,
			filter_regex, filter_min_speed_mbps, filter_media_unlock,
			include_node_names, exclude_node_names, node_renames,
			source_nodes, manual_nodes, raw_nodes, last_fetched_at, last_fetch_error,
			fetch_failed_count, fetch_comments, subscription_userinfo,
			profile_update_interval, profile_web_page_url, proxy_chain, node_proxy_chains
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	res, err := r.db.ExecContext(ctx, query,
		sub.Name,
		sub.URL,
		sub.UpdateInterval,
		boolToInt(sub.IsPrimary),
		boolToInt(sub.Enabled),
		nullStringVal(sub.NodePrefix),
		marshalJSONSafe(sub.FilterRegex, "[]"),
		nullFloat64FromPtr(sub.FilterMinSpeedMbps),
		marshalJSONSafe(sub.FilterMediaUnlock, "[]"),
		marshalJSONSafe(sub.IncludeNodeNames, "[]"),
		marshalJSONSafe(sub.ExcludeNodeNames, "[]"),
		marshalJSONSafe(sub.NodeRenames, "{}"),
		marshalJSONSafe(sub.SourceNodes, "[]"),
		marshalJSONSafe(sub.ManualNodes, "[]"),
		marshalJSONSafe(sub.RawNodes, "[]"),
		nullTimeFromPtr(sub.LastFetchedAt),
		nullStringVal(sub.LastFetchError),
		sub.FetchFailedCount,
		marshalJSONSafe(sub.FetchComments, "[]"),
		nullStringVal(sub.SubscriptionUserinfo),
		nullStringVal(sub.ProfileUpdateInterval),
		nullStringVal(sub.ProfileWebPageURL),
		nullStringVal(sub.ProxyChain),
		marshalJSONSafe(sub.NodeProxyChains, "{}"),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicate
		}
		return fmt.Errorf("create subscription: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		sub.ID = id
	}
	return nil
}

func (r *sqliteSubscriptionRepository) Update(ctx context.Context, sub *domain.Subscription) error {
	if sub == nil {
		return ErrNilEntity
	}

	if err := sub.Validate(); err != nil {
		return fmt.Errorf("validate subscription: %w", err)
	}

	query := `
		UPDATE subscriptions SET
			name = ?,
			url = ?,
			update_interval = ?,
			is_primary = ?,
			enabled = ?,
			node_prefix = ?,
			filter_regex = ?,
			filter_min_speed_mbps = ?,
			filter_media_unlock = ?,
			include_node_names = ?,
			exclude_node_names = ?,
			node_renames = ?,
			source_nodes = ?,
			manual_nodes = ?,
			raw_nodes = ?,
			last_fetched_at = ?,
			last_fetch_error = ?,
			fetch_failed_count = ?,
			fetch_comments = ?,
			subscription_userinfo = ?,
			profile_update_interval = ?,
			profile_web_page_url = ?,
			proxy_chain = ?,
			node_proxy_chains = ?
		WHERE id = ?
	`

	res, err := r.db.ExecContext(ctx, query,
		sub.Name,
		sub.URL,
		sub.UpdateInterval,
		boolToInt(sub.IsPrimary),
		boolToInt(sub.Enabled),
		nullStringVal(sub.NodePrefix),
		marshalJSONSafe(sub.FilterRegex, "[]"),
		nullFloat64FromPtr(sub.FilterMinSpeedMbps),
		marshalJSONSafe(sub.FilterMediaUnlock, "[]"),
		marshalJSONSafe(sub.IncludeNodeNames, "[]"),
		marshalJSONSafe(sub.ExcludeNodeNames, "[]"),
		marshalJSONSafe(sub.NodeRenames, "{}"),
		marshalJSONSafe(sub.SourceNodes, "[]"),
		marshalJSONSafe(sub.ManualNodes, "[]"),
		marshalJSONSafe(sub.RawNodes, "[]"),
		nullTimeFromPtr(sub.LastFetchedAt),
		nullStringVal(sub.LastFetchError),
		sub.FetchFailedCount,
		marshalJSONSafe(sub.FetchComments, "[]"),
		nullStringVal(sub.SubscriptionUserinfo),
		nullStringVal(sub.ProfileUpdateInterval),
		nullStringVal(sub.ProfileWebPageURL),
		nullStringVal(sub.ProxyChain),
		marshalJSONSafe(sub.NodeProxyChains, "{}"),
		sub.ID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicate
		}
		return fmt.Errorf("update subscription: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, "DELETE FROM subscriptions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *sqliteSubscriptionRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM subscriptions").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count subscriptions: %w", err)
	}
	return count, nil
}
