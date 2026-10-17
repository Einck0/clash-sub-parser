package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

type settingsRepository struct {
	db *sql.DB
}

// NewSettingsRepository constructs a SQLite implementation of domain.SettingsRepository.
func NewSettingsRepository(db *sql.DB) domain.SettingsRepository {
	return &settingsRepository{db: db}
}

func (r *settingsRepository) Get(ctx context.Context) (*domain.Settings, error) {
	const query = `
	SELECT probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
	       fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
	       admin_token, admin_auth_enabled, export_auth_enabled, updated_at,
        subscription_fetch_mode, subscription_fetch_proxy_url, COALESCE(public_base_url, ''),
        COALESCE(probe_timeout_seconds, 5)
	FROM settings
	WHERE id = 1;`

	var s domain.Settings
	var updatedStr string
	var adminAuthInt, exportAuthInt int

	err := r.db.QueryRowContext(ctx, query).Scan(
		&s.ProbeConcurrencyWindow,
		&s.MaxConcurrentProbes,
		&s.ProbePerNodeTTLSeconds,
		&s.FetchTimeoutSeconds,
		&s.FetchMaxResponseBytes,
		&s.MaxPageSize,
		&s.DefaultPageSize,
		&s.AdminToken,
		&adminAuthInt,
		&exportAuthInt,
		&updatedStr,
		&s.SubscriptionFetchMode,
		&s.SubscriptionFetchProxyURL,
		&s.PublicBaseURL,
		&s.ProbeTimeoutSeconds,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If not seeded, return domain default
			def := domain.DefaultSettings()
			return &def, nil
		}
		return nil, fmt.Errorf("failed to query settings: %w", err)
	}

	s.AdminAuthEnabled = adminAuthInt != 0
	s.ExportAuthEnabled = exportAuthInt != 0
	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &s, nil
}

func (r *settingsRepository) Update(ctx context.Context, settings *domain.Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	if settings.SubscriptionFetchProxyURL != "" {
		normalized, err := fetch.NormalizeProxyURL(settings.SubscriptionFetchProxyURL)
		if err != nil {
			return err
		}
		settings.SubscriptionFetchProxyURL = normalized
	}

	const query = `
	INSERT INTO settings (
		id, probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
		fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
		admin_token, admin_auth_enabled, export_auth_enabled, updated_at,
        subscription_fetch_mode, subscription_fetch_proxy_url, public_base_url, probe_timeout_seconds
	) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		probe_concurrency_window = excluded.probe_concurrency_window,
		max_concurrent_probes = excluded.max_concurrent_probes,
		probe_per_node_ttl_seconds = excluded.probe_per_node_ttl_seconds,
		fetch_timeout_seconds = excluded.fetch_timeout_seconds,
		fetch_max_response_bytes = excluded.fetch_max_response_bytes,
		max_page_size = excluded.max_page_size,
		default_page_size = excluded.default_page_size,
		admin_token = excluded.admin_token,
		admin_auth_enabled = excluded.admin_auth_enabled,
		export_auth_enabled = excluded.export_auth_enabled,
        subscription_fetch_mode = excluded.subscription_fetch_mode,
        subscription_fetch_proxy_url = excluded.subscription_fetch_proxy_url,
        public_base_url = excluded.public_base_url,
        probe_timeout_seconds = excluded.probe_timeout_seconds,
		updated_at = excluded.updated_at;`

	updatedStr := domain.NowUTC().Format(time.RFC3339)
	settings.UpdatedAt = domain.NowUTC()

	adminAuthInt := 0
	if settings.AdminAuthEnabled {
		adminAuthInt = 1
	}
	exportAuthInt := 0
	if settings.ExportAuthEnabled {
		exportAuthInt = 1
	}

	_, err := r.db.ExecContext(ctx, query,
		settings.ProbeConcurrencyWindow,
		settings.MaxConcurrentProbes,
		settings.ProbePerNodeTTLSeconds,
		settings.FetchTimeoutSeconds,
		settings.FetchMaxResponseBytes,
		settings.MaxPageSize,
		settings.DefaultPageSize,
		settings.AdminToken,
		adminAuthInt,
		exportAuthInt,
		updatedStr,
		settings.SubscriptionFetchMode,
		settings.SubscriptionFetchProxyURL,
		settings.PublicBaseURL,
		settings.ProbeTimeoutSeconds,
	)
	if err != nil {
		return fmt.Errorf("failed to update settings: %w", err)
	}

	return nil
}

