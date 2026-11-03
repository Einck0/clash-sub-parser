package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"clash-sub-parser/internal/domain"
)

type proxyChainRepository struct {
	db *sql.DB
}

// NewProxyChainRepository creates a SQLite repository for atomic proxy chain binding operations.
func NewProxyChainRepository(db *sql.DB) domain.ProxyChainRepository {
	return &proxyChainRepository{db: db}
}

func (r *proxyChainRepository) Get(ctx context.Context) (*domain.ProxyChainConfig, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, target_type, target_id, dialer_type, dialer_id, enabled, sort_order, note, revision
		FROM proxy_chain_bindings
		ORDER BY sort_order ASC, id ASC;
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query proxy_chain_bindings: %w", err)
	}
	defer rows.Close()

	bindings := make([]domain.ProxyChainBinding, 0)
	for rows.Next() {
		var b domain.ProxyChainBinding
		var enabledInt int
		var targetType, dialerType string
		if err := rows.Scan(
			&b.ID,
			&targetType,
			&b.TargetID,
			&dialerType,
			&b.DialerID,
			&enabledInt,
			&b.SortOrder,
			&b.Note,
			&b.Revision,
		); err != nil {
			return nil, fmt.Errorf("failed to scan proxy_chain_binding row: %w", err)
		}
		b.TargetType = domain.ProxyChainTargetType(targetType)
		b.DialerType = domain.ProxyChainDialerType(dialerType)
		b.Enabled = enabledInt == 1
		bindings = append(bindings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate proxy_chain_bindings: %w", err)
	}

	version := domain.ProxyChainConfigVersion(bindings)
	return &domain.ProxyChainConfig{
		Bindings: bindings,
		Version:  version,
	}, nil
}

func (r *proxyChainRepository) Replace(ctx context.Context, expectedVersion string, bindings []domain.ProxyChainBinding) (*domain.ProxyChainConfig, error) {
	// Validate all candidate bindings before acquiring transaction
	seenIDs := make(map[string]struct{}, len(bindings))
	for _, b := range bindings {
		if err := b.Validate(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(b.ID) == "" {
			return nil, domain.NewValidationError("invalid_chain_binding", "binding id cannot be empty")
		}
		if _, exists := seenIDs[b.ID]; exists {
			return nil, domain.NewValidationError("invalid_chain_binding", fmt.Sprintf("duplicate binding id: %s", b.ID))
		}
		seenIDs[b.ID] = struct{}{}
	}

	var result *domain.ProxyChainConfig
	err := WithTx(ctx, r.db, func(ctx context.Context, tx *sql.Tx) error {
		// 1. Read current bindings to check expected version
		rows, err := tx.QueryContext(ctx, `
			SELECT id, target_type, target_id, dialer_type, dialer_id, enabled, sort_order, note, revision
			FROM proxy_chain_bindings
			ORDER BY sort_order ASC, id ASC;
		`)
		if err != nil {
			return fmt.Errorf("failed to query current proxy_chain_bindings in tx: %w", err)
		}
		defer rows.Close()

		currentBindings := make([]domain.ProxyChainBinding, 0)
		for rows.Next() {
			var b domain.ProxyChainBinding
			var enabledInt int
			var targetType, dialerType string
			if err := rows.Scan(
				&b.ID,
				&targetType,
				&b.TargetID,
				&dialerType,
				&b.DialerID,
				&enabledInt,
				&b.SortOrder,
				&b.Note,
				&b.Revision,
			); err != nil {
				return fmt.Errorf("failed to scan current proxy_chain_binding: %w", err)
			}
			b.TargetType = domain.ProxyChainTargetType(targetType)
			b.DialerType = domain.ProxyChainDialerType(dialerType)
			b.Enabled = enabledInt == 1
			currentBindings = append(currentBindings, b)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("failed to iterate current proxy_chain_bindings: %w", err)
		}
		rows.Close()

		currentVersion := domain.ProxyChainConfigVersion(currentBindings)
		if expectedVersion != "" && currentVersion != expectedVersion {
			return domain.NewConflictError("proxy_chain_conflict", fmt.Sprintf("proxy chain config version mismatch: expected %s, got %s", expectedVersion, currentVersion))
		}

		// 2. Clear existing bindings
		if _, err := tx.ExecContext(ctx, `DELETE FROM proxy_chain_bindings;`); err != nil {
			return fmt.Errorf("failed to clear proxy_chain_bindings: %w", err)
		}

		// 3. Insert new bindings
		if len(bindings) > 0 {
			stmt, err := tx.PrepareContext(ctx, `
				INSERT INTO proxy_chain_bindings (id, target_type, target_id, dialer_type, dialer_id, enabled, sort_order, note, revision, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
			`)
			if err != nil {
				return fmt.Errorf("failed to prepare insert statement: %w", err)
			}
			defer stmt.Close()

			now := domain.NowUTC()
			for _, b := range bindings {
				enabledInt := 0
				if b.Enabled {
					enabledInt = 1
				}
				rev := b.Revision
				if rev < 1 {
					rev = 1
				}
				if _, err := stmt.ExecContext(ctx,
					b.ID,
					string(b.TargetType),
					b.TargetID,
					string(b.DialerType),
					b.DialerID,
					enabledInt,
					b.SortOrder,
					b.Note,
					rev,
					now,
				); err != nil {
					return fmt.Errorf("failed to insert proxy_chain_binding %s: %w", b.ID, err)
				}
			}
		}

		// Sort canonical return bindings by sort_order, id
		storedBindings := make([]domain.ProxyChainBinding, len(bindings))
		copy(storedBindings, bindings)
		sort.SliceStable(storedBindings, func(i, j int) bool {
			if storedBindings[i].SortOrder != storedBindings[j].SortOrder {
				return storedBindings[i].SortOrder < storedBindings[j].SortOrder
			}
			return storedBindings[i].ID < storedBindings[j].ID
		})

		newVersion := domain.ProxyChainConfigVersion(storedBindings)
		result = &domain.ProxyChainConfig{
			Bindings: storedBindings,
			Version:  newVersion,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
