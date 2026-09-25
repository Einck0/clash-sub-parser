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

type ipRiskProviderSettingsRepository struct{ db *sql.DB }

// NewIPRiskProviderSettingsRepository constructs provider settings persistence.
func NewIPRiskProviderSettingsRepository(db *sql.DB) domain.IPRiskProviderSettingsRepository {
	return &ipRiskProviderSettingsRepository{db: db}
}

func (r *ipRiskProviderSettingsRepository) Get(ctx context.Context, provider, schemaVersion string) (*domain.IPRiskProviderSettings, error) {
	const query = `
	SELECT provider, schema_version, enabled, secret_reference, max_concurrency,
	requests_per_minute, daily_request_budget, per_request_timeout_ms,
	       max_response_bytes, created_at, updated_at
	FROM ip_risk_provider_settings
	WHERE provider = ? AND schema_version = ?;`
	var settings domain.IPRiskProviderSettings
	var enabled int
	var timeoutMilliseconds int
	var createdAt, updatedAt string
	if err := r.db.QueryRowContext(ctx, query, provider, schemaVersion).Scan(
		&settings.Provider, &settings.SchemaVersion, &enabled, &settings.SecretReference,
		&settings.MaxConcurrency, &settings.RequestsPerMinute, &settings.DailyRequestBudget,
		&timeoutMilliseconds, &settings.MaxResponseBytes, &createdAt, &updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("ip_risk_provider_settings_not_found", "IP risk provider settings not found")
		}
		return nil, fmt.Errorf("failed to query IP risk provider settings: %w", err)
	}
	settings.Enabled = enabled != 0
	settings.PerRequestTimeout = time.Duration(timeoutMilliseconds) * time.Millisecond
	var parseErr error
	settings.CreatedAt, parseErr = parseIPRiskTime(createdAt, "created_at")
	if parseErr != nil {
		return nil, parseErr
	}
	settings.UpdatedAt, parseErr = parseIPRiskTime(updatedAt, "updated_at")
	if parseErr != nil {
		return nil, parseErr
	}
	return &settings, nil
}

func (r *ipRiskProviderSettingsRepository) List(ctx context.Context, enabledOnly bool) ([]domain.IPRiskProviderSettings, error) {
	query := `
	SELECT provider, schema_version, enabled, secret_reference, max_concurrency,
	requests_per_minute, daily_request_budget, per_request_timeout_ms,
	       max_response_bytes, created_at, updated_at
	FROM ip_risk_provider_settings`
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY provider ASC, schema_version ASC;"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list IP risk provider settings: %w", err)
	}
	defer rows.Close()
	items := make([]domain.IPRiskProviderSettings, 0)
	for rows.Next() {
		var settings domain.IPRiskProviderSettings
		var enabled, timeoutMilliseconds int
		var createdAt, updatedAt string
		if err := rows.Scan(&settings.Provider, &settings.SchemaVersion, &enabled, &settings.SecretReference,
			&settings.MaxConcurrency, &settings.RequestsPerMinute, &settings.DailyRequestBudget,
			&timeoutMilliseconds, &settings.MaxResponseBytes, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan IP risk provider settings: %w", err)
		}
		settings.Enabled = enabled != 0
		settings.PerRequestTimeout = time.Duration(timeoutMilliseconds) * time.Millisecond
		var parseErr error
		settings.CreatedAt, parseErr = parseIPRiskTime(createdAt, "created_at")
		if parseErr != nil {
			return nil, parseErr
		}
		settings.UpdatedAt, parseErr = parseIPRiskTime(updatedAt, "updated_at")
		if parseErr != nil {
			return nil, parseErr
		}
		items = append(items, settings)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating IP risk provider settings: %w", err)
	}
	return items, nil
}

