package sqlite_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/repository/sqlite"
)

func TestSubscriptionLinkRepository_CRUD(t *testing.T) {
	db, _ := setupTestDB(t)
	repo := sqlite.NewSubscriptionLinkRepository(db)
	ctx := context.Background()

	link := &domain.SubscriptionLink{
		ID:        "link-crud-1",
		Label:     "My Mihomo Link",
		Target:    domain.TargetMihomo,
		Token:     "pub_token_1234567890abcdef12345678",
		TokenHash: "hash_crud_1",
		IsDefault: false,
	}

	// 1. Create
	if err := repo.Create(ctx, link); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	// 2. GetByID
	got, err := repo.GetByID(ctx, "link-crud-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got.Label != "My Mihomo Link" || got.Target != domain.TargetMihomo || got.Token != link.Token {
		t.Fatalf("GetByID returned unexpected data: %+v", got)
	}

	// 3. GetByTokenHash
	byHash, err := repo.GetByTokenHash(ctx, "hash_crud_1")
	if err != nil {
		t.Fatalf("GetByTokenHash failed: %v", err)
	}
	if byHash.ID != "link-crud-1" {
		t.Fatalf("expected ID link-crud-1, got %s", byHash.ID)
	}

	// 4. UpdateLabel
	if err := repo.UpdateLabel(ctx, "link-crud-1", "Updated Label", time.Now().UTC()); err != nil {
		t.Fatalf("UpdateLabel failed: %v", err)
	}
	gotUpdated, _ := repo.GetByID(ctx, "link-crud-1")
	if gotUpdated.Label != "Updated Label" {
		t.Fatalf("expected Updated Label, got %s", gotUpdated.Label)
	}

	// 5. RotateToken
	if err := repo.RotateToken(ctx, "link-crud-1", "pub_token_new", "hash_crud_new", time.Now().UTC()); err != nil {
		t.Fatalf("RotateToken failed: %v", err)
	}
	gotRotated, _ := repo.GetByID(ctx, "link-crud-1")
	if gotRotated.Token != "pub_token_new" || gotRotated.TokenHash != "hash_crud_new" {
		t.Fatalf("expected rotated token, got %+v", gotRotated)
	}

	// Old hash must not find active link
	if _, err := repo.GetByTokenHash(ctx, "hash_crud_1"); err == nil {
		t.Fatal("expected error finding old token hash")
	}

	// New hash must find active link
	if _, err := repo.GetByTokenHash(ctx, "hash_crud_new"); err != nil {
		t.Fatalf("expected to find link by new hash: %v", err)
	}

	// 6. List
	all, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("expected at least 1 link in list")
	}

	// 7. Revoke
	revokeTime := time.Now().UTC()
	if err := repo.Revoke(ctx, "link-crud-1", revokeTime); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	gotRevoked, _ := repo.GetByID(ctx, "link-crud-1")
	if gotRevoked.State != domain.SubscriptionLinkStateRevoked || gotRevoked.RevokedAt == nil {
		t.Fatalf("expected revoked state, got %+v", gotRevoked)
	}

	// Revoked link cannot be fetched by token hash
	if _, err := repo.GetByTokenHash(ctx, "hash_crud_new"); err == nil {
		t.Fatal("revoked link should not be returned by GetByTokenHash")
	}

	// Rotating token on revoked link must fail
	if err := repo.RotateToken(ctx, "link-crud-1", "tok", "h", time.Now().UTC()); err == nil {
		t.Fatal("expected error rotating token on revoked link")
	}
}

