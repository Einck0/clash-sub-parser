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

// --- Subscription Payload Repository ---

type subscriptionPayloadRepository struct {
	db *sql.DB
}

func NewSubscriptionPayloadRepository(db *sql.DB) domain.SubscriptionPayloadRepository {
	return &subscriptionPayloadRepository{db: db}
}

func (r *subscriptionPayloadRepository) Save(ctx context.Context, p *domain.SubscriptionPayload) error {
	if p == nil {
		return domain.NewValidationError("missing_payload", "subscription payload is required")
	}
	if p.ID == "" {
		p.ID = "payload_" + domain.MustNewUUIDv7()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = domain.NowUTC()
	}
	if p.HeadersJSON == "" {
		p.HeadersJSON = "{}"
	}

	const query = `
	INSERT INTO subscription_payloads (
		id, subscription_id, fetch_id, content_digest, body_blob,
		http_status, headers_json, pinned, created_at
	) VALUES (?, ?, ?, ?, X'', ?, ?, ?, ?);`

	pinnedInt := 0
	if p.Pinned {
		pinnedInt = 1
	}

	_, err := r.db.ExecContext(ctx, query,
		p.ID,
		p.SubscriptionID,
		p.FetchID,
		p.ContentDigest,
		p.HTTPStatus,
		p.HeadersJSON,
		pinnedInt,
		p.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to insert subscription payload: %w", err)
	}
	return nil
}

func (r *subscriptionPayloadRepository) GetByID(ctx context.Context, id string) (*domain.SubscriptionPayload, error) {
	query := `SELECT id, subscription_id, fetch_id, content_digest, X'' AS body_blob, http_status, headers_json, pinned, created_at FROM subscription_payloads WHERE id = ?`
	args := []any{id}
	var evidenceTable int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='subscription_payload_evidence'`).Scan(&evidenceTable); err != nil {
		return nil, err
	}
	if evidenceTable != 0 {
		query += ` UNION ALL SELECT payload_id,COALESCE(subscription_id,''),fetch_id,content_digest,X'',http_status,'{}',pinned,original_created_at FROM subscription_payload_evidence WHERE payload_id=? AND NOT EXISTS(SELECT 1 FROM subscription_payloads WHERE id=?)`
		args = append(args, id, id)
	}

	var p domain.SubscriptionPayload
	var pinnedInt int
	var createdStr string

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&p.ID,
		&p.SubscriptionID,
		&p.FetchID,
		&p.ContentDigest,
		&p.BodyBlob,
		&p.HTTPStatus,
		&p.HeadersJSON,
		&pinnedInt,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("payload_not_found", fmt.Sprintf("payload %s not found", id))
		}
		return nil, fmt.Errorf("failed to query payload %s: %w", id, err)
	}

	if p.BodyBlob == nil {
		p.BodyBlob = []byte{}
	}
	p.Pinned = pinnedInt == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &p, nil
}

func (r *subscriptionPayloadRepository) GetLatestBySubscription(ctx context.Context, subscriptionID string) (*domain.SubscriptionPayload, error) {
	const query = `
	SELECT id, subscription_id, fetch_id, content_digest, X'' AS body_blob, http_status, headers_json, pinned, created_at
	FROM subscription_payloads WHERE subscription_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1;`

	var p domain.SubscriptionPayload
	var pinnedInt int
	var createdStr string

	err := r.db.QueryRowContext(ctx, query, subscriptionID).Scan(
		&p.ID,
		&p.SubscriptionID,
		&p.FetchID,
		&p.ContentDigest,
		&p.BodyBlob,
		&p.HTTPStatus,
		&p.HeadersJSON,
		&pinnedInt,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("payload_not_found", "no payload found for subscription")
		}
		return nil, fmt.Errorf("failed to query latest payload for subscription %s: %w", subscriptionID, err)
	}

	if p.BodyBlob == nil {
		p.BodyBlob = []byte{}
	}
	p.Pinned = pinnedInt == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &p, nil
}

func (r *subscriptionPayloadRepository) GetByDigest(ctx context.Context, contentDigest string) (*domain.SubscriptionPayload, error) {
	const query = `
	SELECT id, subscription_id, fetch_id, content_digest, X'' AS body_blob, http_status, headers_json, pinned, created_at
	FROM subscription_payloads WHERE content_digest = ? ORDER BY created_at DESC, rowid DESC LIMIT 1;`

	var p domain.SubscriptionPayload
	var pinnedInt int
	var createdStr string

	err := r.db.QueryRowContext(ctx, query, contentDigest).Scan(
		&p.ID,
		&p.SubscriptionID,
		&p.FetchID,
		&p.ContentDigest,
		&p.BodyBlob,
		&p.HTTPStatus,
		&p.HeadersJSON,
		&pinnedInt,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("payload_not_found", "payload not found by digest")
		}
		return nil, fmt.Errorf("failed to query payload by digest: %w", err)
	}

	if p.BodyBlob == nil {
		p.BodyBlob = []byte{}
	}
	p.Pinned = pinnedInt == 1
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &p, nil
}

func (r *subscriptionPayloadRepository) Pin(ctx context.Context, id string, pinned bool) error {
	pinnedInt := 0
	if pinned {
		pinnedInt = 1
	}
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO subscription_payload_evidence (payload_id,subscription_id,source_identity,fetch_id,content_digest,original_created_at,captured_at,http_status,evidence_kind,quality,pinned)
		SELECT id,subscription_id,'sub:'||subscription_id,fetch_id,content_digest,created_at,?,http_status,'pin','observed',pinned FROM subscription_payloads WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE subscription_payload_evidence SET pinned=? WHERE payload_id=?`, pinnedInt, id)
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			return domain.NewNotFoundError("payload_not_found", "payload evidence not found")
		}
		_, err = tx.ExecContext(ctx, `UPDATE subscription_payloads SET pinned=? WHERE id=?`, pinnedInt, id)
		return err
	})
}

