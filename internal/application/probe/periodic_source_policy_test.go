package probe

import (
	"context"
	"testing"
	"time"

	"clash-sub-parser/internal/domain"
)

type memorySubRepo struct {
	items map[string]domain.Subscription
}

func (m *memorySubRepo) GetByID(_ context.Context, id string) (*domain.Subscription, error) {
	s, ok := m.items[id]
	if !ok {
		return nil, domain.NewNotFoundError("sub_not_found", "not found")
	}
	return &s, nil
}

func (m *memorySubRepo) GetPrimary(_ context.Context) (*domain.Subscription, error) {
	for _, s := range m.items {
		if s.IsPrimary && s.Enabled {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *memorySubRepo) List(_ context.Context, filter domain.SubscriptionFilter) ([]domain.Subscription, int, error) {
	var list []domain.Subscription
	for _, s := range m.items {
		if filter.EnabledOnly && !s.Enabled {
			continue
		}
		list = append(list, s)
	}
	return list, len(list), nil
}

func (m *memorySubRepo) Create(_ context.Context, s *domain.Subscription) error {
	m.items[s.ID] = *s
	return nil
}

func (m *memorySubRepo) Update(_ context.Context, s *domain.Subscription) error {
	m.items[s.ID] = *s
	return nil
}

func (m *memorySubRepo) Delete(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}

type memoryNodeSourceRepo struct {
	sources map[string][]domain.NodeSource
}

func (m *memoryNodeSourceRepo) ListByNode(_ context.Context, logicalID string) ([]domain.NodeSource, error) {
	return m.sources[logicalID], nil
}

func (m *memoryNodeSourceRepo) ListByNodes(_ context.Context, logicalIDs []string) (map[string][]domain.NodeSource, error) {
	res := make(map[string][]domain.NodeSource, len(logicalIDs))
	for _, id := range logicalIDs {
		res[id] = m.sources[id]
	}
	return res, nil
}

func (m *memoryNodeSourceRepo) ListBySubscription(_ context.Context, subID string) ([]domain.NodeSource, error) {
	var res []domain.NodeSource
	for _, list := range m.sources {
		for _, s := range list {
			if s.SubscriptionID == subID {
				res = append(res, s)
			}
		}
	}
	return res, nil
}

func (m *memoryNodeSourceRepo) Upsert(_ context.Context, src *domain.NodeSource) error {
	m.sources[src.NodeLogicalID] = append(m.sources[src.NodeLogicalID], *src)
	return nil
}

func (m *memoryNodeSourceRepo) DeleteBySubscriptionAndFetch(_ context.Context, subID string, currentFetchID string) error {
	return nil
}

type memoryHistoryRepo struct {
	histories map[string][]domain.NodeSourceHistory
}

func (m *memoryHistoryRepo) InsertBatch(_ context.Context, records []domain.NodeSourceHistory) error {
	for _, r := range records {
		m.histories[r.NodeLogicalID] = append(m.histories[r.NodeLogicalID], r)
	}
	return nil
}

func (m *memoryHistoryRepo) ListByNodeLogicalID(_ context.Context, nodeLogicalID string) ([]domain.NodeSourceHistory, error) {
	return m.histories[nodeLogicalID], nil
}

func (m *memoryHistoryRepo) ListBySubscriptionID(_ context.Context, subscriptionID string) ([]domain.NodeSourceHistory, error) {
	var res []domain.NodeSourceHistory
	for _, list := range m.histories {
		for _, h := range list {
			if h.SubscriptionID != nil && *h.SubscriptionID == subscriptionID {
				res = append(res, h)
			}
		}
	}
	return res, nil
}

type mockRunnerWithPool struct {
	mockRunner
	poolNodes map[string]bool
}

func (r *mockRunnerWithPool) IsNodeInPool(logicalID string) bool {
	return r.poolNodes[logicalID]
}

func TestPeriodicCoordinator_MultiSourceShortestInterval(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	currentClock := t0

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 3600, // Global interval 3600s
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &t0,
		Generation:      1,
		UpdatedAt:       t0,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-multi", Active: true, ConnectionRevision: 1},
		},
	}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo()
	runner := &mockRunner{}

	// Setup two sources:
	// Sub A: custom 100s
	// Sub B: custom 300s
	invA := 100
	invB := 300
	subRepo := &memorySubRepo{
		items: map[string]domain.Subscription{
			"sub-a": {
				ID:      "sub-a",
				Enabled: true,
				Config: domain.SubscriptionConfig{
					ProbePolicy: &domain.SubscriptionProbePolicy{
						Mode:            domain.ProbePolicyModeCustom,
						IntervalSeconds: &invA,
					},
				},
			},
			"sub-b": {
				ID:      "sub-b",
				Enabled: true,
				Config: domain.SubscriptionConfig{
					ProbePolicy: &domain.SubscriptionProbePolicy{
						Mode:            domain.ProbePolicyModeCustom,
						IntervalSeconds: &invB,
					},
				},
			},
		},
	}
	sourceRepo := &memoryNodeSourceRepo{
		sources: map[string][]domain.NodeSource{
			"node-multi": {
				{NodeLogicalID: "node-multi", SubscriptionID: "sub-a"},
				{NodeLogicalID: "node-multi", SubscriptionID: "sub-b"},
			},
		},
	}

	rev := int64(1)
	// Initial observation at t0
	obsRepo.SetObservation("node-multi", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "node-multi",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		ObservedAt:         t0,
		ConnectionRevision: &rev,
	})

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorSubscriptions(subRepo),
		WithCoordinatorNodeSources(sourceRepo),
		WithCoordinatorClock(func() time.Time { return currentClock }),
	)

	// Case 1: At t0 + 50s (age 50s < shortest interval 100s): not due, 0 runs
	currentClock = t0.Add(50 * time.Second)
	schedBefore, _ := schedRepo.Get(ctx)
	schedBefore.NextDueAt = &currentClock
	_ = schedRepo.Update(ctx, schedBefore)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow at +50s failed: %v", err)
	}
	if len(runner.executedRuns) != 0 {
		t.Fatalf("expected 0 runs at +50s, got %d", len(runner.executedRuns))
	}

	// Case 2: At t0 + 110s (age 110s >= shortest interval 100s): due and dispatched!
	currentClock = t0.Add(110 * time.Second)
	schedBefore, _ = schedRepo.Get(ctx)
	schedBefore.NextDueAt = &currentClock
	_ = schedRepo.Update(ctx, schedBefore)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow at +110s failed: %v", err)
	}
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected 1 run at +110s, got %d", len(runner.executedRuns))
	}
	nodeIDs := runner.executedNodeIDs[runner.executedRuns[0].ID]
	if len(nodeIDs) != 1 || nodeIDs[0] != "node-multi" {
		t.Fatalf("expected node-multi scheduled, got %v", nodeIDs)
	}
}

