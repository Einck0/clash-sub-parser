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

type probeScheduleRepository struct {
	db *sql.DB
}

// NewProbeScheduleRepository constructs a SQLite implementation of domain.ProbeScheduleRepository.
func NewProbeScheduleRepository(db *sql.DB) domain.ProbeScheduleRepository {
	return &probeScheduleRepository{db: db}
}

func parseDBTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("unparseable db timestamp: %q", s)
}

func (r *probeScheduleRepository) Get(ctx context.Context) (*domain.ProbeSchedule, error) {
	const query = `
	SELECT enabled, interval_seconds, kinds, next_due_at, generation, updated_at
	FROM probe_schedules
	WHERE id = 1;`

	var (
		enabledInt   int
		intervalSec  int
		kindsJSON    string
		nextDueAtStr sql.NullString
		generation   int64
		updatedAtStr string
	)

	err := r.db.QueryRowContext(ctx, query).Scan(
		&enabledInt,
		&intervalSec,
		&kindsJSON,
		&nextDueAtStr,
		&generation,
		&updatedAtStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("probe_schedule_not_found", "probe schedule not configured")
		}
		return nil, fmt.Errorf("failed to query probe schedule: %w", err)
	}

	var kinds []domain.ProbeKind
	if err := json.Unmarshal([]byte(kindsJSON), &kinds); err != nil {
		kinds = []domain.ProbeKind{domain.ProbeKindBaseline}
	}

	updatedAt, _ := parseDBTime(updatedAtStr)
	var nextDueAt *time.Time
	if nextDueAtStr.Valid && strings.TrimSpace(nextDueAtStr.String) != "" {
		if t, err := parseDBTime(nextDueAtStr.String); err == nil {
			nextDueAt = &t
		}
	}

	schedule := &domain.ProbeSchedule{
		Enabled:         enabledInt == 1,
		IntervalSeconds: intervalSec,
		Kinds:           kinds,
		NextDueAt:       nextDueAt,
		Generation:      generation,
		UpdatedAt:       updatedAt,
	}

	return schedule, nil
}

func (r *probeScheduleRepository) Update(ctx context.Context, schedule *domain.ProbeSchedule) error {
	if schedule == nil {
		return domain.NewValidationError("nil_schedule", "probe schedule is required")
	}
	if err := schedule.Validate(); err != nil {
		return err
	}

	kindsJSON, err := json.Marshal(schedule.Kinds)
	if err != nil {
		return domain.NewValidationError("invalid_kinds", fmt.Sprintf("failed to encode kinds: %v", err))
	}

	now := domain.NowUTC()
	schedule.UpdatedAt = now
	updatedAtStr := now.Format(time.RFC3339)

	var nextDueAtArg any
	if schedule.NextDueAt != nil && !schedule.NextDueAt.IsZero() {
		nextDueAtArg = schedule.NextDueAt.UTC().Format(time.RFC3339)
	}

	enabledInt := 0
	if schedule.Enabled {
		enabledInt = 1
	}

	const updateQuery = `
	UPDATE probe_schedules
	SET enabled = ?, interval_seconds = ?, kinds = ?, next_due_at = ?, generation = ?, updated_at = ?
	WHERE id = 1;`

	res, err := r.db.ExecContext(ctx, updateQuery,
		enabledInt,
		schedule.IntervalSeconds,
		string(kindsJSON),
		nextDueAtArg,
		schedule.Generation,
		updatedAtStr,
	)
	if err != nil {
		return fmt.Errorf("failed to update probe schedule: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected for probe schedule update: %w", err)
	}

	if rows == 0 {
		const insertQuery = `
		INSERT INTO probe_schedules (id, enabled, interval_seconds, kinds, next_due_at, generation, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?);`

		if _, err := r.db.ExecContext(ctx, insertQuery,
			enabledInt,
			schedule.IntervalSeconds,
			string(kindsJSON),
			nextDueAtArg,
			schedule.Generation,
			updatedAtStr,
		); err != nil {
			return fmt.Errorf("failed to insert probe schedule: %w", err)
		}
	}

	return nil
}