func (r *subscriptionPayloadRepository) PruneUnreferenced(ctx context.Context, olderThan time.Time) (int64, error) {
	if err := CheckSubscriptionStorageReady(ctx, r.db); err != nil {
		return 0, err
	}
	// 1. Release expired preview drafts and failed previews so they don't permanently pin payloads:
	//    - Failed previews (token_hash LIKE 'draft_failed:%') are immediately released
	//    - Stale preview drafts older than 24 hours (or olderThan if olderThan is further in the past) are released
	//    - Active/published publications are never released
	draftCutoff := time.Now().Add(-24 * time.Hour)
	if olderThan.Before(draftCutoff) {
		draftCutoff = olderThan
	}
	draftCutoffStr := draftCutoff.Format(time.RFC3339)
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM publication_payload_refs
		WHERE publication_id IN (
			SELECT id FROM publications
			WHERE token_hash LIKE 'draft_failed:%'
			   OR (state = 'draft' AND created_at < ?)
		);`, draftCutoffStr); err != nil {
		return 0, fmt.Errorf("failed to release expired draft publication payload refs: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM publications
		WHERE token_hash LIKE 'draft_failed:%'
		   OR (state = 'draft' AND created_at < ?);`, draftCutoffStr); err != nil {
		return 0, fmt.Errorf("failed to release expired draft publications: %w", err)
	}

	// 2. Canonical safe retention algorithm protects newest 2 success payloads, newest any payload,
	// pins, publication refs, connection-version source entries, and current node source fetches.
	res, err := PruneSubscriptionRetention(ctx, r.db, "", 0, &olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to prune unreferenced payloads: %w", err)
	}
	return res.DeletedPayloads, nil
}

// --- Subscription Entry Repository ---

type subscriptionEntryRepository struct {
	db *sql.DB
}

func NewSubscriptionEntryRepository(db *sql.DB) domain.SubscriptionEntryRepository {
	return &subscriptionEntryRepository{db: db}
}

