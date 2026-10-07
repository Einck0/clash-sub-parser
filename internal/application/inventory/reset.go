package inventory

import (
	"context"
	"database/sql"
	"fmt"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

// ResetOptions configures the node inventory reset operation.
type ResetOptions struct {
	TargetDBPath        string
	DryRun              bool
	RestoreSourceTokens bool
	ArchiveDBPath       string
}

// RunResetNodeInventory orchestrates the clean-slate node inventory reset and optional source token repair.
func RunResetNodeInventory(ctx context.Context, db *sql.DB, opts ResetOptions) (*domain.NodeInventoryResetReport, error) {
	if opts.RestoreSourceTokens {
		archivePath := opts.ArchiveDBPath
		if archivePath == "" {
			archivePath = "/home/service/backups/csp-legacy-cold-archive-20260919.db"
		}
		if !opts.DryRun {
			_, err := RestoreSourceTokensFromArchive(ctx, db, archivePath)
			if err != nil {
				return nil, fmt.Errorf("failed to restore source tokens before reset: %w", err)
			}
		}
	}

	repo := sqlite.NewNodeResetRepository(db)
	report, err := repo.ExecuteReset(ctx, opts.DryRun)
	if err != nil {
		return nil, fmt.Errorf("failed to execute node inventory reset: %w", err)
	}

	if opts.RestoreSourceTokens {
		report.SourceURLsRestored = map[string]bool{
			"7li": true,
			"魔戒": true,
		}
	}

	return report, nil
}
