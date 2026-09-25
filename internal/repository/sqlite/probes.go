package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

type probeRunRepository struct {
	db *sql.DB
}

// NewProbeRunRepository constructs a SQLite implementation of domain.ProbeRunRepository.
func NewProbeRunRepository(db *sql.DB) domain.ProbeRunRepository {
	return &probeRunRepository{db: db}
}

func (r *probeRunRepository) GetByID(ctx context.Context, id string) (*domain.ProbeRun, error) {
	const query = `
	SELECT id, idempotency_key, actor_scope, config_revision, state, deadline_at, created_at, updated_at
	FROM probe_runs
	WHERE id = ?;`

	var run domain.ProbeRun
	var stateStr, deadlineStr, createdStr, updatedStr string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&run.ID,
		&run.IdempotencyKey,
		&run.ActorScope,
		&run.ConfigRevision,
		&stateStr,
		&deadlineStr,
		&createdStr,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("probe_run_not_found", fmt.Sprintf("probe run %s not found", id))
		}
		return nil, fmt.Errorf("failed to query probe run %s: %w", id, err)
	}

	run.State = domain.ProbeRunState(stateStr)
	run.DeadlineAt, _ = time.Parse(time.RFC3339, deadlineStr)
	run.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	run.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &run, nil
}

func (r *probeRunRepository) GetByIdempotencyKey(ctx context.Context, actorScope string, key string) (*domain.ProbeRun, error) {
	const query = `
	SELECT id, idempotency_key, actor_scope, config_revision, state, deadline_at, created_at, updated_at
	FROM probe_runs
	WHERE actor_scope = ? AND idempotency_key = ?;`

	var run domain.ProbeRun
	var stateStr, deadlineStr, createdStr, updatedStr string

	err := r.db.QueryRowContext(ctx, query, actorScope, key).Scan(
		&run.ID,
		&run.IdempotencyKey,
		&run.ActorScope,
		&run.ConfigRevision,
		&stateStr,
		&deadlineStr,
		&createdStr,
		&updatedStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("probe_run_not_found", fmt.Sprintf("probe run for scope %s and key %s not found", actorScope, key))
		}
		return nil, fmt.Errorf("failed to query probe run by idempotency key: %w", err)
	}

	run.State = domain.ProbeRunState(stateStr)
	run.DeadlineAt, _ = time.Parse(time.RFC3339, deadlineStr)
	run.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	run.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	return &run, nil
}

func (r *probeRunRepository) Create(ctx context.Context, run *domain.ProbeRun) error {
	const query = `
	INSERT INTO probe_runs (id, idempotency_key, actor_scope, config_revision, state, deadline_at, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?);`

	nowStr := domain.NowUTC().Format(time.RFC3339)
	deadlineStr := run.DeadlineAt.Format(time.RFC3339)
	createdStr := run.CreatedAt.Format(time.RFC3339)
	if run.CreatedAt.IsZero() {
		createdStr = nowStr
	}
	updatedStr := run.UpdatedAt.Format(time.RFC3339)
	if run.UpdatedAt.IsZero() {
		updatedStr = nowStr
	}

	_, err := r.db.ExecContext(ctx, query,
		run.ID,
		run.IdempotencyKey,
		run.ActorScope,
		run.ConfigRevision,
		string(run.State),
		deadlineStr,
		createdStr,
		updatedStr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert probe run: %w", err)
	}
	return nil
}

func (r *probeRunRepository) UpdateState(ctx context.Context, id string, state domain.ProbeRunState) error {
	const query = `
	UPDATE probe_runs SET state = ?, updated_at = ?
	WHERE id = ?;`

	updatedStr := domain.NowUTC().Format(time.RFC3339)
	res, err := r.db.ExecContext(ctx, query, string(state), updatedStr, id)
	if err != nil {
		return fmt.Errorf("failed to update probe run state: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return domain.NewNotFoundError("probe_run_not_found", fmt.Sprintf("probe run %s not found", id))
	}
	return nil
}

func (r *probeRunRepository) List(ctx context.Context, state *domain.ProbeRunState, page, pageSize int) ([]domain.ProbeRun, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	where := ""
	args := make([]any, 0, 1)
	if state != nil {
		where = " WHERE state = ?"
		args = append(args, string(*state))
	}
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM probe_runs"+where+";", args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count probe runs: %w", err)
	}
	query := "SELECT id, idempotency_key, actor_scope, config_revision, state, deadline_at, created_at, updated_at FROM probe_runs" + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?;"
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list probe runs: %w", err)
	}
	defer rows.Close()
	items := make([]domain.ProbeRun, 0)
	for rows.Next() {
		var run domain.ProbeRun
		var stateStr, deadlineStr, createdStr, updatedStr string
		if err := rows.Scan(&run.ID, &run.IdempotencyKey, &run.ActorScope, &run.ConfigRevision, &stateStr, &deadlineStr, &createdStr, &updatedStr); err != nil {
			return nil, 0, fmt.Errorf("failed to scan probe run: %w", err)
		}
		run.State = domain.ProbeRunState(stateStr)
		run.DeadlineAt, _ = time.Parse(time.RFC3339, deadlineStr)
		run.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		run.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		items = append(items, run)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating probe runs: %w", err)
	}
	return items, total, nil
}