func (r *subscriptionEntryRepository) SaveBatch(ctx context.Context, entries []domain.SubscriptionEntry) error {
	if len(entries) == 0 {
		return nil
	}

	const query = `
	INSERT INTO subscription_entries (
		id, payload_id, subscription_id, ordinal, source_key, raw_name,
		protocol, server, port, entry_kind, classification_reason,
		classification_version, source_provenance_json, user_kind_override,
		override_anchor, override_reason, override_at, actor_ref,
		parsed_config_json, parser_version, warnings_json, node_logical_id, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(payload_id, ordinal) DO UPDATE SET
		source_key = excluded.source_key,
		raw_name = excluded.raw_name,
		protocol = excluded.protocol,
		server = excluded.server,
		port = excluded.port,
		entry_kind = excluded.entry_kind,
		classification_reason = excluded.classification_reason,
		classification_version = excluded.classification_version,
		source_provenance_json = excluded.source_provenance_json,
		parsed_config_json = excluded.parsed_config_json,
		warnings_json = excluded.warnings_json,
		node_logical_id = excluded.node_logical_id;`

	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to prepare entry insert statement: %w", err)
		}
		defer stmt.Close()

		for _, e := range entries {
			id := e.ID
			if id == "" {
				id = "entry_" + domain.MustNewUUIDv7()
			}
			nowStr := domain.NowUTC().Format(time.RFC3339)
			createdStr := e.CreatedAt.Format(time.RFC3339)
			if e.CreatedAt.IsZero() {
				createdStr = nowStr
			}

			var userOverrideVal sql.NullString
			if e.UserKindOverride != nil {
				userOverrideVal = sql.NullString{String: string(*e.UserKindOverride), Valid: true}
			}
			var overrideAtVal sql.NullString
			if e.OverrideAt != nil {
				overrideAtVal = sql.NullString{String: e.OverrideAt.Format(time.RFC3339), Valid: true}
			}
			var nodeLogicalIDVal sql.NullString
			if e.NodeLogicalID != nil && *e.NodeLogicalID != "" {
				nodeLogicalIDVal = sql.NullString{String: *e.NodeLogicalID, Valid: true}
			}

			provenanceJSON := e.SourceProvenanceJSON
			if provenanceJSON == "" {
				provenanceJSON = "{}"
			}
			parsedConfig := e.ParsedConfigJSON
			if parsedConfig == "" {
				parsedConfig = "{}"
			}
			warningsJSON := e.WarningsJSON
			if warningsJSON == "" {
				warningsJSON = "[]"
			}
			parserVer := e.ParserVersion
			if parserVer == "" {
				parserVer = "1.0.0"
			}
			classVer := e.ClassificationVersion
			if classVer == "" {
				classVer = "v2-proven-combo"
			}

			_, err = stmt.ExecContext(ctx,
				id,
				e.PayloadID,
				e.SubscriptionID,
				e.Ordinal,
				e.SourceKey,
				e.RawName,
				string(e.Protocol),
				e.Server,
				e.Port,
				string(e.EntryKind),
				e.ClassificationReason,
				classVer,
				provenanceJSON,
				userOverrideVal,
				e.OverrideAnchor,
				e.OverrideReason,
				overrideAtVal,
				e.ActorRef,
				parsedConfig,
				parserVer,
				warningsJSON,
				nodeLogicalIDVal,
				createdStr,
			)
			if err != nil {
				return fmt.Errorf("failed to insert entry (ordinal %d): %w", e.Ordinal, err)
			}
		}
		return nil
	})
}

func scanEntry(rows interface{ Scan(...any) error }) (*domain.SubscriptionEntry, error) {
	var e domain.SubscriptionEntry
	var protoStr, kindStr string
	var userOverride, overrideAt, nodeLogicalID sql.NullString
	var createdStr string

	err := rows.Scan(
		&e.ID,
		&e.PayloadID,
		&e.SubscriptionID,
		&e.Ordinal,
		&e.SourceKey,
		&e.RawName,
		&protoStr,
		&e.Server,
		&e.Port,
		&kindStr,
		&e.ClassificationReason,
		&e.ClassificationVersion,
		&e.SourceProvenanceJSON,
		&userOverride,
		&e.OverrideAnchor,
		&e.OverrideReason,
		&overrideAt,
		&e.ActorRef,
		&e.ParsedConfigJSON,
		&e.ParserVersion,
		&e.WarningsJSON,
		&nodeLogicalID,
		&createdStr,
	)
	if err != nil {
		return nil, err
	}

	e.Protocol = domain.Protocol(protoStr)
	e.EntryKind = domain.EntryKind(kindStr)
	if userOverride.Valid {
		k := domain.EntryKind(userOverride.String)
		e.UserKindOverride = &k
	}
	if overrideAt.Valid {
		t, _ := time.Parse(time.RFC3339, overrideAt.String)
		e.OverrideAt = &t
	}
	if nodeLogicalID.Valid {
		e.NodeLogicalID = &nodeLogicalID.String
	}
	e.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &e, nil
}