func (r *settingsRepository) UpdatePublicBaseURL(ctx context.Context, publicBaseURL string) error {
	trimmed := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if trimmed != "" {
		u, err := url.ParseRequestURI(trimmed)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return domain.NewValidationError("invalid_public_base_url", "public_base_url must be a valid absolute HTTP or HTTPS URL")
		}
		if u.User != nil {
			return domain.NewValidationError("invalid_public_base_url", "public_base_url must not contain userinfo")
		}
		if u.RawQuery != "" || strings.Contains(trimmed, "?") {
			return domain.NewValidationError("invalid_public_base_url", "public_base_url must not contain query parameters")
		}
		if u.Fragment != "" || strings.Contains(trimmed, "#") {
			return domain.NewValidationError("invalid_public_base_url", "public_base_url must not contain a fragment")
		}
	}

	const query = `UPDATE settings SET public_base_url = ?, updated_at = ? WHERE id = 1;`
	res, err := r.db.ExecContext(ctx, query, trimmed, domain.NowUTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to update public_base_url: %w", err)
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		def := domain.DefaultSettings()
		def.PublicBaseURL = trimmed
		return r.Update(ctx, &def)
	}
	return nil
}

func (r *settingsRepository) UpdateProbeTimeout(ctx context.Context, seconds int) error {
	if seconds < 1 || seconds > 30 {
		return domain.NewValidationError("invalid_probe_timeout_seconds",
			fmt.Sprintf("probe_timeout_seconds must be between 1 and 30, got %d", seconds))
	}
	const query = `
	INSERT INTO settings (
		id, probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
		fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
		admin_token, admin_auth_enabled, export_auth_enabled, updated_at,
        subscription_fetch_mode, subscription_fetch_proxy_url, public_base_url, probe_timeout_seconds
	) VALUES (1, 16, 16, 300, 30, 10485760, 100, 50, '', 1, 1, ?, 'environment', '', '', ?)
	ON CONFLICT(id) DO UPDATE SET
		probe_timeout_seconds = excluded.probe_timeout_seconds,
		updated_at = excluded.updated_at;`

	updatedStr := domain.NowUTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, query, updatedStr, seconds)
	if err != nil {
		return fmt.Errorf("failed to update probe timeout: %w", err)
	}
	return nil
}

