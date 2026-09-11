package main

import (
	"os"
	"testing"
)

func TestResolveDefaultDB(t *testing.T) {
	origCSP := os.Getenv("CSP_DB_PATH")
	origDB := os.Getenv("DB_PATH")
	origURL := os.Getenv("CLASH_DATABASE_URL")
	defer func() {
		_ = os.Setenv("CSP_DB_PATH", origCSP)
		_ = os.Setenv("DB_PATH", origDB)
		_ = os.Setenv("CLASH_DATABASE_URL", origURL)
	}()

	t.Run("default when all unset", func(t *testing.T) {
		_ = os.Unsetenv("CSP_DB_PATH")
		_ = os.Unsetenv("DB_PATH")
		_ = os.Unsetenv("CLASH_DATABASE_URL")
		if got := resolveDefaultDB(); got != "clash_sub_parser.db" {
			t.Errorf("expected clash_sub_parser.db, got: %s", got)
		}
	})

	t.Run("CSP_DB_PATH has highest priority", func(t *testing.T) {
		_ = os.Setenv("CSP_DB_PATH", "custom_csp.db")
		_ = os.Setenv("DB_PATH", "custom_db.db")
		_ = os.Setenv("CLASH_DATABASE_URL", "sqlite+aiosqlite:///url.db")
		if got := resolveDefaultDB(); got != "custom_csp.db" {
			t.Errorf("expected custom_csp.db, got: %s", got)
		}
	})

	t.Run("DB_PATH fallback", func(t *testing.T) {
		_ = os.Unsetenv("CSP_DB_PATH")
		_ = os.Setenv("DB_PATH", "custom_db.db")
		_ = os.Setenv("CLASH_DATABASE_URL", "sqlite+aiosqlite:///url.db")
		if got := resolveDefaultDB(); got != "custom_db.db" {
			t.Errorf("expected custom_db.db, got: %s", got)
		}
	})

	t.Run("CLASH_DATABASE_URL sqlite+aiosqlite prefix strip", func(t *testing.T) {
		_ = os.Unsetenv("CSP_DB_PATH")
		_ = os.Unsetenv("DB_PATH")
		_ = os.Setenv("CLASH_DATABASE_URL", "sqlite+aiosqlite:////data/clash_sub_parser.db")
		if got := resolveDefaultDB(); got != "/data/clash_sub_parser.db" {
			t.Errorf("expected /data/clash_sub_parser.db, got: %s", got)
		}
	})

	t.Run("CLASH_DATABASE_URL sqlite prefix strip", func(t *testing.T) {
		_ = os.Unsetenv("CSP_DB_PATH")
		_ = os.Unsetenv("DB_PATH")
		_ = os.Setenv("CLASH_DATABASE_URL", "sqlite:///test.db")
		if got := resolveDefaultDB(); got != "test.db" {
			t.Errorf("expected test.db, got: %s", got)
		}
	})
}