const entryColumns = `id, payload_id, subscription_id, ordinal, source_key, raw_name,
	protocol, server, port, entry_kind, classification_reason,
	classification_version, source_provenance_json, user_kind_override,
	override_anchor, override_reason, override_at, actor_ref,
	parsed_config_json, parser_version, warnings_json, node_logical_id, created_at`

func (r *subscriptionEntryRepository) ListByPayload(ctx context.Context, payloadID string) ([]domain.SubscriptionEntry, error) {
	query := "SELECT " + entryColumns + " FROM subscription_entries WHERE payload_id = ? ORDER BY ordinal ASC;"
	rows, err := r.db.QueryContext(ctx, query, payloadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query entries by payload: %w", err)
	}
	defer rows.Close()

	var entries []domain.SubscriptionEntry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, *e)
	}
	return entries, nil
}

func (r *subscriptionEntryRepository) ListBySubscription(ctx context.Context, subscriptionID string, limit, offset int) ([]domain.SubscriptionEntry, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM subscription_entries WHERE subscription_id = ?;", subscriptionID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count entries: %w", err)
	}

	query := "SELECT " + entryColumns + " FROM subscription_entries WHERE subscription_id = ? ORDER BY created_at DESC, ordinal ASC LIMIT ? OFFSET ?;"
	rows, err := r.db.QueryContext(ctx, query, subscriptionID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query entries: %w", err)
	}
	defer rows.Close()

	var entries []domain.SubscriptionEntry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan entry: %w", err)
		}
		entries = append(entries, *e)
	}
	return entries, total, nil
}

func (r *subscriptionEntryRepository) ListLatestBySubscription(ctx context.Context, subscriptionID string) ([]domain.SubscriptionEntry, error) {
	const payloadQuery = `
	SELECT id FROM subscription_payloads
	WHERE subscription_id = ?
	ORDER BY created_at DESC, rowid DESC LIMIT 1;`

	var payloadID string
	err := r.db.QueryRowContext(ctx, payloadQuery, subscriptionID).Scan(&payloadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query latest payload for subscription %s: %w", subscriptionID, err)
	}

	return r.ListByPayload(ctx, payloadID)
}

func (r *subscriptionEntryRepository) GetByID(ctx context.Context, id string) (*domain.SubscriptionEntry, error) {
	query := "SELECT " + entryColumns + " FROM subscription_entries WHERE id = ?;"
	row := r.db.QueryRowContext(ctx, query, id)
	e, err := scanEntry(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("entry_not_found", fmt.Sprintf("entry %s not found", id))
		}
		return nil, fmt.Errorf("failed to query entry: %w", err)
	}
	return e, nil
}