func (r *ipRiskProviderSettingsRepository) Upsert(ctx context.Context, settings *domain.IPRiskProviderSettings) error {
	if settings == nil {
		return domain.NewValidationError("missing_ip_risk_provider_settings", "provider settings are required")
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	now := domain.NowUTC()
	if settings.CreatedAt.IsZero() {
		settings.CreatedAt = now
	}
	settings.UpdatedAt = now
	const query = `
	INSERT INTO ip_risk_provider_settings (
		provider, schema_version, enabled, secret_reference, max_concurrency,
		requests_per_minute, daily_request_budget, per_request_timeout_ms,
		max_response_bytes, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(provider, schema_version) DO UPDATE SET
		enabled = excluded.enabled, secret_reference = excluded.secret_reference,
		max_concurrency = excluded.max_concurrency, requests_per_minute = excluded.requests_per_minute,
		daily_request_budget = excluded.daily_request_budget,
		per_request_timeout_ms = excluded.per_request_timeout_ms,
		max_response_bytes = excluded.max_response_bytes, updated_at = excluded.updated_at;`
	_, err := r.db.ExecContext(ctx, query, settings.Provider, settings.SchemaVersion, boolInt(settings.Enabled),
		settings.SecretReference, settings.MaxConcurrency, settings.RequestsPerMinute, settings.DailyRequestBudget,
		settings.PerRequestTimeout.Milliseconds(), settings.MaxResponseBytes,
		settings.CreatedAt.Format(time.RFC3339Nano), settings.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("failed to upsert IP risk provider settings: %w", err)
	}
	return nil
}

type ipRiskObservationRepository struct{ db *sql.DB }

// NewIPRiskObservationRepository constructs append-only IP risk evidence persistence.
func NewIPRiskObservationRepository(db *sql.DB) domain.IPRiskObservationRepository {
	return &ipRiskObservationRepository{db: db}
}

func parseIPRiskTime(value, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid %s: %w", field, err)
	}
	return parsed, nil
}

const ipRiskObservationColumns = `id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
	observed_at, expires_at, status, score, confidence, network_class, anonymizer_traits, evidence_digest, redacted_summary`

func scanIPRiskObservation(scanner interface{ Scan(...any) error }) (*domain.IPRiskObservation, error) {
	var observation domain.IPRiskObservation
	var observedAt, expiresAt string
	var status, networkClass string
	var score, confidence sql.NullInt64
	var traitsJSON string
	if err := scanner.Scan(&observation.ID, &observation.NodeLogicalID, &observation.ExitIdentityDigest,
		&observation.Provider, &observation.ProviderSchemaVersion, &observedAt, &expiresAt, &status,
		&score, &confidence, &networkClass, &traitsJSON, &observation.EvidenceDigest, &observation.RedactedSummary); err != nil {
		return nil, err
	}
	var err error
	if observation.ObservedAt, err = parseIPRiskTime(observedAt, "observed_at"); err != nil {
		return nil, err
	}
	if observation.ExpiresAt, err = parseIPRiskTime(expiresAt, "expires_at"); err != nil {
		return nil, err
	}
	observation.Status = domain.IPRiskStatus(status)
	observation.NetworkClass = domain.NetworkClass(networkClass)
	if score.Valid {
		value := int(score.Int64)
		observation.Score = &value
	}
	if confidence.Valid {
		value := int(confidence.Int64)
		observation.Confidence = &value
	}
	if err := json.Unmarshal([]byte(traitsJSON), &observation.AnonymizerTraits); err != nil {
		return nil, fmt.Errorf("failed to decode IP risk traits: %w", err)
	}
	return &observation, nil
}

func (r *ipRiskObservationRepository) GetByID(ctx context.Context, id string) (*domain.IPRiskObservation, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+ipRiskObservationColumns+" FROM ip_risk_observations WHERE id = ?;", id)
	observation, err := scanIPRiskObservation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.NewNotFoundError("ip_risk_observation_not_found", "IP risk observation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query IP risk observation: %w", err)
	}
	return observation, nil
}

func (r *ipRiskObservationRepository) List(ctx context.Context, filter domain.IPRiskObservationFilter) ([]domain.IPRiskObservation, int, error) {
	filter = filter.Normalize()
	where := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if filter.NodeLogicalID != "" {
		where = append(where, "node_logical_id = ?")
		args = append(args, filter.NodeLogicalID)
	}
	if filter.Provider != "" {
		where = append(where, "provider = ?")
		args = append(args, filter.Provider)
	}
	if filter.SchemaVersion != "" {
		where = append(where, "provider_schema_version = ?")
		args = append(args, filter.SchemaVersion)
	}
	if filter.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(filter.Status))
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ip_risk_observations"+whereSQL+";", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count IP risk observations: %w", err)
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := r.db.QueryContext(ctx, "SELECT "+ipRiskObservationColumns+" FROM ip_risk_observations"+whereSQL+" ORDER BY observed_at DESC, id DESC LIMIT ? OFFSET ?;", args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list IP risk observations: %w", err)
	}
	defer rows.Close()
	items := make([]domain.IPRiskObservation, 0)
	for rows.Next() {
		observation, err := scanIPRiskObservation(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan IP risk observation: %w", err)
		}
		items = append(items, *observation)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating IP risk observations: %w", err)
	}
	return items, total, nil
}

