package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"clash-sub-parser/internal/domain"
)

type sqliteProbeRepository struct {
	db *SQLiteDB
}

func scanProbeResult(s rowScanner) (*domain.NodeProbeResult, error) {
	var (
		res                               domain.NodeProbeResult
		port                              sql.NullInt64
		statusStr                         string
		latency                           sql.NullInt64
		speed                             sql.NullFloat64
		ip, country, org, mediaRaw, errStr sql.NullString
		asn                               sql.NullInt64
		checkedAt                         int64
	)

	err := s.Scan(
		&res.ID,
		&res.NodeKey,
		&res.Name,
		&res.Server,
		&port,
		&res.Type,
		&statusStr,
		&latency,
		&speed,
		&ip,
		&country,
		&asn,
		&org,
		&mediaRaw,
		&errStr,
		&checkedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan probe result: %w", err)
	}

	if port.Valid {
		res.Port = int(port.Int64)
	}
	res.Status = domain.ProbeStatus(statusStr)
	res.LatencyMs = int64PtrFromNull(latency)
	res.SpeedMbps = float64PtrFromNull(speed)
	res.IP = stringFromNull(ip)
	res.Country = stringFromNull(country)
	res.ASN = int64PtrFromNull(asn)
	res.Organization = stringFromNull(org)
	res.Error = stringFromNull(errStr)
	res.CheckedAt = checkedAt

	_ = unmarshalJSONSafe(stringFromNull(mediaRaw), &res.Media)

	return &res, nil
}

const probeSelectFields = `
	id, node_key, name, server, port, type, status,
	latency_ms, speed_mbps, ip, country, asn, organization,
	media, error, checked_at
`

func (r *sqliteProbeRepository) GetByNodeKey(ctx context.Context, nodeKey string) (*domain.NodeProbeResult, error) {
	query := fmt.Sprintf("SELECT %s FROM node_probe_results WHERE node_key = ?", probeSelectFields)
	row := r.db.QueryRowContext(ctx, query, nodeKey)
	return scanProbeResult(row)
}

func (r *sqliteProbeRepository) List(ctx context.Context, limit int, offset int) ([]*domain.NodeProbeResult, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf("SELECT %s FROM node_probe_results ORDER BY checked_at DESC, id DESC LIMIT ? OFFSET ?", probeSelectFields)
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list probe results: %w", err)
	}
	defer rows.Close()

	var result []*domain.NodeProbeResult
	for rows.Next() {
		pr, err := scanProbeResult(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, pr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate probe results: %w", err)
	}
	return result, nil
}

const probeUpsertQuery = `
	INSERT INTO node_probe_results (
		node_key, name, server, port, type, status,
		latency_ms, speed_mbps, ip, country, asn, organization,
		media, error, checked_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(node_key) DO UPDATE SET
		name = excluded.name,
		server = excluded.server,
		port = excluded.port,
		type = excluded.type,
		status = excluded.status,
		latency_ms = excluded.latency_ms,
		speed_mbps = excluded.speed_mbps,
		ip = excluded.ip,
		country = excluded.country,
		asn = excluded.asn,
		organization = excluded.organization,
		media = excluded.media,
		error = excluded.error,
		checked_at = excluded.checked_at
`

func (r *sqliteProbeRepository) Upsert(ctx context.Context, result *domain.NodeProbeResult) error {
	if result == nil {
		return ErrNilEntity
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("validate probe result: %w", err)
	}
	if result.CheckedAt == 0 {
		result.CheckedAt = time.Now().Unix()
	}

	var portVal any = nil
	if result.Port > 0 {
		portVal = result.Port
	}

	res, err := r.db.ExecContext(ctx, probeUpsertQuery,
		result.NodeKey,
		result.Name,
		result.Server,
		portVal,
		result.Type,
		string(result.Status),
		nullInt64FromPtr(result.LatencyMs),
		nullFloat64FromPtr(result.SpeedMbps),
		nullStringVal(result.IP),
		nullStringVal(result.Country),
		nullInt64FromPtr(result.ASN),
		nullStringVal(result.Organization),
		marshalJSONSafe(result.Media, "{}"),
		nullStringVal(result.Error),
		result.CheckedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert probe result: %w", err)
	}

	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		result.ID = id
	}
	return nil
}

func (r *sqliteProbeRepository) BatchUpsert(ctx context.Context, results []*domain.NodeProbeResult) (int64, error) {
	if len(results) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx for probe batch upsert: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, probeUpsertQuery)
	if err != nil {
		return 0, fmt.Errorf("prepare probe upsert stmt: %w", err)
	}
	defer stmt.Close()

	var affected int64
	now := time.Now().Unix()

	for _, result := range results {
		if result == nil {
			return 0, ErrNilEntity
		}
		if err := result.Validate(); err != nil {
			return 0, fmt.Errorf("validate probe result: %w", err)
		}
		if result.CheckedAt == 0 {
			result.CheckedAt = now
		}

		var portVal any = nil
		if result.Port > 0 {
			portVal = result.Port
		}

		res, err := stmt.ExecContext(ctx,
			result.NodeKey,
			result.Name,
			result.Server,
			portVal,
			result.Type,
			string(result.Status),
			nullInt64FromPtr(result.LatencyMs),
			nullFloat64FromPtr(result.SpeedMbps),
			nullStringVal(result.IP),
			nullStringVal(result.Country),
			nullInt64FromPtr(result.ASN),
			nullStringVal(result.Organization),
			marshalJSONSafe(result.Media, "{}"),
			nullStringVal(result.Error),
			result.CheckedAt,
		)
		if err != nil {
			return 0, fmt.Errorf("batch upsert probe result %s: %w", result.NodeKey, err)
		}

		id, err := res.LastInsertId()
		if err == nil && id > 0 {
			result.ID = id
		}
		affected++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit probe batch upsert: %w", err)
	}

	return affected, nil
}

func (r *sqliteProbeRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT count(*) FROM node_probe_results").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count probe results: %w", err)
	}
	return count, nil
}

func (r *sqliteProbeRepository) DeleteAll(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM node_probe_results")
	if err != nil {
		return fmt.Errorf("delete all probe results: %w", err)
	}
	return nil
}