func (r *probeScheduleRepository) scanBatch(row interface {
	Scan(dest ...any) error
}) (*domain.ProbeBatch, error) {
	var (
		b            domain.ProbeBatch
		windowAtStr  string
		leaseStr     sql.NullString
		stateStr     string
		runIDsJSON   string
		createdAtStr string
		updatedAtStr string
	)

	err := row.Scan(
		&b.ID,
		&windowAtStr,
		&b.Generation,
		&b.Owner,
		&leaseStr,
		&stateStr,
		&runIDsJSON,
		&b.Counts.TotalNodes,
		&b.Counts.DispatchedRuns,
		&b.Counts.CompletedRuns,
		&b.Counts.SkippedNodes,
		&b.RedactedError,
		&createdAtStr,
		&updatedAtStr,
	)
	if err != nil {
		return nil, err
	}

	b.WindowAt, _ = parseDBTime(windowAtStr)
	b.CreatedAt, _ = parseDBTime(createdAtStr)
	b.UpdatedAt, _ = parseDBTime(updatedAtStr)
	b.State = domain.ProbeBatchState(stateStr)

	if leaseStr.Valid && strings.TrimSpace(leaseStr.String) != "" {
		if t, err := parseDBTime(leaseStr.String); err == nil {
			b.LeaseUntil = &t
		}
	}

	if strings.TrimSpace(runIDsJSON) != "" {
		_ = json.Unmarshal([]byte(runIDsJSON), &b.RunIDs)
	}
	if b.RunIDs == nil {
		b.RunIDs = []string{}
	}

	return &b, nil
}

func (r *probeScheduleRepository) GetBatchByID(ctx context.Context, id string) (*domain.ProbeBatch, error) {
	const query = `
	SELECT id, window_at, generation, owner, lease_until, state, run_ids,
	       total_nodes, dispatched_runs, completed_runs, skipped_nodes, redacted_error,
	       created_at, updated_at
	FROM probe_batches
	WHERE id = ?;`

	batch, err := r.scanBatch(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("probe_batch_not_found", fmt.Sprintf("probe batch %s not found", id))
		}
		return nil, fmt.Errorf("failed to query probe batch %s: %w", id, err)
	}

	return batch, nil
}

func (r *probeScheduleRepository) GetBatchByWindow(ctx context.Context, generation int64, windowAt time.Time) (*domain.ProbeBatch, error) {
	const query = `
	SELECT id, window_at, generation, owner, lease_until, state, run_ids,
	       total_nodes, dispatched_runs, completed_runs, skipped_nodes, redacted_error,
	       created_at, updated_at
	FROM probe_batches
	WHERE generation = ? AND window_at = ?;`

	windowStr := windowAt.UTC().Format(time.RFC3339)
	batch, err := r.scanBatch(r.db.QueryRowContext(ctx, query, generation, windowStr))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("probe_batch_not_found", fmt.Sprintf("probe batch for generation %d window %s not found", generation, windowStr))
		}
		return nil, fmt.Errorf("failed to query probe batch by window: %w", err)
	}

	return batch, nil
}

