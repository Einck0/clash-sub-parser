package sqlite_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
	"clash-sub-parser/migrations"
)

func TestDNSSettingsRepository_LifecycleAndCAS(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to apply migrations: %v", err)
	}

	repo := sqlite.NewDNSSettingsRepository(db)

	// 1. Initial Get yields default configuration without DB rows
	initial, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("initial Get failed: %v", err)
	}
	if initial.Source != "default" {
		t.Fatalf("expected source default, got %q", initial.Source)
	}
	if initial.Revision != 0 {
		t.Fatalf("expected revision 0, got %d", initial.Revision)
	}
	if !initial.Enabled {
		t.Fatalf("expected default enabled to be true")
	}

	// 2. Reject initial update when expectedRevision != 0
	toUpdate := &domain.DNSSettings{
		Enabled: true,
		RawYAML: "enable: true\nnameserver:\n  - 1.1.1.1\n",
	}
	err = repo.Update(ctx, toUpdate, 1)
	if err == nil {
		t.Fatalf("expected revision mismatch error when updating empty table with revision 1")
	}
	domErr, ok := domain.AsDomainError(err)
	if !ok || domErr.Code != "dns_revision_conflict" {
		t.Fatalf("expected domain error dns_revision_conflict, got %v", err)
	}

	// 3. First update with expectedRevision == 0 seeds revision 1
	err = repo.Update(ctx, toUpdate, 0)
	if err != nil {
		t.Fatalf("expected initial update with expectedRevision 0 to succeed: %v", err)
	}
	if toUpdate.Revision != 1 {
		t.Fatalf("expected updated struct revision 1, got %d", toUpdate.Revision)
	}
	if toUpdate.Source != "saved" {
		t.Fatalf("expected updated struct source saved, got %q", toUpdate.Source)
	}

	// 4. Get after first save returns persisted config
	saved, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get after save failed: %v", err)
	}
	if saved.Source != "saved" {
		t.Fatalf("expected source saved, got %q", saved.Source)
	}
	if saved.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", saved.Revision)
	}

	// 5. Subsequent update increments revision to 2
	saved.RawYAML = "enable: true\nnameserver:\n  - 8.8.8.8\ncustom-flag: true\n"
	err = repo.Update(ctx, saved, 1)
	if err != nil {
		t.Fatalf("expected CAS update with expected revision 1 to succeed: %v", err)
	}
	if saved.Revision != 2 {
		t.Fatalf("expected revision incremented to 2, got %d", saved.Revision)
	}
	if !strings.Contains(saved.RawYAML, "custom-flag") {
		t.Fatalf("expected custom-flag preserved in normalized YAML")
	}

	// 6. CAS conflict when expected revision is stale
	staleUpdate := &domain.DNSSettings{
		Enabled: true,
		RawYAML: "enable: true\nnameserver:\n  - 9.9.9.9\n",
	}
	err = repo.Update(ctx, staleUpdate, 1)
	if err == nil {
		t.Fatalf("expected CAS conflict on stale expected revision 1")
	}
	domErr, ok = domain.AsDomainError(err)
	if !ok || domErr.Code != "dns_revision_conflict" {
		t.Fatalf("expected dns_revision_conflict, got %v", err)
	}
	if domErr.CurrentRevision == nil || *domErr.CurrentRevision != 2 {
		t.Fatalf("expected current revision in conflict details to be 2, got %#v", domErr.CurrentRevision)
	}
}

func TestDNSSettingsRepository_PersistenceAcrossReopen(t *testing.T) {
	ctx := context.Background()
	_, path := setupTempFileDB(t)

	first, err := sqlite.OpenAndMigrate(ctx, sqlite.DefaultConfig(path))
	if err != nil {
		t.Fatalf("failed to open test sqlite: %v", err)
	}
	firstRepo := sqlite.NewDNSSettingsRepository(first)

	raw := `
enable: false
nameserver:
  - 1.1.1.1
fake-ip-range: 198.18.0.1/16
`
	setting := &domain.DNSSettings{
		Enabled: false,
		RawYAML: raw,
	}
	if err := firstRepo.Update(ctx, setting, 0); err != nil {
		t.Fatalf("failed to save initial DNS settings: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("failed to close first db connection: %v", err)
	}

	// Reopen with new connection
	reopened, err := sqlite.OpenAndMigrate(ctx, sqlite.DefaultConfig(path))
	if err != nil {
		t.Fatalf("failed to reopen db: %v", err)
	}
	defer reopened.Close()

	reopenedRepo := sqlite.NewDNSSettingsRepository(reopened)
	got, err := reopenedRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get DNS settings after reopen: %v", err)
	}
	if got.Enabled {
		t.Fatalf("expected persisted enabled to be false")
	}
	if got.Source != "saved" {
		t.Fatalf("expected source saved, got %q", got.Source)
	}
	if got.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", got.Revision)
	}
	if !strings.Contains(got.RawYAML, "198.18.0.1/16") {
		t.Fatalf("expected fake-ip-range preserved across reopening: %s", got.RawYAML)
	}
}