func (r *subscriptionEntryRepository) SetUserOverride(
	ctx context.Context,
	entryID string,
	userKindOverride *domain.EntryKind,
	overrideAnchor, reason, actorRef string,
	overrideAt time.Time,
) error {
	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		var subID, sourceKey, rawName, currentAnchor, proto, server string
		var port int
		const lookupQuery = `
		SELECT subscription_id, source_key, raw_name, override_anchor, protocol, server, port
		FROM subscription_entries
		WHERE id = ?;`

		err := tx.QueryRowContext(ctx, lookupQuery, entryID).Scan(
			&subID, &sourceKey, &rawName, &currentAnchor, &proto, &server, &port,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NewNotFoundError("entry_not_found", fmt.Sprintf("entry %s not found", entryID))
			}
			return fmt.Errorf("failed to lookup entry %s: %w", entryID, err)
		}

		anchor := overrideAnchor
		if anchor == "" {
			if currentAnchor != "" {
				anchor = currentAnchor
			} else if sourceKey != "" {
				anchor = sourceKey
			} else {
				anchor = fmt.Sprintf("%s|%s|%d|%s", proto, server, port, rawName)
			}
		}

		if overrideAt.IsZero() {
			overrideAt = domain.NowUTC()
		}
		atStr := overrideAt.Format(time.RFC3339)
		canonAnchor := fmt.Sprintf("%s|%s|%d|%s", proto, server, port, rawName)

		if userKindOverride != nil {
			const upsertLedger = `
			INSERT INTO subscription_entry_overrides (
				subscription_id, override_anchor, source_key, raw_name,
				user_kind_override, override_reason, override_at, actor_ref
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(subscription_id, override_anchor) DO UPDATE SET
				source_key = excluded.source_key,
				raw_name = excluded.raw_name,
				user_kind_override = excluded.user_kind_override,
				override_reason = excluded.override_reason,
				override_at = excluded.override_at,
				actor_ref = excluded.actor_ref;`

			if _, err := tx.ExecContext(ctx, upsertLedger,
				subID, anchor, sourceKey, rawName, string(*userKindOverride), reason, atStr, actorRef,
			); err != nil {
				return fmt.Errorf("failed to upsert override ledger: %w", err)
			}

			const updateProj = `
			UPDATE subscription_entries SET
				user_kind_override = ?,
				override_anchor = ?,
				override_reason = ?,
				override_at = ?,
				actor_ref = ?
			WHERE id = ?
			   OR (subscription_id = ? AND (
			       (override_anchor != '' AND override_anchor = ?)
			       OR (? != '' AND source_key = ?)
			       OR (protocol || '|' || server || '|' || port || '|' || raw_name) = ?
			   ));`

			if _, err := tx.ExecContext(ctx, updateProj,
				string(*userKindOverride), anchor, reason, atStr, actorRef,
				entryID, subID, anchor, sourceKey, sourceKey, canonAnchor,
			); err != nil {
				return fmt.Errorf("failed to update entry projection: %w", err)
			}
		} else {
			const deleteLedger = `
			DELETE FROM subscription_entry_overrides
			WHERE subscription_id = ? AND (override_anchor = ? OR (? != '' AND override_anchor = ?));`

			if _, err := tx.ExecContext(ctx, deleteLedger, subID, anchor, currentAnchor, currentAnchor); err != nil {
				return fmt.Errorf("failed to delete override ledger: %w", err)
			}

			const clearProj = `
			UPDATE subscription_entries SET
				user_kind_override = NULL,
				override_anchor = '',
				override_reason = '',
				override_at = NULL,
				actor_ref = ''
			WHERE id = ?
			   OR (subscription_id = ? AND (
			       (override_anchor != '' AND override_anchor = ?)
			       OR (? != '' AND source_key = ?)
			       OR (protocol || '|' || server || '|' || port || '|' || raw_name) = ?
			   ));`

			if _, err := tx.ExecContext(ctx, clearProj,
				entryID, subID, anchor, sourceKey, sourceKey, canonAnchor,
			); err != nil {
				return fmt.Errorf("failed to clear entry projection: %w", err)
			}
		}

		return nil
	})
}

// --- Node Connection Version & Heads Repository ---

type nodeConnectionVersionRepository struct {
	db *sql.DB
}

func NewNodeConnectionVersionRepository(db *sql.DB) domain.NodeConnectionVersionRepository {
	return &nodeConnectionVersionRepository{db: db}
}

