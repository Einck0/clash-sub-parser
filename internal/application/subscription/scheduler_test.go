package subscription_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

type scheduledReconciler struct {
	run func(context.Context, string) (*inventory.ReconcileResult, error)
}

func (r scheduledReconciler) ReconcileSubscription(ctx context.Context, id string) (*inventory.ReconcileResult, error) {
	return r.run(ctx, id)
}

func receiveID(t *testing.T, ch <-chan string) string {
	t.Helper()
	select {
	case id := <-ch:
		return id
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not arrive")
		return ""
	}
}
func stopScheduler(t *testing.T, s *subscription.Scheduler) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleSaveValidation(t *testing.T) {
	repo := &memorySubscriptions{items: map[string]domain.Subscription{}}
	service := subscription.NewService(repo, nil)
	for _, expression := range []string{"* * * *", "* * * * * *", "@every 1s", "61 * * * *", "CRON_TZ=UTC * * * * *"} {
		_, err := service.Create(context.Background(), subscription.CreateSubscriptionCommand{Name: "fixture", SourceURLSecretRef: "https://fixture.example", Config: domain.SubscriptionConfig{CronSchedule: expression}})
		if de, ok := domain.AsDomainError(err); !ok || de.Code != "invalid_cron_schedule" {
			t.Fatalf("accepted %q: %v", expression, err)
		}
	}
	created, err := service.Create(context.Background(), subscription.CreateSubscriptionCommand{Name: "fixture", SourceURLSecretRef: "https://fixture.example", Config: domain.SubscriptionConfig{CronSchedule: "*/5 * * * *"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Update(context.Background(), subscription.UpdateSubscriptionCommand{ID: created.ID, Revision: created.Revision, Config: &domain.SubscriptionConfig{CronSchedule: "not cron"}})
	if err == nil {
		t.Fatal("bad update accepted")
	}
	stored, _ := repo.GetByID(context.Background(), created.ID)
	if stored.Config.CronSchedule != "*/5 * * * *" || stored.Revision != created.Revision {
		t.Fatal("bad update mutated config")
	}
}

func TestSchedulerDueCronEditsAndFailureBackoff(t *testing.T) {
	ctx := context.Background()
	// Fixed server-local time; no sleeps or wall-clock schedule dependency.
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	var clockMu sync.Mutex
	clock := func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
	setTime := func(at time.Time) { clockMu.Lock(); now = at; clockMu.Unlock() }
	created := now.Add(-time.Hour)
	latest := now.Add(-20 * time.Second)
	repo := &memorySubscriptions{items: map[string]domain.Subscription{
		"due":      {ID: "due", Enabled: true, CreatedAt: created, RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 60}},
		"disabled": {ID: "disabled", Enabled: false, CreatedAt: created, RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 60}},
		"recent":   {ID: "recent", Enabled: true, CreatedAt: created, LastRefreshedAt: &latest, RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 60}},
		"cron":     {ID: "cron", Enabled: true, CreatedAt: now, RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 1}, Config: domain.SubscriptionConfig{CronSchedule: "5 * * * *"}},
	}}
	entered := make(chan string, 10)
	release := make(chan struct{})
	service := subscription.NewService(repo, nil, subscription.WithReconciler(scheduledReconciler{run: func(ctx context.Context, id string) (*inventory.ReconcileResult, error) {
		entered <- id
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, errors.New("fixture failure before history write")
	}}))
	scheduler := subscription.NewScheduler(repo, service, clock)
	defer stopScheduler(t, scheduler)
	if err := scheduler.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if id := receiveID(t, entered); id != "due" {
		t.Fatalf("not-due source ran: %s", id)
	}
	if err := scheduler.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-entered:
		t.Fatalf("overlap/not-due: %s", id)
	default:
	}
	close(release)
	stopScheduler(t, scheduler)
	// A separate controlled scheduler proves cron precedence and next-scan edits.
	repo.items["due"] = domain.Subscription{ID: "due", Enabled: false}
	repo.items["recent"] = domain.Subscription{ID: "recent", Enabled: false}
	now = now.Add(time.Minute)
	scheduler2 := subscription.NewScheduler(repo, service, clock)
	if err := scheduler2.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-entered:
		t.Fatalf("cron interval override ignored: %s", id)
	default:
	}
	sub, _ := repo.GetByID(ctx, "cron")
	sub.Config.CronSchedule = "* * * * *"
	if err := repo.Update(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if err := scheduler2.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if id := receiveID(t, entered); id != "cron" {
		t.Fatalf("edit not honored: %s", id)
	}
	stopScheduler(t, scheduler2)
	// Completed failure has the same due anchor as success.
	setTime(created.Add(2 * time.Hour))
	for _, outcome := range []domain.FetchOutcome{domain.FetchOutcomeSuccess, domain.FetchOutcomeFailed} {
		completed := clock()
		sub := domain.Subscription{ID: "retry", Enabled: true, CreatedAt: created, LastRefreshedAt: &completed, LastRefreshOutcome: &outcome, RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 60}}
		r := &memorySubscriptions{items: map[string]domain.Subscription{"retry": sub}}
		svc := subscription.NewService(r, nil, subscription.WithReconciler(scheduledReconciler{run: func(context.Context, string) (*inventory.ReconcileResult, error) {
			entered <- "retry"
			return nil, errors.New("failure")
		}}))
		sch := subscription.NewScheduler(r, svc, clock)
		if err := sch.ScanOnce(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case <-entered:
			t.Fatal("hot retry at completion")
		default:
		}
		setTime(completed.Add(time.Minute))
		if err := sch.ScanOnce(ctx); err != nil {
			t.Fatal(err)
		}
		if id := receiveID(t, entered); id != "retry" {
			t.Fatal(id)
		}
		stopScheduler(t, sch)
	}
}