func TestDNSSettingsRepository_ConcurrentInitialCAS(t *testing.T) {
	ctx := context.Background()
	_, path := setupTempFileDB(t)
	cfg := sqlite.DefaultConfig(path)
	cfg.BusyTimeout = 5 * time.Second
	secondDB, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("open second SQLite connection: %v", err)
	}
	defer secondDB.Close()

	repositories := []domain.DNSSettingsRepository{
		sqlite.NewDNSSettingsRepository(secondDB),
	}
	firstDB, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("open first SQLite connection: %v", err)
	}
	defer firstDB.Close()
	repositories = append(repositories, sqlite.NewDNSSettingsRepository(firstDB))

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, len(repositories))
	for _, repo := range repositories {
		wg.Add(1)
		go func(repo domain.DNSSettingsRepository) {
			defer wg.Done()
			<-start
			settings := &domain.DNSSettings{Enabled: true, RawYAML: "nameserver: [1.1.1.1]\n"}
			results <- repo.Update(ctx, settings, 0)
		}(repo)
	}
	close(start)
	wg.Wait()
	close(results)

	succeeded, conflicted := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		domainErr, ok := domain.AsDomainError(err)
		if !ok || domainErr.Code != "dns_revision_conflict" {
			t.Fatalf("expected a revision conflict, got %T: %v", err, err)
		}
		conflicted++
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent initial CAS: successes=%d conflicts=%d, want 1 each", succeeded, conflicted)
	}
}

func TestDNSSettingsRepository_ConcurrentCASOnExistingRevision(t *testing.T) {
	ctx := context.Background()
	_, path := setupTempFileDB(t)
	cfg := sqlite.DefaultConfig(path)
	cfg.BusyTimeout = 5 * time.Second

	firstDB, err := sqlite.OpenAndMigrate(ctx, cfg)
	if err != nil {
		t.Fatalf("open first SQLite connection: %v", err)
	}
	defer firstDB.Close()

	secondDB, err := sqlite.Open(cfg)
	if err != nil {
		t.Fatalf("open second SQLite connection: %v", err)
	}
	defer secondDB.Close()

	repo1 := sqlite.NewDNSSettingsRepository(firstDB)
	repo2 := sqlite.NewDNSSettingsRepository(secondDB)

	initial := &domain.DNSSettings{Enabled: true, RawYAML: "nameserver: [1.1.1.1]\n"}
	if err := repo1.Update(ctx, initial, 0); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	if initial.Revision != 1 {
		t.Fatalf("expected initial revision 1, got %d", initial.Revision)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		s := &domain.DNSSettings{Enabled: true, RawYAML: "nameserver: [8.8.8.8]\n"}
		results <- repo1.Update(ctx, s, 1)
	}()
	go func() {
		defer wg.Done()
		<-start
		s := &domain.DNSSettings{Enabled: false, RawYAML: "nameserver: [9.9.9.9]\n"}
		results <- repo2.Update(ctx, s, 1)
	}()

	close(start)
	wg.Wait()
	close(results)

	succeeded, conflicted := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		domainErr, ok := domain.AsDomainError(err)
		if !ok || domainErr.Code != "dns_revision_conflict" {
			t.Fatalf("expected revision conflict, got %T: %v", err, err)
		}
		if domainErr.CurrentRevision == nil || *domainErr.CurrentRevision != 2 {
			t.Fatalf("expected conflict current revision 2, got %v", domainErr.CurrentRevision)
		}
		conflicted++
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent CAS on existing revision: successes=%d conflicts=%d, want 1 each", succeeded, conflicted)
	}
}

func TestDNSSettingsRepository_SettingsIsolation(t *testing.T) {
	ctx := context.Background()
	db, _ := setupEmptyTestDB(t)

	runner := sqlite.NewMigrationRunner(db, migrations.FS)
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	settingsRepo := sqlite.NewSettingsRepository(db)
	dnsRepo := sqlite.NewDNSSettingsRepository(db)

	tokenHash := "admin-verifier-hash"
	if err := settingsRepo.UpdateAdminToken(ctx, tokenHash); err != nil {
		t.Fatalf("failed to update admin token: %v", err)
	}

	dns := &domain.DNSSettings{
		Enabled: true,
		RawYAML: "enable: true\nnameserver:\n  - 8.8.8.8\n",
	}
	if err := dnsRepo.Update(ctx, dns, 0); err != nil {
		t.Fatalf("failed to update DNS settings: %v", err)
	}

	st, err := settingsRepo.Get(ctx)
	if err != nil {
		t.Fatalf("failed to get system settings: %v", err)
	}
	if st.AdminToken != tokenHash {
		t.Fatalf("DNS operation leaked into settings table or modified admin token")
	}
}