func TestPeriodicCoordinator_StoppedAndDisabledSources_DoNotAutoSchedule(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	currentClock := t0

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 3600,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &t0,
		Generation:      1,
		UpdatedAt:       t0,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-stopped", Active: true, ConnectionRevision: 1},
		},
	}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo() // unobserved
	runner := &mockRunner{}

	// Sub 1: disabled probe policy (mode: disabled)
	// Sub 2: disabled subscription (enabled: false)
	subRepo := &memorySubRepo{
		items: map[string]domain.Subscription{
			"sub-disabled-probe": {
				ID:      "sub-disabled-probe",
				Enabled: true,
				Config: domain.SubscriptionConfig{
					ProbePolicy: &domain.SubscriptionProbePolicy{
						Mode: domain.ProbePolicyModeDisabled,
					},
				},
			},
			"sub-disabled-sub": {
				ID:      "sub-disabled-sub",
				Enabled: false, // Subscription disabled
				Config: domain.SubscriptionConfig{
					ProbePolicy: &domain.SubscriptionProbePolicy{
						Mode: domain.ProbePolicyModeInherit,
					},
				},
			},
		},
	}
	sourceRepo := &memoryNodeSourceRepo{
		sources: map[string][]domain.NodeSource{
			"node-stopped": {
				{NodeLogicalID: "node-stopped", SubscriptionID: "sub-disabled-probe"},
				{NodeLogicalID: "node-stopped", SubscriptionID: "sub-disabled-sub"},
			},
		},
	}

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorSubscriptions(subRepo),
		WithCoordinatorNodeSources(sourceRepo),
		WithCoordinatorClock(func() time.Time { return currentClock }),
	)

	// Automatic sweep: "来源全部停测不自动排" -> must NOT be scheduled
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}
	if len(runner.executedRuns) != 0 {
		t.Fatalf("expected 0 runs in automatic sweep for stopped sources, got %d", len(runner.executedRuns))
	}

	// Manual TriggerImmediate: "manual不受来源周期限制" -> can be scheduled manually
	if err := coord.TriggerImmediate(ctx); err != nil {
		t.Fatalf("TriggerImmediate failed: %v", err)
	}
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected manual trigger to schedule node even if source stopped, got %d runs", len(runner.executedRuns))
	}
}

func TestPeriodicCoordinator_ManualConfirmedAndOrphanNodes(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	currentClock := t0

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 3600,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &t0,
		Generation:      1,
		UpdatedAt:       t0,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-manual-confirmed", Active: true, ConnectionRevision: 1},
			{LogicalID: "node-orphan", Active: true, ConnectionRevision: 1},
		},
	}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo() // both unobserved
	runner := &mockRunner{}

	subRepo := &memorySubRepo{items: make(map[string]domain.Subscription)}
	sourceRepo := &memoryNodeSourceRepo{sources: make(map[string][]domain.NodeSource)}
	historyRepo := &memoryHistoryRepo{
		histories: map[string][]domain.NodeSourceHistory{
			"node-manual-confirmed": {
				{
					NodeLogicalID: "node-manual-confirmed",
					Cause:         domain.CauseManualConfirmed,
				},
			},
			// node-orphan has NO manual_confirmed history
		},
	}

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorSubscriptions(subRepo),
		WithCoordinatorNodeSources(sourceRepo),
		WithCoordinatorSourceHistory(historyRepo),
		WithCoordinatorClock(func() time.Time { return currentClock }),
	)

	// Automatic sweep:
	// "manual confirmed 无有效源继承全局，普通孤立不复活"
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}

	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runner.executedRuns))
	}
	scheduledNodes := runner.executedNodeIDs[runner.executedRuns[0].ID]
	if len(scheduledNodes) != 1 || scheduledNodes[0] != "node-manual-confirmed" {
		t.Fatalf("expected only node-manual-confirmed scheduled, got %v", scheduledNodes)
	}
}