func TestSchedulerManualExclusionCapacityAndShutdown(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	repo := &memorySubscriptions{items: map[string]domain.Subscription{}}
	for _, id := range []string{"a", "b", "c"} {
		repo.items[id] = domain.Subscription{ID: id, Enabled: true, CreatedAt: now.Add(-time.Hour), RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 1}}
	}
	entered := make(chan string, 10)
	var active, max atomic.Int32
	svc := subscription.NewService(repo, nil, subscription.WithReconciler(scheduledReconciler{run: func(ctx context.Context, id string) (*inventory.ReconcileResult, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := max.Load(); n > old; old = max.Load() {
			if max.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- id
		<-ctx.Done()
		return nil, ctx.Err()
	}}))
	manualCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := svc.Refresh(manualCtx, "a", "manual", domain.ActorKindAdmin); done <- err }()
	if id := receiveID(t, entered); id != "a" {
		t.Fatal(id)
	}
	scheduler := subscription.NewScheduler(repo, svc, func() time.Time { return now })
	if err := scheduler.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	other := receiveID(t, entered)
	if other == "a" {
		t.Fatal("same source overlapped")
	}
	if _, err := svc.Refresh(ctx, other, "manual-overlap", domain.ActorKindAdmin); err == nil {
		t.Fatal("manual/scheduled overlap allowed")
	}
	if _, err := svc.Refresh(ctx, "a", "manual-overlap", domain.ActorKindAdmin); err == nil {
		t.Fatal("manual/manual overlap allowed")
	}
	if max.Load() != 2 {
		t.Fatalf("expected 2 total workers, got %d", max.Load())
	}
	stopScheduler(t, scheduler)
	if active.Load() != 1 {
		t.Fatalf("scheduled workers not drained: %d", active.Load())
	}
	if err := scheduler.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if active.Load() != 0 {
		t.Fatal("manual worker leaked")
	}
}

