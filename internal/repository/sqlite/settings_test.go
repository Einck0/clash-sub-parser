package sqlite_test

import (
	"context"
	"database/sql"
	"testing"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestSettingsRepository_AuthSettings(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	// Run all migrations including 000013_auth_settings.sql
	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	assertAuthSchemaDefaults(t, db)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("reapplying migrations must be idempotent: %v", err)
	}

	repo := sqlite.NewSettingsRepository(db)

	// 1. Initial seeded settings: both admin and export auth default to true (1)
	st, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get initial settings: %v", err)
	}
	if !st.AdminAuthEnabled {
		t.Fatalf("expected admin_auth_enabled default to be true")
	}
	if !st.ExportAuthEnabled {
		t.Fatalf("expected export_auth_enabled default to be true")
	}
	if st.AdminToken != "" {
		t.Fatalf("expected initial admin_token to be empty")
	}

	// 2. Update via standard Update() modifying switches
	st.AdminAuthEnabled = false
	st.ExportAuthEnabled = true
	st.AdminToken = "test-token-hash"
	if err := repo.Update(ctx, st); err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	st2, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get updated settings: %v", err)
	}
	if st2.AdminAuthEnabled {
		t.Fatalf("expected admin_auth_enabled to be false")
	}
	if !st2.ExportAuthEnabled {
		t.Fatalf("expected export_auth_enabled to be true")
	}
	if st2.AdminToken != "test-token-hash" {
		t.Fatalf("expected admin_token to be 'test-token-hash', got %q", st2.AdminToken)
	}

	// 3. UpdateAdminToken preserves existing switch values
	if err := repo.UpdateAdminToken(ctx, "new-token-hash"); err != nil {
		t.Fatalf("failed to update admin token: %v", err)
	}
	st3, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get settings after UpdateAdminToken: %v", err)
	}
	if st3.AdminToken != "new-token-hash" {
		t.Fatalf("expected admin_token 'new-token-hash', got %q", st3.AdminToken)
	}
	if st3.AdminAuthEnabled {
		t.Fatalf("expected admin_auth_enabled to remain false")
	}
	if !st3.ExportAuthEnabled {
		t.Fatalf("expected export_auth_enabled to remain true")
	}

	// 4. UpdateAuthSettings without token change
	if err := repo.UpdateAuthSettings(ctx, true, false, nil); err != nil {
		t.Fatalf("failed to update auth settings without token: %v", err)
	}
	st4, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get settings: %v", err)
	}
	if !st4.AdminAuthEnabled {
		t.Fatalf("expected admin_auth_enabled to be true")
	}
	if st4.ExportAuthEnabled {
		t.Fatalf("expected export_auth_enabled to be false")
	}
	if st4.AdminToken != "new-token-hash" {
		t.Fatalf("expected admin_token to remain unchanged, got %q", st4.AdminToken)
	}

	// 5. UpdateAuthSettings with new token
	newTok := "another-hash"
	if err := repo.UpdateAuthSettings(ctx, true, true, &newTok); err != nil {
		t.Fatalf("failed to update auth settings with token: %v", err)
	}
	st5, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get settings: %v", err)
	}
	if !st5.AdminAuthEnabled || !st5.ExportAuthEnabled {
		t.Fatalf("expected both switches to be true")
	}
	if st5.AdminToken != "another-hash" {
		t.Fatalf("expected admin_token to be updated to 'another-hash', got %q", st5.AdminToken)
	}

	// 6. UpdateAuthSettings with cleared token
	emptyTok := ""
	if err := repo.UpdateAuthSettings(ctx, false, false, &emptyTok); err != nil {
		t.Fatalf("failed to clear token via UpdateAuthSettings: %v", err)
	}
	st6, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get settings: %v", err)
	}
	if st6.AdminAuthEnabled || st6.ExportAuthEnabled {
		t.Fatalf("expected both switches to be false")
	}
	if st6.AdminToken != "" {
		t.Fatalf("expected admin_token to be empty, got %q", st6.AdminToken)
	}
}

func assertAuthSchemaDefaults(t *testing.T, db *sql.DB) {
	t.Helper()
	var admin, export int
	if err := db.QueryRow(`SELECT admin_auth_enabled, export_auth_enabled FROM settings WHERE id = 1`).Scan(&admin, &export); err != nil {
		t.Fatal(err)
	}
	if admin != 1 || export != 1 {
		t.Fatalf("migration 13 must seed both protection switches on: %d, %d", admin, export)
	}
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 14 {
		t.Fatalf("expected schema version 14, got %d: %v", version, err)
	}
}

func TestAuthSettingsPersistAcrossSQLiteReopen(t *testing.T) {
	ctx := context.Background()
	_, path := setupTempFileDB(t)
	// Use a separate connection to model a fresh process reading the same file.
	first, err := sqlite.OpenAndMigrate(ctx, sqlite.DefaultConfig(path))
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewSettingsRepository(first)
	verifier := "stored-verifier"
	if err := repo.UpdateAuthSettings(ctx, false, true, &verifier); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.OpenAndMigrate(ctx, sqlite.DefaultConfig(path))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := sqlite.NewSettingsRepository(reopened).Get(ctx)
	if err != nil || got.AdminAuthEnabled || !got.ExportAuthEnabled || got.AdminToken != verifier {
		t.Fatalf("auth settings lost after reopening: %+v, %v", got, err)
	}
}

func TestSettingsRepository_DefaultValues(t *testing.T) {
	def := domain.DefaultSettings()
	if !def.AdminAuthEnabled {
		t.Errorf("expected DefaultSettings().AdminAuthEnabled to be true")
	}
	if !def.ExportAuthEnabled {
		t.Errorf("expected DefaultSettings().ExportAuthEnabled to be true")
	}
}