func (r *nodeConnectionVersionRepository) Save(ctx context.Context, v *domain.NodeConnectionVersion) error {
	if v == nil {
		return domain.NewValidationError("missing_version", "node connection version is required")
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = domain.NowUTC()
	}
	if v.SchemaVersion <= 0 {
		v.SchemaVersion = 1
	}

	const query = `
	INSERT INTO node_connection_versions (
		node_logical_id, connection_revision, effective_config_json,
		config_fingerprint, source_entry_id, schema_version, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(node_logical_id, connection_revision) DO UPDATE SET
		source_entry_id = node_connection_versions.source_entry_id
	WHERE node_connection_versions.effective_config_json = excluded.effective_config_json
	  AND node_connection_versions.config_fingerprint = excluded.config_fingerprint;`

	var sourceEntryIDVal sql.NullString
	if v.SourceEntryID != nil && *v.SourceEntryID != "" {
		sourceEntryIDVal = sql.NullString{String: *v.SourceEntryID, Valid: true}
	}

	result, err := r.db.ExecContext(ctx, query,
		v.NodeLogicalID,
		v.ConnectionRevision,
		v.EffectiveConfigJSON,
		v.ConfigFingerprint,
		sourceEntryIDVal,
		v.SchemaVersion,
		v.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to save node connection version: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return domain.NewConflictError("connection_version_conflict", "a connection revision cannot be rewritten")
	}
	return nil
}

func (r *nodeConnectionVersionRepository) GetByRevision(ctx context.Context, nodeLogicalID string, revision int64) (*domain.NodeConnectionVersion, error) {
	const query = `
	SELECT node_logical_id, connection_revision, effective_config_json, config_fingerprint, source_entry_id, schema_version, created_at
	FROM node_connection_versions
	WHERE node_logical_id = ? AND connection_revision = ?;`

	var v domain.NodeConnectionVersion
	var sourceEntryID sql.NullString
	var createdStr string

	err := r.db.QueryRowContext(ctx, query, nodeLogicalID, revision).Scan(
		&v.NodeLogicalID,
		&v.ConnectionRevision,
		&v.EffectiveConfigJSON,
		&v.ConfigFingerprint,
		&sourceEntryID,
		&v.SchemaVersion,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("version_not_found", fmt.Sprintf("version %d not found for node %s", revision, nodeLogicalID))
		}
		return nil, fmt.Errorf("failed to query version: %w", err)
	}

	if sourceEntryID.Valid {
		v.SourceEntryID = &sourceEntryID.String
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &v, nil
}

func (r *nodeConnectionVersionRepository) GetHead(ctx context.Context, nodeLogicalID string) (*domain.NodeConnectionVersion, error) {
	const query = `
	SELECT v.node_logical_id, v.connection_revision, v.effective_config_json, v.config_fingerprint, v.source_entry_id, v.schema_version, v.created_at
	FROM node_connection_heads h
	INNER JOIN node_connection_versions v
	  ON h.logical_id = v.node_logical_id AND h.connection_revision = v.connection_revision
	WHERE h.logical_id = ?;`

	var v domain.NodeConnectionVersion
	var sourceEntryID sql.NullString
	var createdStr string

	err := r.db.QueryRowContext(ctx, query, nodeLogicalID).Scan(
		&v.NodeLogicalID,
		&v.ConnectionRevision,
		&v.EffectiveConfigJSON,
		&v.ConfigFingerprint,
		&sourceEntryID,
		&v.SchemaVersion,
		&createdStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.NewNotFoundError("head_not_found", fmt.Sprintf("head not found for node %s", nodeLogicalID))
		}
		return nil, fmt.Errorf("failed to query node head: %w", err)
	}

	if sourceEntryID.Valid {
		v.SourceEntryID = &sourceEntryID.String
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &v, nil
}

func (r *nodeConnectionVersionRepository) ListByNode(ctx context.Context, nodeLogicalID string) ([]domain.NodeConnectionVersion, error) {
	const query = `
	SELECT node_logical_id, connection_revision, effective_config_json, config_fingerprint, source_entry_id, schema_version, created_at
	FROM node_connection_versions
	WHERE node_logical_id = ?
	ORDER BY connection_revision ASC;`

	rows, err := r.db.QueryContext(ctx, query, nodeLogicalID)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}
	defer rows.Close()

	var versions []domain.NodeConnectionVersion
	for rows.Next() {
		var v domain.NodeConnectionVersion
		var sourceEntryID sql.NullString
		var createdStr string

		if err := rows.Scan(
			&v.NodeLogicalID,
			&v.ConnectionRevision,
			&v.EffectiveConfigJSON,
			&v.ConfigFingerprint,
			&sourceEntryID,
			&v.SchemaVersion,
			&createdStr,
		); err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}
		if sourceEntryID.Valid {
			v.SourceEntryID = &sourceEntryID.String
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		versions = append(versions, v)
	}
	return versions, nil
}

