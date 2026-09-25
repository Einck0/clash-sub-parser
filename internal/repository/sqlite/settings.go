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
	       admin_token, updated_at
	FROM settings
	WHERE id = 1;`

	var s domain.Settings
	var updatedStr string

	err := r.db.QueryRowContext(ctx, query).Scan(
		&s.ProbeConcurrencyWindow,
		&s.MaxConcurrentProbes,
		&s.ProbePerNodeTTLSeconds,
		&s.FetchTimeoutSeconds,
		&s.FetchMaxResponseBytes,
		&s.MaxPageSize,
		&s.DefaultPageSize,
		&s.AdminToken,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If not seeded, return domain default
			def := domain.DefaultSettings()
			return &def, nil
		}
		return nil, fmt.Errorf("failed to query settings: %w", err)
	}

	s.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &s, nil
}

func (r *settingsRepository) Update(ctx context.Context, settings *domain.Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}

	const query = `
	INSERT INTO settings (
		id, probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
		fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
		admin_token, updated_at
	) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		probe_concurrency_window = excluded.probe_concurrency_window,
		max_concurrent_probes = excluded.max_concurrent_probes,
		probe_per_node_ttl_seconds = excluded.probe_per_node_ttl_seconds,
		fetch_timeout_seconds = excluded.fetch_timeout_seconds,
		fetch_max_response_bytes = excluded.fetch_max_response_bytes,
		max_page_size = excluded.max_page_size,
		default_page_size = excluded.default_page_size,
		admin_token = excluded.admin_token,
		updated_at = excluded.updated_at;`

	updatedStr := domain.NowUTC().Format(time.RFC3339)
	settings.UpdatedAt = domain.NowUTC()

	_, err := r.db.ExecContext(ctx, query,
		settings.ProbeConcurrencyWindow,
		settings.MaxConcurrentProbes,
		settings.ProbePerNodeTTLSeconds,
		settings.FetchTimeoutSeconds,
		settings.FetchMaxResponseBytes,
		settings.MaxPageSize,
		settings.DefaultPageSize,
		settings.AdminToken,
		updatedStr,
	)
	if err != nil {
		return fmt.Errorf("failed to update settings: %w", err)
	}

	return nil
}

func (r *settingsRepository) UpdateAdminToken(ctx context.Context, tokenVerifier string) error {
	const query = `
	INSERT INTO settings (
		id, probe_concurrency_window, max_concurrent_probes, probe_per_node_ttl_seconds,
		fetch_timeout_seconds, fetch_max_response_bytes, max_page_size, default_page_size,
		admin_token, updated_at
	) VALUES (1, 16, 16, 300, 30, 10485760, 100, 50, ?, ?)
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