func (r *ipRiskObservationRepository) Create(ctx context.Context, observation *domain.IPRiskObservation) error {
	if observation == nil {
		return domain.NewValidationError("missing_ip_risk_observation", "IP risk observation is required")
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	traits, err := json.Marshal(observation.AnonymizerTraits)
	if err != nil {
		return fmt.Errorf("failed to encode IP risk traits: %w", err)
	}
	const query = `
	INSERT INTO ip_risk_observations (
		id, node_logical_id, exit_identity_digest, provider, provider_schema_version,
		observed_at, expires_at, status, score, confidence, network_class,
		anonymizer_traits, evidence_digest, redacted_summary
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	_, err = r.db.ExecContext(ctx, query, observation.ID, observation.NodeLogicalID, observation.ExitIdentityDigest,
		observation.Provider, observation.ProviderSchemaVersion, observation.ObservedAt.Format(time.RFC3339Nano),
		observation.ExpiresAt.Format(time.RFC3339Nano), string(observation.Status), nullableInt(observation.Score),
		nullableInt(observation.Confidence), string(observation.NetworkClass), string(traits), observation.EvidenceDigest,
		observation.RedactedSummary)
	if err != nil {
		return fmt.Errorf("failed to insert IP risk observation: %w", err)
	}
	return nil
}

type riskPolicyRevisionRepository struct{ db *sql.DB }

// NewRiskPolicyRevisionRepository constructs versioned risk policy persistence.
func NewRiskPolicyRevisionRepository(db *sql.DB) domain.RiskPolicyRevisionRepository {
	return &riskPolicyRevisionRepository{db: db}
}

func (r *riskPolicyRevisionRepository) GetByID(ctx context.Context, id string) (*domain.RiskPolicyRevision, error) {
	return r.get(ctx, "WHERE id = ?", id)
}

func (r *riskPolicyRevisionRepository) GetActive(ctx context.Context) (*domain.RiskPolicyRevision, error) {
	return r.get(ctx, "WHERE active = 1 ORDER BY created_at DESC LIMIT 1")
}

func (r *riskPolicyRevisionRepository) get(ctx context.Context, suffix string, args ...any) (*domain.RiskPolicyRevision, error) {
	const query = `SELECT id, fusion_mode, max_observation_age_seconds, minimum_confidence,
		unknown_action, conflict_action, review_action, active, created_at, activated_at, deactivated_at
		FROM risk_policy_revisions `
	var revision domain.RiskPolicyRevision
	var mode, unknownAction, conflictAction, reviewAction string
	var active int
	var maxAge, confidence int
	var createdAt string
	var activatedAt, deactivatedAt sql.NullString
	var parseErr error
	row := r.db.QueryRowContext(ctx, query+suffix+";", args...)
	if err := row.Scan(&revision.RevisionID, &mode, &maxAge, &confidence, &unknownAction, &conflictAction,
		&reviewAction, &active, &createdAt, &activatedAt, &deactivatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("risk_policy_revision_not_found", "risk policy revision not found")
		}
		return nil, fmt.Errorf("failed to query risk policy revision: %w", err)
	}
	revision.ProviderSelection.Mode = domain.RiskFusionMode(mode)
	revision.MaxObservationAge = time.Duration(maxAge) * time.Second
	revision.MinimumConfidence = confidence
	revision.UnknownAction = domain.RiskAction(unknownAction)
	revision.ConflictAction = domain.RiskAction(conflictAction)
	revision.ReviewAction = domain.RiskAction(reviewAction)
	revision.Active = active != 0
	revision.CreatedAt, parseErr = parseIPRiskTime(createdAt, "created_at")
	if parseErr != nil {
		return nil, parseErr
	}
	if activatedAt.Valid {
		value, err := parseIPRiskTime(activatedAt.String, "activated_at")
		if err != nil {
			return nil, err
		}
		revision.ActivatedAt = &value
	}
	if deactivatedAt.Valid {
		value, err := parseIPRiskTime(deactivatedAt.String, "deactivated_at")
		if err != nil {
			return nil, err
		}
		revision.DeactivatedAt = &value
	}
	providerRows, err := r.db.QueryContext(ctx, "SELECT provider, schema_version FROM risk_policy_providers WHERE revision_id = ? ORDER BY position ASC;", revision.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query risk policy providers: %w", err)
	}
	for providerRows.Next() {
		var provider, version string
		if err := providerRows.Scan(&provider, &version); err != nil {
			providerRows.Close()
			return nil, err
		}
		revision.ProviderSelection.Providers = append(revision.ProviderSelection.Providers, domain.ProviderRef{Provider: provider, SchemaVersion: version})
	}
	if err := providerRows.Close(); err != nil {
		return nil, err
	}
	bandRows, err := r.db.QueryContext(ctx, "SELECT min_score, max_score, band, action FROM risk_policy_score_bands WHERE revision_id = ? ORDER BY position ASC;", revision.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query risk policy score bands: %w", err)
	}
	for bandRows.Next() {
		var band domain.ScoreBand
		if err := bandRows.Scan(&band.Min, &band.Max, &band.Band, &band.Action); err != nil {
			bandRows.Close()
			return nil, err
		}
		revision.ScoreBands = append(revision.ScoreBands, band)
	}
	if err := bandRows.Close(); err != nil {
		return nil, err
	}
	traitRows, err := r.db.QueryContext(ctx, "SELECT trait, action FROM risk_policy_trait_rules WHERE revision_id = ?;", revision.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query risk policy trait rules: %w", err)
	}
	for traitRows.Next() {
		var rule domain.TraitRule
		if err := traitRows.Scan(&rule.Trait, &rule.Action); err != nil {
			traitRows.Close()
			return nil, err
		}
		revision.TraitRules = append(revision.TraitRules, rule)
	}
	if err := traitRows.Close(); err != nil {
		return nil, err
	}
	return &revision, nil
}

func (r *riskPolicyRevisionRepository) Create(ctx context.Context, revision *domain.RiskPolicyRevision) error {
	if revision == nil {
		return domain.NewValidationError("missing_risk_policy_revision", "risk policy revision is required")
	}
	if revision.CreatedAt.IsZero() {
		revision.CreatedAt = domain.NowUTC()
	}
	if err := revision.RiskPolicy.Validate(); err != nil {
		return err
	}
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		const query = `INSERT INTO risk_policy_revisions (
			id, fusion_mode, max_observation_age_seconds, minimum_confidence,
			unknown_action, conflict_action, review_action, active, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`
		if _, err := tx.ExecContext(ctx, query, revision.RevisionID, string(revision.ProviderSelection.Mode),
			int(revision.MaxObservationAge/time.Second), revision.MinimumConfidence, revision.EffectiveUnknownAction(),
			revision.EffectiveConflictAction(), revision.EffectiveReviewAction(), boolInt(revision.Active), revision.CreatedAt.Format(time.RFC3339)); err != nil {
			return fmt.Errorf("failed to insert risk policy revision: %w", err)
		}
		for position, provider := range revision.ProviderSelection.Providers {
			if _, err := tx.ExecContext(ctx, "INSERT INTO risk_policy_providers (revision_id, provider, schema_version, position) VALUES (?, ?, ?, ?);", revision.RevisionID, provider.Provider, provider.SchemaVersion, position); err != nil {
				return fmt.Errorf("failed to insert risk policy provider: %w", err)
			}
		}
		for position, band := range revision.ScoreBands {
			if _, err := tx.ExecContext(ctx, "INSERT INTO risk_policy_score_bands (revision_id, min_score, max_score, band, action, position) VALUES (?, ?, ?, ?, ?, ?);", revision.RevisionID, band.Min, band.Max, band.Band, band.Action, position); err != nil {
				return fmt.Errorf("failed to insert risk policy score band: %w", err)
			}
		}
		for _, rule := range revision.TraitRules {
			if _, err := tx.ExecContext(ctx, "INSERT INTO risk_policy_trait_rules (revision_id, trait, action) VALUES (?, ?, ?);", revision.RevisionID, rule.Trait, rule.Action); err != nil {
				return fmt.Errorf("failed to insert risk policy trait rule: %w", err)
			}
		}
		return nil
	})
}

func (r *riskPolicyRevisionRepository) SetActive(ctx context.Context, id string, active bool) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		if active {
			if _, err := tx.ExecContext(ctx, "UPDATE risk_policy_revisions SET active = 0, deactivated_at = ? WHERE active = 1;", domain.NowUTC().Format(time.RFC3339)); err != nil {
				return fmt.Errorf("failed to deactivate risk policies: %w", err)
			}
		}
		var query string
		if active {
			query = "UPDATE risk_policy_revisions SET active = 1, activated_at = ?, deactivated_at = NULL WHERE id = ?;"
		} else {
			query = "UPDATE risk_policy_revisions SET active = 0, deactivated_at = ? WHERE id = ?;"
		}
		now := domain.NowUTC().Format(time.RFC3339)
		res, err := tx.ExecContext(ctx, query, now, id)
		if err != nil {
			return fmt.Errorf("failed to set risk policy active state: %w", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return domain.NewNotFoundError("risk_policy_revision_not_found", "risk policy revision not found")
		}
		return nil
	})
}

func (r *riskPolicyRevisionRepository) List(ctx context.Context, filter domain.RiskPolicyFilter) ([]domain.RiskPolicyRevision, int, error) {
	norm := filter.Normalize()
	var whereClauses []string
	var args []any
	if filter.Active != nil {
		whereClauses = append(whereClauses, "active = ?")
		args = append(args, boolInt(*filter.Active))
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM risk_policy_revisions" + whereSQL + ";"
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count risk policy revisions: %w", err)
	}

	offset := (norm.Page - 1) * norm.PageSize
	queryArgs := append(args, norm.PageSize, offset)
	idQuery := "SELECT id FROM risk_policy_revisions" + whereSQL + " ORDER BY created_at DESC LIMIT ? OFFSET ?;"
	rows, err := r.db.QueryContext(ctx, idQuery, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list risk policy revision ids: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	items := make([]domain.RiskPolicyRevision, 0, len(ids))
	for _, id := range ids {
		rev, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *rev)
	}
	return items, total, nil
}

type riskPolicyGroupBindingRepository struct {
	db *sql.DB
}

// NewRiskPolicyGroupBindingRepository constructs a SQLite implementation of domain.RiskPolicyGroupBindingRepository.
func NewRiskPolicyGroupBindingRepository(db *sql.DB) domain.RiskPolicyGroupBindingRepository {
	return &riskPolicyGroupBindingRepository{db: db}
}

func (r *riskPolicyGroupBindingRepository) GetByGroupID(ctx context.Context, groupID string) (*domain.RiskPolicyGroupBinding, error) {
	const query = `SELECT group_id, policy_revision_id, created_at, updated_at
		FROM risk_policy_group_bindings WHERE group_id = ?;`
	var binding domain.RiskPolicyGroupBinding
	var createdStr, updatedStr string
	err := r.db.QueryRowContext(ctx, query, groupID).Scan(
		&binding.GroupID,
		&binding.PolicyRevisionID,
		&createdStr,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("group_binding_not_found", fmt.Sprintf("no risk policy binding for group %s", groupID))
		}
		return nil, fmt.Errorf("failed to query group binding: %w", err)
	}
	var parseErr error
	binding.CreatedAt, parseErr = parseIPRiskTime(createdStr, "created_at")
	if parseErr != nil {
		return nil, parseErr
	}
	binding.UpdatedAt, parseErr = parseIPRiskTime(updatedStr, "updated_at")
	if parseErr != nil {
		return nil, parseErr
	}
	return &binding, nil
}

func (r *riskPolicyGroupBindingRepository) List(ctx context.Context) ([]domain.RiskPolicyGroupBinding, error) {
	const query = `SELECT group_id, policy_revision_id, created_at, updated_at
		FROM risk_policy_group_bindings ORDER BY group_id ASC;`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list group bindings: %w", err)
	}
	defer rows.Close()

	items := make([]domain.RiskPolicyGroupBinding, 0)
	for rows.Next() {
		var binding domain.RiskPolicyGroupBinding
		var createdStr, updatedStr string
		if err := rows.Scan(&binding.GroupID, &binding.PolicyRevisionID, &createdStr, &updatedStr); err != nil {
			return nil, err
		}
		var parseErr error
		binding.CreatedAt, parseErr = parseIPRiskTime(createdStr, "created_at")
		if parseErr != nil {
			return nil, parseErr
		}
		binding.UpdatedAt, parseErr = parseIPRiskTime(updatedStr, "updated_at")
		if parseErr != nil {
			return nil, parseErr
		}
		items = append(items, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *riskPolicyGroupBindingRepository) Set(ctx context.Context, binding *domain.RiskPolicyGroupBinding) error {
	if binding == nil {
		return domain.NewValidationError("missing_binding", "group binding is required")
	}
	if err := binding.Validate(); err != nil {
		return err
	}
	const query = `INSERT INTO risk_policy_group_bindings (group_id, policy_revision_id, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (group_id) DO UPDATE SET
			policy_revision_id = excluded.policy_revision_id,
			updated_at = excluded.updated_at;`
	_, err := r.db.ExecContext(ctx, query,
		binding.GroupID,
		binding.PolicyRevisionID,
		binding.CreatedAt.Format(time.RFC3339),
		binding.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to save risk policy group binding: %w", err)
	}
	return nil
}

func (r *riskPolicyGroupBindingRepository) Delete(ctx context.Context, groupID string) error {
	const query = `DELETE FROM risk_policy_group_bindings WHERE group_id = ?;`
	res, err := r.db.ExecContext(ctx, query, groupID)
	if err != nil {
		return fmt.Errorf("failed to delete risk policy group binding: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("group_binding_not_found", fmt.Sprintf("no risk policy binding for group %s", groupID))
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