func (r *nodeConnectionVersionRepository) SetHead(ctx context.Context, nodeLogicalID string, revision int64, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = domain.NowUTC()
	}

	const query = `
	INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
	VALUES (?, ?, ?)
	ON CONFLICT(logical_id) DO UPDATE SET
		connection_revision = excluded.connection_revision,
		updated_at = excluded.updated_at;`

	_, err := r.db.ExecContext(ctx, query, nodeLogicalID, revision, updatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to set node head: %w", err)
	}
	return nil
}

// --- Node Overrides Repository ---

type nodeOverrideRepository struct {
	db *sql.DB
}

func NewNodeOverrideRepository(db *sql.DB) domain.NodeOverrideRepository {
	return &nodeOverrideRepository{db: db}
}

func (r *nodeOverrideRepository) Save(ctx context.Context, o *domain.NodeOverride) error {
	if o == nil {
		return domain.NewValidationError("missing_override", "node override is required")
	}
	nowStr := domain.NowUTC().Format(time.RFC3339)
	createdStr := o.CreatedAt.Format(time.RFC3339)
	if o.CreatedAt.IsZero() {
		createdStr = nowStr
	}

	const query = `
	INSERT INTO node_overrides (node_logical_id, field_path, override_value_json, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(node_logical_id, field_path) DO UPDATE SET
		override_value_json = excluded.override_value_json,
		updated_at = excluded.updated_at;`

	_, err := r.db.ExecContext(ctx, query, o.NodeLogicalID, o.FieldPath, o.OverrideValueJSON, createdStr, nowStr)
	if err != nil {
		return fmt.Errorf("failed to save node override: %w", err)
	}
	return nil
}

func (r *nodeOverrideRepository) GetByNode(ctx context.Context, nodeLogicalID string) ([]domain.NodeOverride, error) {
	const query = `
	SELECT node_logical_id, field_path, override_value_json, created_at, updated_at
	FROM node_overrides WHERE node_logical_id = ? ORDER BY field_path ASC;`

	rows, err := r.db.QueryContext(ctx, query, nodeLogicalID)
	if err != nil {
		return nil, fmt.Errorf("failed to query node overrides: %w", err)
	}
	defer rows.Close()

	var overrides []domain.NodeOverride
	for rows.Next() {
		var o domain.NodeOverride
		var cStr, uStr string
		if err := rows.Scan(&o.NodeLogicalID, &o.FieldPath, &o.OverrideValueJSON, &cStr, &uStr); err != nil {
			return nil, fmt.Errorf("failed to scan node override: %w", err)
		}
		o.CreatedAt, _ = time.Parse(time.RFC3339, cStr)
		o.UpdatedAt, _ = time.Parse(time.RFC3339, uStr)
		overrides = append(overrides, o)
	}
	return overrides, nil
}

func (r *nodeOverrideRepository) GetAll(ctx context.Context) (map[string][]domain.NodeOverride, error) {
	const query = `
	SELECT node_logical_id, field_path, override_value_json, created_at, updated_at
	FROM node_overrides ORDER BY node_logical_id ASC, field_path ASC;`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all node overrides: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]domain.NodeOverride)
	for rows.Next() {
		var o domain.NodeOverride
		var cStr, uStr string
		if err := rows.Scan(&o.NodeLogicalID, &o.FieldPath, &o.OverrideValueJSON, &cStr, &uStr); err != nil {
			return nil, fmt.Errorf("failed to scan node override: %w", err)
		}
		o.CreatedAt, _ = time.Parse(time.RFC3339, cStr)
		o.UpdatedAt, _ = time.Parse(time.RFC3339, uStr)
		result[o.NodeLogicalID] = append(result[o.NodeLogicalID], o)
	}
	return result, nil
}