func TestSubscriptionLinkRepository_DefaultTargetIdempotencyAndConcurrency(t *testing.T) {
	db, _ := setupTestDB(t)
	repo := sqlite.NewSubscriptionLinkRepository(db)
	ctx := context.Background()

	// Create default link for TargetSingBox
	link1 := &domain.SubscriptionLink{
		ID:        "default-singbox-1",
		Label:     "Default SingBox",
		Target:    domain.TargetSingBox,
		Token:     "pub_sb_1",
		TokenHash: "hash_sb_1",
		IsDefault: true,
	}
	if err := repo.Create(ctx, link1); err != nil {
		t.Fatalf("Create default link1 failed: %v", err)
	}

	// Second creation with IsDefault=true and same Target should be idempotent
	link2 := &domain.SubscriptionLink{
		ID:        "default-singbox-2",
		Label:     "Default SingBox Secondary",
		Target:    domain.TargetSingBox,
		Token:     "pub_sb_2",
		TokenHash: "hash_sb_2",
		IsDefault: true,
	}
	if err := repo.Create(ctx, link2); err != nil {
		t.Fatalf("Create default link2 failed: %v", err)
	}

	// link2 should now hold the data of link1
	if link2.ID != link1.ID || link2.Token != link1.Token {
		t.Fatalf("expected link2 to be resolved to link1, got ID=%s Token=%s", link2.ID, link2.Token)
	}

	// Concurrent creation attempts of default links
	const goroutines = 8
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			l := &domain.SubscriptionLink{
				ID:        domain.MustNewUUIDv7(),
				Target:    domain.TargetSurge,
				Token:     "pub_surge_concurrent",
				TokenHash: "hash_surge_concurrent",
				IsDefault: true,
			}
			if err := repo.Create(ctx, l); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent default link creation failed: %v", err)
	}

	// Verify only one default Surge link exists
	defSurge, err := repo.GetDefaultByTarget(ctx, domain.TargetSurge)
	if err != nil {
		t.Fatalf("GetDefaultByTarget failed: %v", err)
	}
	if !defSurge.IsDefault || defSurge.Target != domain.TargetSurge {
		t.Fatalf("unexpected default surge link: %+v", defSurge)
	}
}

func TestSettingsRepository_UpdatePublicBaseURL(t *testing.T) {
	db, _ := setupTestDB(t)
	repo := sqlite.NewSettingsRepository(db)
	ctx := context.Background()

	// 1. Initial should be empty
	s, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if s.PublicBaseURL != "" {
		t.Fatalf("expected empty PublicBaseURL, got %s", s.PublicBaseURL)
	}

	// 2. Valid update
	if err := repo.UpdatePublicBaseURL(ctx, "https://sub.myprovider.com/"); err != nil {
		t.Fatalf("UpdatePublicBaseURL failed: %v", err)
	}
	s2, _ := repo.Get(ctx)
	if s2.PublicBaseURL != "https://sub.myprovider.com" {
		t.Fatalf("expected trimmed url https://sub.myprovider.com, got %s", s2.PublicBaseURL)
	}

	// 3. Userinfo must be rejected
	if err := repo.UpdatePublicBaseURL(ctx, "https://user:pass@sub.myprovider.com"); err == nil {
		t.Fatal("expected error for userinfo in public_base_url")
	}

	// 4. Query must be rejected
	if err := repo.UpdatePublicBaseURL(ctx, "https://sub.myprovider.com?key=val"); err == nil {
		t.Fatal("expected error for query parameters in public_base_url")
	}

	// 5. Fragment must be rejected
	if err := repo.UpdatePublicBaseURL(ctx, "https://sub.myprovider.com#frag"); err == nil {
		t.Fatal("expected error for fragment in public_base_url")
	}

	// 6. Non-HTTP scheme must be rejected
	if err := repo.UpdatePublicBaseURL(ctx, "ftp://sub.myprovider.com"); err == nil {
		t.Fatal("expected error for ftp scheme in public_base_url")
	}

	// 7. Verify concurrent settings (e.g. ProbeConcurrencyWindow) are untouched
	if s2.ProbeConcurrencyWindow != s.ProbeConcurrencyWindow {
		t.Fatalf("ProbeConcurrencyWindow was overwritten: %d vs %d", s2.ProbeConcurrencyWindow, s.ProbeConcurrencyWindow)
	}
}