func (r *settingsRepository) UpdateSubscriptionFetch(ctx context.Context, mode string, proxyURL *string) error {
	// Validate a replacement before writing. Omission retains the URL in the same SQL statement.
	candidate := ""
	var replacement any
	if proxyURL != nil {
		candidate = *proxyURL
		if candidate != "" {
			normalized, err := fetch.NormalizeProxyURL(candidate)
			if err != nil {
				return err
			}
			candidate = normalized
		}
		replacement = candidate
	}
	validationURL := candidate
	if proxyURL == nil {
		validationURL = "retained"
	}
	if err := domain.ValidateSubscriptionFetchSettings(mode, validationURL); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE settings SET
  subscription_fetch_mode = ?, subscription_fetch_proxy_url = COALESCE(?, subscription_fetch_proxy_url), updated_at = ?
  WHERE id = 1 AND (? != 'proxy' OR length(COALESCE(?, subscription_fetch_proxy_url)) > 0)`,
		mode, replacement, domain.NowUTC().Format(time.RFC3339), mode, replacement)
	if err != nil {
		return fmt.Errorf("failed to update subscription fetch settings: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return domain.NewValidationError("invalid_subscription_fetch", "proxy mode requires a configured proxy URL")
	}
	return nil
}

func (r *settingsRepository) UpdateAdminToken(ctx context.Context, tokenVerifier string) error {
	const query = `
	INSERT INTO settings (
		id, probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
		fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
		admin_token, admin_auth_enabled, export_auth_enabled, updated_at
	) VALUES (1, 16, 16, 300, 30, 10485760, 100, 50, ?, 1, 1, ?)
	ON CONFLICT(id) DO UPDATE SET
		admin_token = excluded.admin_token,
		updated_at = excluded.updated_at;`

	updatedStr := domain.NowUTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, query, tokenVerifier, updatedStr)
	if err != nil {
		return fmt.Errorf("failed to update admin token verifier: %w", err)
	}
	return nil
}

func (r *settingsRepository) UpdateAuthSettings(ctx context.Context, adminAuthEnabled, exportAuthEnabled bool, tokenVerifier *string) error {
	adminAuthInt := 0
	if adminAuthEnabled {
		adminAuthInt = 1
	}
	exportAuthInt := 0
	if exportAuthEnabled {
		exportAuthInt = 1
	}
	updatedStr := domain.NowUTC().Format(time.RFC3339)

	var query string
	var args []any
	if tokenVerifier != nil {
		query = `
		INSERT INTO settings (
			id, probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
			fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
			admin_token, admin_auth_enabled, export_auth_enabled, updated_at
		) VALUES (1, 16, 16, 300, 30, 10485760, 100, 50, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			admin_token = excluded.admin_token,
			admin_auth_enabled = excluded.admin_auth_enabled,
			export_auth_enabled = excluded.export_auth_enabled,
			updated_at = excluded.updated_at;`
		args = []any{*tokenVerifier, adminAuthInt, exportAuthInt, updatedStr}
	} else {
		query = `
		INSERT INTO settings (
			id, probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
			fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
			admin_token, admin_auth_enabled, export_auth_enabled, updated_at
		) VALUES (1, 16, 16, 300, 30, 10485760, 100, 50, '', ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			admin_auth_enabled = excluded.admin_auth_enabled,
			export_auth_enabled = excluded.export_auth_enabled,
			updated_at = excluded.updated_at;`
		args = []any{adminAuthInt, exportAuthInt, updatedStr}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update auth settings: %w", err)
	}
	return nil
}

// AuditRepository implementation

type auditRepository struct {
	db *sql.DB
}

// NewAuditRepository constructs a SQLite implementation of domain.AuditRepository.
func NewAuditRepository(db *sql.DB) domain.AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) Record(ctx context.Context, event *domain.AuditEvent) error {
	const query = `
	INSERT INTO audit_events (id, actor_kind, request_id, action, result, redacted_summary, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?);`

	createdStr := event.CreatedAt.Format(time.RFC3339)
	if event.CreatedAt.IsZero() {
		createdStr = domain.NowUTC().Format(time.RFC3339)
	}

	_, err := r.db.ExecContext(ctx, query,
		event.ID,
		string(event.ActorKind),
		event.RequestID,
		event.Action,
		string(event.Result),
		event.RedactedSummary,
		createdStr,
	)
	if err != nil {
		return fmt.Errorf("failed to record audit event: %w", err)
	}
	return nil
}

func (r *auditRepository) List(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEvent, int, error) {
	whereClauses := make([]string, 0)
	args := make([]interface{}, 0)

	if filter.ActorKind != nil && *filter.ActorKind != "" {
		whereClauses = append(whereClauses, "actor_kind = ?")
		args = append(args, string(*filter.ActorKind))
	}
	if filter.Action != "" {
		whereClauses = append(whereClauses, "action = ?")
		args = append(args, filter.Action)
	}
	if filter.From != nil && !filter.From.IsZero() {
		whereClauses = append(whereClauses, "created_at >= ?")
		args = append(args, filter.From.Format(time.RFC3339))
	}
	if filter.To != nil && !filter.To.IsZero() {
		whereClauses = append(whereClauses, "created_at <= ?")
		args = append(args, filter.To.Format(time.RFC3339))
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_events %s;", whereSQL)
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit events: %w", err)
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
	SELECT id, actor_kind, request_id, action, result, redacted_summary, created_at
	FROM audit_events
	%s
	ORDER BY created_at DESC
	LIMIT ? OFFSET ?;`, whereSQL)

	queryArgs := append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, selectQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query audit events: %w", err)
	}
	defer rows.Close()

	items := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var e domain.AuditEvent
		var actorStr, resultStr, createdStr string

		err := rows.Scan(
			&e.ID,
			&actorStr,
			&e.RequestID,
			&e.Action,
			&resultStr,
			&e.RedactedSummary,
			&createdStr,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit event: %w", err)
		}

		e.ActorKind = domain.ActorKind(actorStr)
		e.Result = domain.AuditResult(resultStr)
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)

		items = append(items, e)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating audit events: %w", err)
	}

	return items, total, nil
}