func (r *nodeOverrideRepository) Delete(ctx context.Context, nodeLogicalID, fieldPath string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM node_overrides WHERE node_logical_id = ? AND field_path = ?;", nodeLogicalID, fieldPath)
	if err != nil {
		return fmt.Errorf("failed to delete node override: %w", err)
	}
	return nil
}

// --- Publication Payload Ref Repository ---

type publicationPayloadRefRepository struct {
	db *sql.DB
}

func NewPublicationPayloadRefRepository(db *sql.DB) domain.PublicationPayloadRefRepository {
	return &publicationPayloadRefRepository{db: db}
}

func (r *publicationPayloadRefRepository) AddRefs(ctx context.Context, publicationID string, payloadIDs []string) error {
	if len(payloadIDs) == 0 {
		return nil
	}

	const query = `
	INSERT OR IGNORE INTO publication_payload_refs (publication_id, payload_id)
	VALUES (?, ?);`

	return WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		var evidenceTable int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='subscription_payload_evidence'`).Scan(&evidenceTable); err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to prepare payload ref statement: %w", err)
		}
		defer stmt.Close()

		for _, pID := range payloadIDs {
			if strings.TrimSpace(pID) == "" {
				continue
			}
			if evidenceTable != 0 {
				if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO subscription_payload_evidence (payload_id,subscription_id,source_identity,fetch_id,content_digest,original_created_at,captured_at,http_status,evidence_kind,quality,pinned)
				SELECT id,subscription_id,'sub:'||subscription_id,fetch_id,content_digest,created_at,?,http_status,'publication','observed',pinned FROM subscription_payloads WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), strings.TrimSpace(pID)); err != nil {
					return err
				}
			}
			if _, err := stmt.ExecContext(ctx, publicationID, strings.TrimSpace(pID)); err != nil {
				return fmt.Errorf("failed to insert publication payload ref (%s -> %s): %w", publicationID, pID, err)
			}
		}
		return nil
	})
}

func (r *publicationPayloadRefRepository) ListPayloadIDsByPublication(ctx context.Context, publicationID string) ([]string, error) {
	const query = `
	SELECT payload_id FROM publication_payload_refs WHERE publication_id = ? ORDER BY payload_id ASC;`

	rows, err := r.db.QueryContext(ctx, query, publicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query payload refs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan payload ref: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (r *publicationPayloadRefRepository) DeleteRefsByPublication(ctx context.Context, publicationID string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM publication_payload_refs WHERE publication_id = ?;", publicationID)
	return err
}

func (r *publicationPayloadRefRepository) ResolvePayloadIDsForNodes(ctx context.Context, nodes []domain.ManifestIncludedNode) ([]string, error) {
	if len(nodes) == 0 {
		return nil, nil
	}
	payloadMap := make(map[string]bool)
	var result []string

	const queryVersioned = `
	SELECT e.payload_id FROM node_connection_versions v
	JOIN subscription_entries e ON v.source_entry_id = e.id
	WHERE v.node_logical_id = ? AND v.connection_revision = ? AND e.payload_id IS NOT NULL AND e.payload_id != '';`

	for _, n := range nodes {
		var payloadID sql.NullString
		rows, err := r.db.QueryContext(ctx, `SELECT origin_payload_id FROM node_connection_provenance WHERE node_logical_id=? AND connection_revision=? ORDER BY origin_payload_id`, n.NodeID, n.ConnectionRevision)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, err
			}
			if !payloadMap[id] {
				payloadMap[id] = true
				result = append(result, id)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
		err = r.db.QueryRowContext(ctx, queryVersioned, n.NodeID, n.ConnectionRevision).Scan(&payloadID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if payloadID.Valid && strings.TrimSpace(payloadID.String) != "" {
			pid := strings.TrimSpace(payloadID.String)
			if !payloadMap[pid] {
				payloadMap[pid] = true
				result = append(result, pid)
			}
		}
	}
	return result, nil
}