func TestScheduledRefreshUsesCurrentRouteSQLite(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.OpenAndMigrate(ctx, sqlite.DefaultConfig(filepath.Join(t.TempDir(), "schedule.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subs := sqlite.NewSubscriptionRepository(db)
	settings := sqlite.NewSettingsRepository(db)
	var envHits, newHits atomic.Int32
	body := "proxies:\n  - {name: Scheduled, type: ss, server: node.example, port: 8388, cipher: aes-128-gcm, password: fixture}\n"
	envProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { envHits.Add(1); fmt.Fprint(w, body) }))
	defer envProxy.Close()
	newProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { newHits.Add(1); fmt.Fprint(w, body) }))
	defer newProxy.Close()
	policy := fetch.DefaultPolicy()
	_ = policy.AddAllowedProxy(envProxy.URL)
	inv := inventory.NewService(db, subs, sqlite.NewSubscriptionFetchRepository(db), sqlite.NewNodeRepository(db), sqlite.NewNodeSourceRepository(db), fetch.NewClientWithPolicy(policy), inventory.WithFetchSettingsRepository(settings), inventory.WithDefaultFetchProxy(envProxy.URL))
	service := subscription.NewService(subs, nil, subscription.WithReconciler(inv))
	now := time.Now().UTC().Add(-time.Minute)
	sub := &domain.Subscription{ID: domain.MustNewUUIDv7(), Name: "fixture", SourceURLSecretRef: "http://8.8.8.8/feed", Enabled: true, RefreshPolicy: domain.RefreshPolicy{IntervalSeconds: 60, TimeoutSeconds: 3, MaxResponseBytes: 4096}, Revision: domain.MustNewUUIDv7(), CreatedAt: now.Add(-time.Hour), UpdatedAt: now}
	if err := subs.Create(ctx, sub); err != nil {
		t.Fatal(err)
	}
	// Manual refresh produces persisted history that postpones scheduled work.
	if _, err := service.Refresh(ctx, sub.ID, "manual", domain.ActorKindAdmin); err != nil {
		t.Fatal(err)
	}
	current, _ := subs.GetByID(ctx, sub.ID)
	if current.LastRefreshedAt == nil {
		t.Fatal("manual completion missing")
	}
	now = current.LastRefreshedAt.Add(59 * time.Second)
	sch := subscription.NewScheduler(subs, service, func() time.Time { return now })
	if err := sch.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if envHits.Load() != 1 {
		t.Fatal("early scheduled fetch")
	}
	if err := settings.UpdateSubscriptionFetch(ctx, "proxy", &newProxy.URL); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	stopScheduler(t, sch)
	// A channel from the real audit boundary observes committed completion,
	// without polling, fixed sleeps, or a shutdown/fetch race.
	audit := &completionAudit{done: make(chan struct{}, 1)}
	service2 := subscription.NewService(subs, audit, subscription.WithReconciler(inv))
	sch2 := subscription.NewScheduler(subs, service2, func() time.Time { return now })
	defer stopScheduler(t, sch2)
	if err := sch2.ScanOnce(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-audit.done:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduled refresh not committed")
	}
	if newHits.Load() < 1 || envHits.Load() != 1 {
		t.Fatalf("stale route: env=%d new=%d", envHits.Load(), newHits.Load())
	}
	latest, err := subs.GetByID(ctx, sub.ID)
	if err != nil || latest.LastRefreshOutcome == nil || *latest.LastRefreshOutcome != domain.FetchOutcomeSuccess {
		t.Fatalf("missing successful scheduled history: %+v %v", latest, err)
	}
}

type completionAudit struct{ done chan struct{} }

func (a *completionAudit) Record(_ context.Context, e *domain.AuditEvent) error {
	if e.Action == "subscription.refresh" {
		a.done <- struct{}{}
	}
	return nil
}
func (a *completionAudit) List(context.Context, domain.AuditFilter) ([]domain.AuditEvent, int, error) {
	return nil, 0, nil
}
