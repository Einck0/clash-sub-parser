package subscription

import (
	"context"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
	"github.com/robfig/cron/v3"
)

var scheduleParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func validateSchedule(config domain.SubscriptionConfig) error {
	expression := strings.TrimSpace(config.CronSchedule)
	if expression == "" {
		return nil
	}
	// Only a standard five-field schedule; timezone is the server's time.Local.
	if len(strings.Fields(expression)) != 5 {
		return domain.NewValidationError("invalid_cron_schedule", "cron_schedule must have five fields in server timezone")
	}
	if _, err := scheduleParser.Parse(expression); err != nil {
		return domain.NewValidationError("invalid_cron_schedule", "invalid five-field cron_schedule")
	}
	return nil
}

func refreshDue(sub domain.Subscription, now time.Time, lastAttempt time.Time) bool {
	if !sub.Enabled {
		return false
	}
	base := sub.CreatedAt
	if sub.LastRefreshedAt != nil {
		base = *sub.LastRefreshedAt
	}
	if lastAttempt.After(base) {
		base = lastAttempt
	}
	expression := strings.TrimSpace(sub.Config.CronSchedule)
	if expression != "" {
		if validateSchedule(sub.Config) != nil {
			return false
		}
		schedule, err := scheduleParser.Parse(expression)
		if err != nil {
			return false
		}
		next := schedule.Next(base.In(time.Local))
		return !next.IsZero() && !now.Before(next)
	}
	interval := normalizedPolicy(sub.RefreshPolicy).IntervalSeconds
	// Compare seconds without overflowing a time.Duration for large saved values.
	return now.Sub(base).Seconds() >= float64(interval)
}

// Scheduler owns periodic refresh workers. It never queues catch-up work and
// shares Service.Refresh's per-source exclusion and two-worker limit with manual requests.
// Constructing services does not start network activity; only serve calls Start.
type Scheduler struct {
	repo      domain.SubscriptionRepository
	service   *Service
	now       func() time.Time
	mu        sync.Mutex
	running   map[string]bool
	attempted map[string]time.Time
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   bool
	started   bool
	wg        sync.WaitGroup
}

func NewScheduler(repo domain.SubscriptionRepository, service *Service, now func() time.Time) *Scheduler {
	if now == nil {
		now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{repo: repo, service: service, now: now, running: map[string]bool{}, attempted: map[string]time.Time{}, ctx: ctx, cancel: cancel}
}

// ScanOnce reads current configuration/history and starts only available work.
// A completion fallback prevents floods even if a failure could not persist history.
func (s *Scheduler) ScanOnce(ctx context.Context) error {
	scanParent := ctx
	ctx, cancelScan := context.WithCancel(ctx)
	stopScan := context.AfterFunc(s.ctx, cancelScan)
	defer func() { stopScan(); cancelScan() }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.ctx.Err() != nil {
		return nil
	}
	now := s.now()
	for page := 1; ; page++ {
		items, total, err := s.repo.List(ctx, domain.SubscriptionFilter{Pagination: domain.Pagination{Page: page, PageSize: 100}, EnabledOnly: true})
		if err != nil {
			return err
		}
		for _, sub := range items {
			if len(s.running) >= 2 {
				return nil
			}
			last := s.attempted[sub.ID]
			if s.running[sub.ID] || !refreshDue(sub, now, last) {
				continue
			}
			s.running[sub.ID] = true
			s.wg.Add(1)
			go func(id string) {
				defer s.wg.Done()
				// Both scan cancellation and lifecycle cancellation reach the real fetch.
				workCtx, cancel := context.WithCancel(s.ctx)
				stop := context.AfterFunc(scanParent, cancel)
				defer func() { stop(); cancel() }()
				result, err := s.service.refresh(workCtx, id, "scheduled-"+domain.MustNewUUIDv7(), domain.ActorKindSystem, &now, last)
				// Only genuine attempts need backoff. Admission conflicts and a
				// stale scan must not postpone a source that has not been fetched.
				attempted := result != nil
				if de, ok := domain.AsDomainError(err); err != nil && (!ok || (de.Code != "subscription_refresh_busy" && de.Code != "subscription_refresh_in_progress" && de.Code != "subscription_disabled")) {
					attempted = true
				}
				fallback := attempted
				if attempted {
					if current, e := s.repo.GetByID(workCtx, id); e == nil && current.LastRefreshedAt != nil && (sub.LastRefreshedAt == nil || current.LastRefreshedAt.After(*sub.LastRefreshedAt)) {
						fallback = false
					}
				}
				s.mu.Lock()
				delete(s.running, id)
				if fallback {
					s.attempted[id] = s.now()
				} else if attempted {
					delete(s.attempted, id)
				}
				s.mu.Unlock()
			}(sub.ID)
		}
		if page*100 >= total || len(items) == 0 {
			return nil
		}
	}
}

// Start runs until shutdown. Stop must be called before closing the database.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.stopped || s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		stop := context.AfterFunc(ctx, s.cancel)
		defer stop()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if s.ctx.Err() != nil {
				return
			}
			_ = s.ScanOnce(s.ctx)
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Scheduler) Stop(ctx context.Context) error {
	s.cancel()
	s.mu.Lock()
	s.stopped = true
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