func TestPeriodicCoordinator_ConnectionRevisionMismatch_TriggersImmediate(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	currentClock := t0

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 86400, // 1 day interval
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &t0,
		Generation:      1,
		UpdatedAt:       t0,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-rev", Active: true, ConnectionRevision: 2}, // Current rev is 2
		},
	}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo()
	runner := &mockRunner{}

	// Observation was recorded only 10 seconds ago, but for old revision 1!
	oldRev := int64(1)
	obsRepo.SetObservation("node-rev", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "node-rev",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictAvailable,
		ObservedAt:         t0.Add(-10 * time.Second),
		ConnectionRevision: &oldRev,
	})

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorClock(func() time.Time { return currentClock }),
	)

	// Even though age (10s) < interval (86400s), revision mismatch triggers immediate re-probe!
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected revision mismatch to trigger probe, got %d runs", len(runner.executedRuns))
	}
}

func TestPeriodicCoordinator_FailureCooldownAndPoolDeduplication(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	currentClock := t0

	sched := domain.ProbeSchedule{
		Enabled:         true,
		IntervalSeconds: 60,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &t0,
		Generation:      1,
		UpdatedAt:       t0,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-in-cooldown", Active: true, ConnectionRevision: 1},
			{LogicalID: "node-in-pool", Active: true, ConnectionRevision: 1},
			{LogicalID: "node-ready", Active: true, ConnectionRevision: 1},
		},
	}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo()

	// node-in-cooldown failed 60s ago, failure cooldown is 300s -> in cooldown!
	rev := int64(1)
	obsRepo.SetObservation("node-in-cooldown", domain.ProbeKindBaseline, domain.ProbeObservation{
		NodeLogicalID:      "node-in-cooldown",
		Kind:               domain.ProbeKindBaseline,
		Verdict:            domain.VerdictError,
		ObservedAt:         t0.Add(-60 * time.Second),
		ConnectionRevision: &rev,
	})

	// Runner where node-in-pool is marked as currently in pool
	runner := &mockRunnerWithPool{
		poolNodes: map[string]bool{
			"node-in-pool": true,
		},
	}

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorFailureCooldown(300*time.Second),
		WithCoordinatorClock(func() time.Time { return currentClock }),
	)

	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}

	// Only node-ready should be scheduled
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runner.executedRuns))
	}
	scheduled := runner.executedNodeIDs[runner.executedRuns[0].ID]
	if len(scheduled) != 1 || scheduled[0] != "node-ready" {
		t.Fatalf("expected only node-ready scheduled, got %v", scheduled)
	}
}

func TestPeriodicCoordinator_GlobalSwitchDisabled_AutoBlockedManualAvailable(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	// Global switch disabled!
	sched := domain.ProbeSchedule{
		Enabled:         false,
		IntervalSeconds: 3600,
		Kinds:           []domain.ProbeKind{domain.ProbeKindBaseline},
		NextDueAt:       &t0,
		Generation:      1,
		UpdatedAt:       t0,
	}

	schedRepo := newMemoryScheduleRepo(sched)
	nodeRepo := &memoryNodeRepo{
		nodes: []domain.Node{
			{LogicalID: "node-1", Active: true, ConnectionRevision: 1},
		},
	}
	runRepo := &memoryRuns{items: make(map[string]domain.ProbeRun)}
	obsRepo := newMemoryObsRepo() // unobserved
	runner := &mockRunner{}

	coord := NewPeriodicCoordinator(
		schedRepo,
		nodeRepo,
		runRepo,
		runner,
		WithCoordinatorObservations(obsRepo),
		WithCoordinatorClock(func() time.Time { return t0 }),
	)

	// Automatic sweep must do nothing when global switch is false
	if err := coord.TriggerWindow(); err != nil {
		t.Fatalf("TriggerWindow failed: %v", err)
	}
	if len(runner.executedRuns) != 0 {
		t.Fatalf("expected 0 runs when global schedule is disabled, got %d", len(runner.executedRuns))
	}

	// Manual TriggerImmediate must STILL work even when global switch is false!
	if err := coord.TriggerImmediate(ctx); err != nil {
		t.Fatalf("TriggerImmediate failed: %v", err)
	}
	if len(runner.executedRuns) != 1 {
		t.Fatalf("expected manual trigger to work even when global schedule is disabled, got %d runs", len(runner.executedRuns))
	}
}