func (r *probeRunRepository) ListActive(ctx context.Context) ([]domain.ProbeRun, error) {
	const query = `
	SELECT id, idempotency_key, actor_scope, config_revision, state, deadline_at, created_at, updated_at
	FROM probe_runs
	WHERE state IN ('queued', 'running')
	ORDER BY created_at ASC;`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query active probe runs: %w", err)
	}
	defer rows.Close()

	runs := make([]domain.ProbeRun, 0)
	for rows.Next() {
		var run domain.ProbeRun
		var stateStr, deadlineStr, createdStr, updatedStr string

		err := rows.Scan(
			&run.ID,
			&run.IdempotencyKey,
			&run.ActorScope,
			&run.ConfigRevision,
			&stateStr,
			&deadlineStr,
			&createdStr,
			&updatedStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan active probe run: %w", err)
		}

		run.State = domain.ProbeRunState(stateStr)
		run.DeadlineAt, _ = time.Parse(time.RFC3339, deadlineStr)
		run.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		run.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating active probe runs: %w", err)
	}
	return runs, nil
}

// ProbeObservationRepository implementation

type probeObservationRepository struct {
	db *sql.DB
}

// NewProbeObservationRepository constructs a SQLite implementation of domain.ProbeObservationRepository.
func NewProbeObservationRepository(db *sql.DB) domain.ProbeObservationRepository {
	return &probeObservationRepository{db: db}
}

func (r *probeObservationRepository) GetByID(ctx context.Context, id string) (*domain.ProbeObservation, error) {
	const query = `
	SELECT id, probe_run_id, node_logical_id, kind, verdict, evidence_digest, observed_at, latency_ms, redacted_summary
	FROM probe_observations
	WHERE id = ?;`

	var obs domain.ProbeObservation
	var kindStr, verdictStr, observedStr string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&obs.ID,
		&obs.ProbeRunID,
		&obs.NodeLogicalID,
		&kindStr,
		&verdictStr,
		&obs.EvidenceDigest,
		&observedStr,
		&obs.LatencyMS,
		&obs.RedactedSummary,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("observation_not_found", fmt.Sprintf("probe observation %s not found", id))
		}
		return nil, fmt.Errorf("failed to query probe observation %s: %w", id, err)
	}

	obs.Kind = domain.ProbeKind(kindStr)
	obs.Verdict = domain.ProbeVerdict(verdictStr)
	obs.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)

	return &obs, nil
}

func (r *probeObservationRepository) ListByRun(ctx context.Context, runID string) ([]domain.ProbeObservation, error) {
	const query = `
	SELECT id, probe_run_id, node_logical_id, kind, verdict, evidence_digest, observed_at, latency_ms, redacted_summary
	FROM probe_observations
	WHERE probe_run_id = ?
	ORDER BY observed_at ASC;`

	rows, err := r.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to query probe observations for run %s: %w", runID, err)
	}
	defer rows.Close()

	items := make([]domain.ProbeObservation, 0)
	for rows.Next() {
		var obs domain.ProbeObservation
		var kindStr, verdictStr, observedStr string

		err := rows.Scan(
			&obs.ID,
			&obs.ProbeRunID,
			&obs.NodeLogicalID,
			&kindStr,
			&verdictStr,
			&obs.EvidenceDigest,
			&observedStr,
			&obs.LatencyMS,
			&obs.RedactedSummary,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan probe observation: %w", err)
		}

		obs.Kind = domain.ProbeKind(kindStr)
		obs.Verdict = domain.ProbeVerdict(verdictStr)
		obs.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)

		items = append(items, obs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating probe observations: %w", err)
	}
	return items, nil
}

func (r *probeObservationRepository) ListByNode(ctx context.Context, nodeLogicalID string, limit int) ([]domain.ProbeObservation, error) {
	if limit <= 0 {
		limit = 20
	}

	const query = `
	SELECT id, probe_run_id, node_logical_id, kind, verdict, evidence_digest, observed_at, latency_ms, redacted_summary
	FROM probe_observations
	WHERE node_logical_id = ?
	ORDER BY observed_at DESC
	LIMIT ?;`

	rows, err := r.db.QueryContext(ctx, query, nodeLogicalID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query observations for node %s: %w", nodeLogicalID, err)
	}
	defer rows.Close()

	items := make([]domain.ProbeObservation, 0)
	for rows.Next() {
		var obs domain.ProbeObservation
		var kindStr, verdictStr, observedStr string

		err := rows.Scan(
			&obs.ID,
			&obs.ProbeRunID,
			&obs.NodeLogicalID,
			&kindStr,
			&verdictStr,
			&obs.EvidenceDigest,
			&observedStr,
			&obs.LatencyMS,
			&obs.RedactedSummary,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan probe observation: %w", err)
		}

		obs.Kind = domain.ProbeKind(kindStr)
		obs.Verdict = domain.ProbeVerdict(verdictStr)
		obs.ObservedAt, _ = time.Parse(time.RFC3339, observedStr)

		items = append(items, obs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating node observations: %w", err)
	}
	return items, nil
}

func (r *probeObservationRepository) Create(ctx context.Context, obs *domain.ProbeObservation) error {
	const query = `
	INSERT INTO probe_observations (
		id, probe_run_id, node_logical_id, kind, verdict,
		evidence_digest, observed_at, latency_ms, redacted_summary
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`

	observedStr := obs.ObservedAt.Format(time.RFC3339)
	if obs.ObservedAt.IsZero() {
		observedStr = domain.NowUTC().Format(time.RFC3339)
	}

	_, err := r.db.ExecContext(ctx, query,
		obs.ID,
		obs.ProbeRunID,
		obs.NodeLogicalID,
		string(obs.Kind),
		string(obs.Verdict),
		obs.EvidenceDigest,
		observedStr,
		obs.LatencyMS,
		obs.RedactedSummary,
	)
	if err != nil {
		return fmt.Errorf("failed to insert probe observation: %w", err)
	}
	return nil
}