func (r *probeScheduleRepository) ListBatches(ctx context.Context, page, pageSize int) ([]domain.ProbeBatch, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM probe_batches;`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count probe batches: %w", err)
	}

	offset := (page - 1) * pageSize
	const query = `
	SELECT id, window_at, generation, owner, lease_until, state, run_ids,
	       total_nodes, dispatched_runs, completed_runs, skipped_nodes, redacted_error,
	       created_at, updated_at
	FROM probe_batches
	ORDER BY window_at DESC, created_at DESC
	LIMIT ? OFFSET ?;`

	rows, err := r.db.QueryContext(ctx, query, pageSize, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list probe batches: %w", err)
	}
	defer rows.Close()

	batches := make([]domain.ProbeBatch, 0, pageSize)
	for rows.Next() {
		batch, err := r.scanBatch(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan probe batch: %w", err)
		}
		batches = append(batches, *batch)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error listing probe batches: %w", err)
	}

	return batches, total, nil
}

func (r *probeScheduleRepository) CreateBatch(ctx context.Context, batch *domain.ProbeBatch) error {
	if batch == nil {
		return domain.NewValidationError("nil_batch", "probe batch is required")
	}
	if strings.TrimSpace(batch.ID) == "" {
		return domain.NewValidationError("missing_batch_id", "probe batch ID is required")
	}

	runIDsJSON, err := json.Marshal(batch.RunIDs)
	if err != nil {
		runIDsJSON = []byte("[]")
	}

	now := domain.NowUTC()
	if batch.CreatedAt.IsZero() {
		batch.CreatedAt = now
	}
	if batch.UpdatedAt.IsZero() {
		batch.UpdatedAt = now
	}

	windowStr := batch.WindowAt.UTC().Format(time.RFC3339)
	createdAtStr := batch.CreatedAt.UTC().Format(time.RFC3339)
	updatedAtStr := batch.UpdatedAt.UTC().Format(time.RFC3339)

	var leaseUntilArg any
	if batch.LeaseUntil != nil && !batch.LeaseUntil.IsZero() {
		leaseUntilArg = batch.LeaseUntil.UTC().Format(time.RFC3339)
	}

	stateStr := string(batch.State)
	if stateStr == "" {
		stateStr = string(domain.ProbeBatchStatePending)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx for batch create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const insertBatch = `
	INSERT INTO probe_batches (
		id, window_at, generation, owner, lease_until, state, run_ids,
		total_nodes, dispatched_runs, completed_runs, skipped_nodes, redacted_error,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	_, err = tx.ExecContext(ctx, insertBatch,
		batch.ID,
		windowStr,
		batch.Generation,
		batch.Owner,
		leaseUntilArg,
		stateStr,
		string(runIDsJSON),
		batch.Counts.TotalNodes,
		batch.Counts.DispatchedRuns,
		batch.Counts.CompletedRuns,
		batch.Counts.SkippedNodes,
		batch.RedactedError,
		createdAtStr,
		updatedAtStr,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return domain.NewConflictError("probe_batch_already_exists",
				fmt.Sprintf("probe batch already exists for id %s or generation %d window %s: %v", batch.ID, batch.Generation, windowStr, err))
		}
		return fmt.Errorf("failed to insert probe batch: %w", err)
	}

	const insertRunAssoc = `INSERT OR IGNORE INTO probe_batch_runs (batch_id, probe_run_id) VALUES (?, ?);`
	for _, runID := range batch.RunIDs {
		if strings.TrimSpace(runID) != "" {
			if _, err := tx.ExecContext(ctx, insertRunAssoc, batch.ID, runID); err != nil {
				return fmt.Errorf("failed to link batch run %s: %w", runID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch create: %w", err)
	}

	return nil
}

func (r *probeScheduleRepository) UpdateBatch(ctx context.Context, batch *domain.ProbeBatch) error {
	if batch == nil {
		return domain.NewValidationError("nil_batch", "probe batch is required")
	}

	runIDsJSON, err := json.Marshal(batch.RunIDs)
	if err != nil {
		runIDsJSON = []byte("[]")
	}

	now := domain.NowUTC()
	batch.UpdatedAt = now
	updatedAtStr := now.Format(time.RFC3339)

	var leaseUntilArg any
	if batch.LeaseUntil != nil && !batch.LeaseUntil.IsZero() {
		leaseUntilArg = batch.LeaseUntil.UTC().Format(time.RFC3339)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx for batch update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const updateBatch = `
	UPDATE probe_batches
	SET owner = ?, lease_until = ?, state = ?, run_ids = ?,
	    total_nodes = ?, dispatched_runs = ?, completed_runs = ?, skipped_nodes = ?,
	    redacted_error = ?, updated_at = ?
	WHERE id = ?;`

	res, err := tx.ExecContext(ctx, updateBatch,
		batch.Owner,
		leaseUntilArg,
		string(batch.State),
		string(runIDsJSON),
		batch.Counts.TotalNodes,
		batch.Counts.DispatchedRuns,
		batch.Counts.CompletedRuns,
		batch.Counts.SkippedNodes,
		batch.RedactedError,
		updatedAtStr,
		batch.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update probe batch: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return domain.NewNotFoundError("probe_batch_not_found", fmt.Sprintf("probe batch %s not found", batch.ID))
	}

	const insertRunAssoc = `INSERT OR IGNORE INTO probe_batch_runs (batch_id, probe_run_id) VALUES (?, ?);`
	for _, runID := range batch.RunIDs {
		if strings.TrimSpace(runID) != "" {
			if _, err := tx.ExecContext(ctx, insertRunAssoc, batch.ID, runID); err != nil {
				return fmt.Errorf("failed to link batch run %s: %w", runID, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch update: %w", err)
	}

	return nil
}

func (r *probeScheduleRepository) AcquireLease(ctx context.Context, batchID string, owner string, leaseDuration time.Duration) (bool, error) {
	if strings.TrimSpace(batchID) == "" || strings.TrimSpace(owner) == "" {
		return false, domain.NewValidationError("invalid_lease_args", "batchID and owner are required to acquire lease")
	}

	now := domain.NowUTC()
	nowStr := now.Format(time.RFC3339)
	leaseUntilStr := now.Add(leaseDuration).Format(time.RFC3339)

	const casQuery = `
	UPDATE probe_batches
	SET owner = ?, lease_until = ?, updated_at = ?
	WHERE id = ?
	  AND state NOT IN ('succeeded', 'failed', 'cancelled', 'expired')
	  AND (owner = '' OR owner = ? OR lease_until IS NULL OR lease_until < ?);`

	res, err := r.db.ExecContext(ctx, casQuery,
		owner,
		leaseUntilStr,
		nowStr,
		batchID,
		owner,
		nowStr,
	)
	if err != nil {
		return false, fmt.Errorf("failed to execute acquire lease CAS query: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read rows affected for lease acquire: %w", err)
	}

	if rows > 0 {
		return true, nil
	}

	// Verify if batch exists or if lease was held by another unexpired owner / batch terminal
	var existingID string
	err = r.db.QueryRowContext(ctx, `SELECT id FROM probe_batches WHERE id = ?;`, batchID).Scan(&existingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, domain.NewNotFoundError("probe_batch_not_found", fmt.Sprintf("probe batch %s not found", batchID))
		}
		return false, fmt.Errorf("failed to query batch existence: %w", err)
	}

	return false, nil
}

func (r *probeScheduleRepository) HeartbeatLease(ctx context.Context, batchID string, owner string, leaseDuration time.Duration) error {
	if strings.TrimSpace(batchID) == "" || strings.TrimSpace(owner) == "" {
		return domain.NewValidationError("invalid_lease_args", "batchID and owner are required to heartbeat lease")
	}

	now := domain.NowUTC()
	nowStr := now.Format(time.RFC3339)
	leaseUntilStr := now.Add(leaseDuration).Format(time.RFC3339)

	const heartbeatQuery = `
	UPDATE probe_batches
	SET lease_until = ?, updated_at = ?
	WHERE id = ?
	  AND owner = ?
	  AND state NOT IN ('succeeded', 'failed', 'cancelled', 'expired');`

	res, err := r.db.ExecContext(ctx, heartbeatQuery, leaseUntilStr, nowStr, batchID, owner)
	if err != nil {
		return fmt.Errorf("failed to execute heartbeat lease query: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected for heartbeat: %w", err)
	}

	if rows == 0 {
		var (
			existingOwner string
			existingState string
		)
		err := r.db.QueryRowContext(ctx, `SELECT owner, state FROM probe_batches WHERE id = ?;`, batchID).
			Scan(&existingOwner, &existingState)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NewNotFoundError("probe_batch_not_found", fmt.Sprintf("probe batch %s not found", batchID))
			}
			return fmt.Errorf("failed to query batch for heartbeat validation: %w", err)
		}
		return domain.NewConflictError("lost_lease",
			fmt.Sprintf("lost lease for batch %s: owner is %q (expected %q), state is %s", batchID, existingOwner, owner, existingState))
	}

	return nil
}

func (r *probeScheduleRepository) ReleaseLease(ctx context.Context, batchID string, owner string) error {
	if strings.TrimSpace(batchID) == "" || strings.TrimSpace(owner) == "" {
		return nil
	}

	nowStr := domain.NowUTC().Format(time.RFC3339)
	const releaseQuery = `
	UPDATE probe_batches
	SET owner = '', lease_until = NULL, updated_at = ?
	WHERE id = ? AND owner = ?;`

	_, err := r.db.ExecContext(ctx, releaseQuery, nowStr, batchID, owner)
	if err != nil {
		return fmt.Errorf("failed to release lease: %w", err)
	}
	return nil
}
