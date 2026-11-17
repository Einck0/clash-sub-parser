package inventory_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/repository/sqlite"
)

// Compare current facts, including payload identity and exact raw bytes, so a
// stale completion cannot pass merely by leaving table row counts unchanged.
type fencingSnapshot struct {
	sequence                                                                int64
	sourceRevision, appliedPayload, appliedFetch, latestFetch, latestStatus string
	payloadFetch, rawFetch, rawHex, members, latestError, latestOutcome     string
	fetches, payloads, originals, sources, nodes, entries                   int
}

func readFencingSnapshot(t *testing.T, ctx context.Context, db *sql.DB, subID string) fencingSnapshot {
	t.Helper()
	var s fencingSnapshot
	err := db.QueryRowContext(ctx, `SELECT attempt_sequence,source_revision,COALESCE(applied_payload_id,''),COALESCE(applied_fetch_id,''),
		COALESCE(last_attempt_fetch_id,''),last_attempt_status FROM subscription_current_state WHERE subscription_id=?`, subID).
		Scan(&s.sequence, &s.sourceRevision, &s.appliedPayload, &s.appliedFetch, &s.latestFetch, &s.latestStatus)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT fetch_id FROM subscription_original_configs WHERE subscription_id=?),''),
		COALESCE((SELECT hex(body_blob) FROM subscription_original_configs WHERE subscription_id=?),''),
		COALESCE((SELECT group_concat(item, '|') FROM (SELECT n.display_name||':'||ns.last_seen_fetch_id AS item FROM node_sources ns JOIN nodes n ON n.logical_id=ns.node_logical_id WHERE ns.subscription_id=? ORDER BY item)),''),
		(SELECT COUNT(*) FROM subscription_fetches WHERE subscription_id=?),
		(SELECT COUNT(*) FROM subscription_payloads WHERE subscription_id=?),
		(SELECT COUNT(*) FROM subscription_original_configs WHERE subscription_id=?),
		(SELECT COUNT(*) FROM node_sources WHERE subscription_id=?),
		(SELECT COUNT(*) FROM nodes)`, subID, subID, subID, subID, subID, subID, subID).
		Scan(&s.rawFetch, &s.rawHex, &s.members, &s.fetches, &s.payloads, &s.originals, &s.sources, &s.nodes)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT fetch_id FROM subscription_payloads WHERE id=? AND subscription_id=?),''),
		COALESCE((SELECT outcome FROM subscription_fetches WHERE id=? AND subscription_id=?),''),
		COALESCE((SELECT redacted_error FROM subscription_fetches WHERE id=? AND subscription_id=?),''),
		(SELECT COUNT(*) FROM subscription_entries WHERE subscription_id=?)`, s.appliedPayload, subID, s.latestFetch, subID, s.latestFetch, subID, subID).
		Scan(&s.payloadFetch, &s.latestOutcome, &s.latestError, &s.entries)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestInventoryService_ReconcileSubscription_AdmissionFencesFailureInterleavings(t *testing.T) {
	for _, scenario := range []string{
		"newer-fetch-fails", "newer-parse-fails", "older-finishes-while-newer-pending",
		"older-fetch-fails-after-newer-success", "older-parse-fails-after-newer-success",
	} {
		for _, seeded := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/seeded=%t", scenario, seeded), func(t *testing.T) {
				db, subRepo, svc, fetcher := setupOriginalTestDB(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				sub := createOrigSub(t, subRepo, "sub-fence", "Fence", "https://example.com/fence")
				response := func(name, ip string) *fetch.Response {
					body := []byte(fmt.Sprintf("proxies:\n  - name: %s\n    type: ss\n    server: %s\n    port: 8388\n    cipher: aes-128-gcm\n    password: p\n", name, ip))
					return &fetch.Response{StatusCode: 200, Body: body, ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body))}
				}
				seedBody := response("seed-node", "1.1.1.1")
				olderBody := response("older-node", "2.2.2.2")
				newerBody := response("newer-node", "3.3.3.3")
				recoveryBody := response("recovery-node", "4.4.4.4")
				var seedID string
				if seeded {
					fetcher.setResponse(sub.SourceURLSecretRef, seedBody)
					seed, err := svc.ReconcileSubscription(ctx, sub.ID)
					if err != nil || seed == nil || seed.Outcome != domain.FetchOutcomeSuccess {
						t.Fatalf("seed: result=%+v err=%v", seed, err)
					}
					seedID = seed.FetchID
				}
				// The existing newer-commit regression covers all one/two-Service
				// combinations. Exercise both here without duplicating that suite.
				newerSvc := svc
				if seeded {
					newerSvc = inventory.NewService(db, subRepo, sqlite.NewSubscriptionFetchRepository(db), sqlite.NewNodeRepository(db), sqlite.NewNodeSourceRepository(db), fetcher)
				}
				newerFails := strings.HasPrefix(scenario, "newer-")
				olderFails := strings.Contains(scenario, "fails-after-newer-success")
				parseFailure := strings.Contains(scenario, "parse-fails")
				injectFailure := func() {
					if parseFailure {
						// Reuse the existing parse-retention test's malformed document:
						// HTTP 200, nonempty, below budget, with no accepted nodes.
						body := []byte("{{this is not yaml or base64 or anything valid node list}}")
						fetcher.setResponse(sub.SourceURLSecretRef, &fetch.Response{StatusCode: 200, Body: body, ContentDigest: fmt.Sprintf("%x", sha256.Sum256(body))})
					} else {
						fetcher.setError(sub.SourceURLSecretRef, errors.New("synthetic fetch failure"))
					}
				}

				olderEntered, newerEntered := make(chan struct{}), make(chan struct{})
				releaseOlder, releaseNewer := make(chan struct{}), make(chan struct{})
				var olderOnce, newerOnce sync.Once
				unblockOlder := func() { olderOnce.Do(func() { close(releaseOlder) }) }
				unblockNewer := func() { newerOnce.Do(func() { close(releaseNewer) }) }
				var mu sync.Mutex
				calls := 0
				fetcher.onFetch = func(string) {
					mu.Lock()
					calls++
					call := calls
					mu.Unlock()
					switch call {
					case 1:
						close(olderEntered)
						select {
						case <-releaseOlder:
						case <-ctx.Done():
						}
					case 2:
						close(newerEntered)
						select {
						case <-releaseNewer:
						case <-ctx.Done():
						}
					}
				}
				type attempt struct {
					result *inventory.ReconcileResult
					err    error
				}
				olderDone, newerDone := make(chan attempt, 1), make(chan attempt, 1)
				var workers sync.WaitGroup
				start := func(service *inventory.Service, done chan<- attempt) {
					workers.Add(1)
					go func() {
						defer workers.Done()
						result, err := service.ReconcileSubscription(ctx, sub.ID)
						done <- attempt{result, err}
					}()
				}
				defer func() {
					unblockOlder()
					unblockNewer()
					cancel()
					exited := make(chan struct{})
					go func() { workers.Wait(); close(exited) }()
					select {
					case <-exited:
					case <-time.After(5 * time.Second):
						t.Error("refresh workers did not exit during cleanup")
					}
				}()
				waitEntered := func(ch <-chan struct{}) {
					t.Helper()
					select {
					case <-ch:
					case <-ctx.Done():
						t.Fatal("fetch barrier timed out")
					}
				}
				waitResult := func(ch <-chan attempt) attempt {
					t.Helper()
					select {
					case a := <-ch:
						return a
					case <-ctx.Done():
						t.Fatal("reconcile timed out")
						return attempt{}
					}
				}
				assertConflict := func(a attempt) {
					t.Helper()
					var e *domain.DomainError
					if a.result != nil || !errors.As(a.err, &e) || e.Code != "subscription_refresh_conflict" {
						t.Fatalf("expected stale conflict: result=%+v err=%v", a.result, a.err)
					}
				}
				// Reaching Fetch proves that BeginSubscriptionAttempt committed.
				start(svc, olderDone)
				waitEntered(olderEntered)
				beforeNewer := readFencingSnapshot(t, ctx, db, sub.ID)
				start(newerSvc, newerDone)
				waitEntered(newerEntered)
				pending := readFencingSnapshot(t, ctx, db, sub.ID)
				wantPending := beforeNewer
				wantPending.sequence++
				if pending != wantPending {
					t.Fatalf("admission must only advance the fence: before=%+v after=%+v", beforeNewer, pending)
				}
				var newer attempt
				if scenario == "older-finishes-while-newer-pending" {
					fetcher.setResponse(sub.SourceURLSecretRef, olderBody)
					unblockOlder()
					assertConflict(waitResult(olderDone))
					if after := readFencingSnapshot(t, ctx, db, sub.ID); after != pending {
						t.Fatalf("stale success changed pending facts: before=%+v after=%+v", pending, after)
					}
					fetcher.setResponse(sub.SourceURLSecretRef, newerBody)
					unblockNewer()
					newer = waitResult(newerDone)
				} else {
					if newerFails {
						injectFailure()
					} else {
						fetcher.setResponse(sub.SourceURLSecretRef, newerBody)
					}
					unblockNewer()
					newer = waitResult(newerDone)
					before := readFencingSnapshot(t, ctx, db, sub.ID)
					if olderFails {
						injectFailure()
					} else {
						fetcher.setResponse(sub.SourceURLSecretRef, olderBody)
					}
					unblockOlder()
					assertConflict(waitResult(olderDone))
					if after := readFencingSnapshot(t, ctx, db, sub.ID); after != before {
						t.Fatalf("stale completion changed facts: before=%+v after=%+v", before, after)
					}
				}
				s := readFencingSnapshot(t, ctx, db, sub.ID)
				if newer.result == nil {
					t.Fatalf("newer missing result: %v", newer.err)
				}
				if newerFails {
					if newer.err == nil || newer.result.Outcome != domain.FetchOutcomeFailed || s.latestFetch != newer.result.FetchID || s.latestStatus != "failed" || s.latestOutcome != "failed" || s.latestError == "" || s.latestError != newer.result.RedactedError {
						t.Fatalf("failed latest attempt not recorded: result=%+v err=%v state=%+v", newer.result, newer.err, s)
					}
					if parseFailure && !strings.Contains(s.latestError, "reason_code:parse_failed") {
						t.Fatalf("fixture did not exercise parse failure: %s", s.latestError)
					}
					// A failed B may add only its latest-attempt summary. With a seed,
					// the summary union contains two rows, not one generation each.
					wantFailed := pending
					wantFailed.latestFetch = newer.result.FetchID
					wantFailed.latestStatus = "failed"
					wantFailed.latestOutcome = "failed"
					wantFailed.latestError = newer.result.RedactedError
					wantFailed.fetches++
					if s != wantFailed {
						t.Fatalf("failure changed applied/raw facts: want=%+v got=%+v", wantFailed, s)
					}
					if seeded {
						if s.appliedFetch != seedID || s.rawFetch != seedID || s.rawHex != fmt.Sprintf("%X", seedBody.Body) || s.appliedPayload == "" || s.members != "seed-node:"+seedID || s.fetches != 2 || s.payloads != 1 || s.originals != 1 || s.sources != 1 || s.nodes != 1 {
							t.Fatalf("failed attempt changed seed facts: %+v", s)
						}
					} else if s.appliedFetch != "" || s.appliedPayload != "" || s.rawFetch != "" || s.rawHex != "" || s.members != "" || s.fetches != 1 || s.payloads != 0 || s.originals != 0 || s.sources != 0 || s.nodes != 0 {
						t.Fatalf("failed attempt fabricated applied facts: %+v", s)
					}
				} else {
					if newer.err != nil || newer.result.Outcome != domain.FetchOutcomeSuccess || s.latestFetch != newer.result.FetchID || s.latestStatus != "success" || s.latestOutcome != "success" || s.latestError != "" || s.appliedFetch != newer.result.FetchID || s.appliedPayload == "" || s.rawFetch != newer.result.FetchID || s.rawHex != fmt.Sprintf("%X", newerBody.Body) || s.members != "newer-node:"+newer.result.FetchID || s.fetches != 1 || s.payloads != 1 || s.originals != 1 || s.sources != 1 {
						t.Fatalf("newer successful facts inconsistent: result=%+v err=%v state=%+v", newer.result, newer.err, s)
					}
				}
				if s.sequence != pending.sequence || s.sourceRevision != sub.Revision || s.payloadFetch != s.appliedFetch || s.entries != s.payloads {
					t.Fatalf("fence or applied payload/entry mismatch: %+v", s)
				}
				fetcher.setResponse(sub.SourceURLSecretRef, recoveryBody)
				recovery, err := svc.ReconcileSubscription(ctx, sub.ID)
				if err != nil || recovery == nil || recovery.Outcome != domain.FetchOutcomeSuccess {
					t.Fatalf("later ordinary refresh failed: result=%+v err=%v", recovery, err)
				}
				s = readFencingSnapshot(t, ctx, db, sub.ID)
				if s.sequence != pending.sequence+1 || s.appliedFetch != recovery.FetchID || s.payloadFetch != recovery.FetchID || s.latestFetch != recovery.FetchID || s.latestStatus != "success" || s.latestOutcome != "success" || s.latestError != "" || s.rawFetch != recovery.FetchID || s.rawHex != fmt.Sprintf("%X", recoveryBody.Body) || s.members != "recovery-node:"+recovery.FetchID || s.fetches != 1 || s.payloads != 1 || s.originals != 1 || s.sources != 1 || s.entries != 1 {
					t.Fatalf("recovery facts inconsistent: %+v", s)
				}
				current, err := subRepo.GetByID(ctx, sub.ID)
				if err != nil || current.Revision != sub.Revision || current.SourceURLSecretRef != sub.SourceURLSecretRef {
					t.Fatalf("refresh changed editable subscription: current=%+v err=%v", current, err)
				}
				mu.Lock()
				fetchCalls := calls
				mu.Unlock()
				if fetchCalls != 3 {
					t.Fatalf("expected exactly A, B and C fetches, got %d (hidden retry/refetch)", fetchCalls)
				}
			})
		}
	}
}
