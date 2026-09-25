package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/probe/queue"

	_ "modernc.org/sqlite"
)

func TestServeShutdownDrainFailureKeepsDatabaseOpenAndReturnsFailure(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	completed := make(chan struct{})
	scheduler, err := queue.NewScheduler(queue.Config{Concurrency: 10, QueueSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Submit(queue.Task{ID: "stuck", RunID: "fixture", Execute: func(context.Context) error { close(started); <-release; return nil }, OnComplete: func(error) { close(completed) }}); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	drainErr := scheduler.Drain(ctx)
	if !errors.Is(drainErr, context.DeadlineExceeded) {
		t.Fatalf("Drain error = %v, want deadline exceeded", drainErr)
	}
	if clean := serveShutdownResult(drainErr, func() { t.Fatal("database closed before active Execute completed") }); clean {
		t.Fatal("shutdown reported clean despite incomplete drain")
	}
	close(release)
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("fixture callback did not complete after release")
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
	defer cleanupCancel()
	if err := scheduler.Drain(cleanupCtx); err != nil {
		t.Fatalf("cleanup Drain: %v", err)
	}
}

func TestServeShutdownCleanDrainClosesDatabaseAfterCallbacks(t *testing.T) {
	callbackDone := make(chan struct{})
	scheduler, err := queue.NewScheduler(queue.Config{Concurrency: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Submit(queue.Task{ID: "done", RunID: "fixture", Execute: func(context.Context) error { return nil }, OnComplete: func(error) { close(callbackDone) }}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := scheduler.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	closed := false
	if !serveShutdownResult(nil, func() { closed = true }) || !closed {
		t.Fatalf("successful Drain did not close database: closed=%v", closed)
	}
	select {
	case <-callbackDone:
	default:
		t.Fatal("Drain returned before callback completed")
	}
}

const helperEnv = "CSP_TEST_HELPER_SERVE"

type lifecycleEvent struct {
	Seq    int    `json:"seq"`
	Event  string `json:"event"`
	RunID  string `json:"run_id,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type lifecycleEventEmitter struct {
	mu  sync.Mutex
	w   io.Writer
	seq int
}

func (e *lifecycleEventEmitter) Emit(event, runID, detail string) {
	if e == nil || e.w == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seq++
	rec := lifecycleEvent{
		Seq:    e.seq,
		Event:  event,
		RunID:  runID,
		Detail: detail,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = e.w.Write(append(data, '\n'))
	if f, ok := e.w.(*os.File); ok {
		_ = f.Sync()
	}
}

type controlledProbeRunner struct {
	db         *sql.DB
	scheduler  *queue.Scheduler
	runRepo    domain.ProbeRunRepository
	emitter    *lifecycleEventEmitter
	mode       string
	startRunCh <-chan string
	releaseCh  <-chan string
	activeRunM sync.Mutex
	activeRun  string
}

func (r *controlledProbeRunner) CurrentRunID() string {
	r.activeRunM.Lock()
	defer r.activeRunM.Unlock()
	return r.activeRun
}

func (r *controlledProbeRunner) setActiveRun(runID string) {
	r.activeRunM.Lock()
	r.activeRun = runID
	r.activeRunM.Unlock()
}

func (r *controlledProbeRunner) Run(ctx context.Context, run *domain.ProbeRun, nodeIDs []string, kinds []domain.ProbeKind) error {
	if run == nil {
		return errors.New("probe run is required")
	}
	runID := run.ID
	r.setActiveRun(runID)
	if r.startRunCh != nil {
		startID, ok := <-r.startRunCh
		if !ok || (startID != "" && startID != runID) {
			r.emitter.Emit("runner.start.mismatch", runID, fmt.Sprintf("startID=%q ok=%v", startID, ok))
			return fmt.Errorf("unexpected start barrier id %q for run %q", startID, runID)
		}
	}
	r.emitter.Emit("runner.run.entered", runID, strings.Join(func() []string {
		out := make([]string, 0, len(kinds))
		for _, k := range kinds {
			out = append(out, string(k))
		}
		return out
	}(), ","))

	if err := run.TransitionTo(domain.ProbeRunStateRunning); err == nil {
		_ = r.runRepo.UpdateState(ctx, runID, domain.ProbeRunStateRunning)
	}

	done := make(chan error, 1)
	task := queue.Task{
		ID:        "task-" + runID,
		RunID:     runID,
		LogicalID: "node-lifecycle-fixture",
		Kind:      domain.ProbeKindBaseline,
		Context:   ctx,
		Execute: func(tCtx context.Context) error {
			r.emitter.Emit("execute.entered", runID, "")
			return nil
		},
		OnComplete: func(execErr error) {
			r.emitter.Emit("callback.entered", runID, "")
			if r.mode == "graceful" {
				relRunID, ok := <-r.releaseCh
				if !ok {
					done <- errors.New("release pipe closed unexpectedly")
					return
				}
				r.emitter.Emit("callback.released", runID, relRunID)
				tx, err := r.db.Begin()
				if err != nil {
					r.emitter.Emit("callback.db.error", runID, err.Error())
					done <- err
					return
				}
				if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS lifecycle_callback (
					run_id TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					seq INTEGER NOT NULL
				)`); err != nil {
					_ = tx.Rollback()
					r.emitter.Emit("callback.db.error", runID, err.Error())
					done <- err
					return
				}
				if _, err := tx.Exec(`INSERT INTO lifecycle_callback(run_id, value, seq) VALUES (?, 'completed', 1)`, runID); err != nil {
					_ = tx.Rollback()
					r.emitter.Emit("callback.db.error", runID, err.Error())
					done <- err
					return
				}
				var verifyRunID, verifyValue string
				if err := tx.QueryRow(`SELECT run_id, value FROM lifecycle_callback WHERE run_id = ?`, runID).Scan(&verifyRunID, &verifyValue); err != nil {
					_ = tx.Rollback()
					r.emitter.Emit("callback.db.error", runID, err.Error())
					done <- err
					return
				}
				if err := tx.Commit(); err != nil {
					r.emitter.Emit("callback.db.error", runID, err.Error())
					done <- err
					return
				}
				var committedValue string
				if err := r.db.QueryRow(`SELECT value FROM lifecycle_callback WHERE run_id = ?`, runID).Scan(&committedValue); err != nil || committedValue != "completed" {
					r.emitter.Emit("callback.db.error", runID, fmt.Sprintf("readback err=%v val=%q", err, committedValue))
					done <- fmt.Errorf("readback failed: %w", err)
					return
				}
				r.emitter.Emit("callback.persisted", runID, committedValue)
				_ = r.runRepo.UpdateState(context.Background(), runID, domain.ProbeRunStateSucceeded)
				r.emitter.Emit("callback.returned", runID, "")
				done <- execErr
				return
			}
			// Timeout/stuck branch: remain uncooperative in OnComplete until natural drain timeout and process exit.
			select {}
		},
	}

	if err := r.scheduler.Submit(task); err != nil {
		r.emitter.Emit("scheduler.submit.error", runID, err.Error())
		return err
	}
	r.emitter.Emit("scheduler.submitted", runID, task.ID)

	return <-done
}

// TestHelperProcessServeLifecycle is an explicit child-process entrypoint. It
// invokes the same service startup, HTTP listener, signal handling, router and Drain
// path as production; only this helper process injects the controlled Runner and
// lifecycle event observers via package-private serveDependencies.
func TestHelperProcessServeLifecycle(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		return
	}
	mode := os.Getenv("CSP_TEST_LIFECYCLE_MODE")
	eventPipe := os.NewFile(3, "event-pipe")
	cmdPipe := os.NewFile(4, "cmd-pipe")
	emitter := &lifecycleEventEmitter{w: eventPipe}

	startRunCh := make(chan string, 1)
	releaseCh := make(chan string, 1)
	go func() {
		defer close(startRunCh)
		defer close(releaseCh)
		scanner := bufio.NewScanner(cmdPipe)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "start:") {
				startRunCh <- strings.TrimPrefix(line, "start:")
			} else if strings.HasPrefix(line, "release:") {
				releaseCh <- strings.TrimPrefix(line, "release:")
				return
			}
		}
	}()

	shutdownTimeout := 5 * time.Second
	if mode == "stuck" {
		shutdownTimeout = 350 * time.Millisecond
	}

	var runner *controlledProbeRunner
	currentRunID := func() string {
		if runner == nil {
			return ""
		}
		return runner.CurrentRunID()
	}

	deps := &serveDependencies{
		shutdownTimeout: shutdownTimeout,
		newProbeRunner: func(db *sql.DB, _ domain.NodeRepository, _ domain.ProbeObservationRepository, scheduler *queue.Scheduler, runRepo domain.ProbeRunRepository, _ ...probe.DefaultRunnerOption) probe.Runner {
			runner = &controlledProbeRunner{
				db:         db,
				scheduler:  scheduler,
				runRepo:    runRepo,
				emitter:    emitter,
				mode:       mode,
				startRunCh: startRunCh,
				releaseCh:  releaseCh,
			}
			return runner
		},
		observeDrainBegin: func() {
			emitter.Emit("drain.begin", currentRunID(), "")
		},
		observeDrainDecision: func(drainErr error) {
			if drainErr != nil {
				emitter.Emit("drain.timeout", currentRunID(), drainErr.Error())
			} else {
				emitter.Emit("drain.ok", currentRunID(), "")
			}
		},
		observeDBClose: func(stage string, err error) {
			detail := ""
			if err != nil {
				detail = err.Error()
			}
			if runner != nil && runner.db != nil {
				switch stage {
				case "db.close.ok":
					if pingErr := runner.db.Ping(); pingErr != nil {
						detail = "db_closed=true:" + pingErr.Error()
					} else {
						detail = "db_closed=false"
					}
				case "db.close.skipped.drain_timeout":
					var state string
					qErr := runner.db.QueryRow(`SELECT state FROM probe_runs WHERE id = ?`, currentRunID()).Scan(&state)
					if qErr == nil {
						detail = fmt.Sprintf("db_open=true,state=%s,err=%v", state, err)
					} else {
						detail = fmt.Sprintf("db_open=false,query_err=%v,err=%v", qErr, err)
					}
				}
			}
			emitter.Emit(stage, currentRunID(), detail)
		},
		observeCleanExit: func() {
			emitter.Emit("clean", currentRunID(), "CSP control plane stopped cleanly")
		},
	}

	code := runServeWithDependencies(
		context.Background(),
		[]string{
			"-addr", os.Getenv("CSP_ADDR"),
			"-db", os.Getenv("CSP_DB_PATH"),
			"-admin-token", "lifecycle-test-token",
		},
		os.Stdout,
		os.Stderr,
		deps,
	)
	if eventPipe != nil {
		_ = eventPipe.Close()
	}
	if cmdPipe != nil {
		_ = cmdPipe.Close()
	}
	os.Exit(code)
}

type lifecycleChild struct {
	cmd       *exec.Cmd
	output    *synchronizedBuffer
	wait      chan error
	dbPath    string
	addr      string
	mode      string
	runID     string
	cmdWriter *os.File
	eventsCh  chan lifecycleEvent
	eventsEOF chan struct{}
	mu        sync.Mutex
	events    []lifecycleEvent
}

type synchronizedBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *synchronizedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func (c *lifecycleChild) recordEvent(ev lifecycleEvent) {
	c.mu.Lock()
	c.events = append(c.events, ev)
	c.mu.Unlock()
}

func (c *lifecycleChild) snapshotEvents() []lifecycleEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]lifecycleEvent, len(c.events))
	copy(out, c.events)
	return out
}

func (c *lifecycleChild) waitForEvent(t *testing.T, name string, timeout time.Duration) lifecycleEvent {
	t.Helper()
	for _, ev := range c.snapshotEvents() {
		if ev.Event == name {
			return ev
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-c.eventsCh:
			if !ok {
				t.Fatalf("event pipe closed before receiving %q; recorded=%v output=%s", name, c.snapshotEvents(), c.output.String())
			}
			if ev.Event == name {
				return ev
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for event %q; recorded=%v output=%s", name, c.snapshotEvents(), c.output.String())
		}
	}
}

func startLifecycleChild(t *testing.T, mode string) *lifecycleChild {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "lifecycle.db")

	eventReader, eventWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmdReader, cmdWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcessServeLifecycle$")
	cmd.Env = append(
		os.Environ(),
		helperEnv+"=1",
		"CSP_TEST_LIFECYCLE_MODE="+mode,
		"CSP_ADDR="+addr,
		"CSP_DB_PATH="+dbPath,
		"CSP_ADMIN_TOKEN=lifecycle-test-token",
	)
	cmd.ExtraFiles = []*os.File{eventWriter, cmdReader}
	out := &synchronizedBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Close child-side descriptors in parent so EOF is strictly determined by child exit.
	_ = eventWriter.Close()
	_ = cmdReader.Close()

	child := &lifecycleChild{
		cmd:       cmd,
		output:    out,
		wait:      make(chan error, 1),
		dbPath:    dbPath,
		addr:      addr,
		mode:      mode,
		cmdWriter: cmdWriter,
		eventsCh:  make(chan lifecycleEvent, 64),
		eventsEOF: make(chan struct{}),
	}

	go func() {
		defer close(child.eventsEOF)
		defer close(child.eventsCh)
		defer eventReader.Close()
		scanner := bufio.NewScanner(eventReader)
		for scanner.Scan() {
			var ev lifecycleEvent
			if err := json.Unmarshal(scanner.Bytes(), &ev); err == nil {
				child.recordEvent(ev)
				child.eventsCh <- ev
			}
		}
	}()

	go func() {
		child.wait <- cmd.Wait()
	}()

	base := "http://" + addr
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/healthz")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case e := <-child.wait:
			t.Fatalf("child exited before HTTP ready: %v; output=%s", e, out.String())
		default:
		}
		time.Sleep(15 * time.Millisecond)
	}

	resp, err := client.Get(base + "/readyz")
	if err != nil {
		t.Fatalf("readyz request: %v; output=%s", err, out.String())
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz status=%d output=%s", resp.StatusCode, out.String())
	}

	// 1. Verify unauthorized POST is rejected with 401 and enqueues no task.
	unauthReq, err := http.NewRequest(http.MethodPost, base+"/api/v1/probes/runs", bytes.NewBufferString(`{"kinds":["baseline"]}`))
	if err != nil {
		t.Fatal(err)
	}
	unauthReq.Header.Set("Content-Type", "application/json")
	unauthReq.Header.Set("Idempotency-Key", "unauth-key-"+mode)
	unauthResp, err := client.Do(unauthReq)
	if err != nil {
		t.Fatalf("unauthorized POST failed: %v", err)
	}
	_, _ = io.Copy(io.Discard, unauthResp.Body)
	unauthResp.Body.Close()
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthorized POST, got %d", unauthResp.StatusCode)
	}
	if got := len(child.snapshotEvents()); got != 0 {
		t.Fatalf("unauthorized POST triggered lifecycle events: %v", child.snapshotEvents())
	}

	// 2. Send authenticated POST /api/v1/probes/runs with unique Idempotency-Key.
	idemKey := fmt.Sprintf("lifecycle-%s-%d", mode, time.Now().UnixNano())
	createReq, err := http.NewRequest(http.MethodPost, base+"/api/v1/probes/runs", bytes.NewBufferString(`{"kinds":["baseline"]}`))
	if err != nil {
		t.Fatal(err)
	}
	createReq.Header.Set("Authorization", "Bearer lifecycle-test-token")
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Idempotency-Key", idemKey)

	createResp, err := client.Do(createReq)
	if err != nil {
		t.Fatalf("authenticated POST /api/v1/probes/runs failed: %v; output=%s", err, out.String())
	}
	var createPayload struct {
		Data struct {
			RunID string `json:"run_id"`
			State string `json:"state"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createPayload); err != nil {
		createResp.Body.Close()
		t.Fatalf("decode 201 response: %v", err)
	}
	createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d; output=%s", createResp.StatusCode, out.String())
	}
	runID := strings.TrimSpace(createPayload.Data.RunID)
	if runID == "" {
		t.Fatalf("201 response missing run_id; payload=%+v", createPayload)
	}
	child.runID = runID
	child.recordEvent(lifecycleEvent{
		Seq:    0,
		Event:  "http.201.created",
		RunID:  runID,
		Detail: idemKey,
	})
	if _, err := fmt.Fprintf(child.cmdWriter, "start:%s\n", runID); err != nil {
		t.Fatalf("write start command over pipe: %v", err)
	}

	// 3. Wait on bounded synchronization pipe for callback.entered with matching runID.
	enteredEv := child.waitForEvent(t, "callback.entered", 5*time.Second)
	if enteredEv.RunID != runID {
		t.Fatalf("callback.entered run_id=%q, want HTTP 201 run_id=%q", enteredEv.RunID, runID)
	}

	// 4. Verify duplicate Idempotency-Key returns same run_id without creating a second task.
	dupReq, err := http.NewRequest(http.MethodPost, base+"/api/v1/probes/runs", bytes.NewBufferString(`{"kinds":["baseline"]}`))
	if err != nil {
		t.Fatal(err)
	}
	dupReq.Header.Set("Authorization", "Bearer lifecycle-test-token")
	dupReq.Header.Set("Content-Type", "application/json")
	dupReq.Header.Set("Idempotency-Key", idemKey)
	dupResp, err := client.Do(dupReq)
	if err != nil {
		t.Fatalf("duplicate key POST failed: %v", err)
	}
	var dupPayload struct {
		Data struct {
			RunID string `json:"run_id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(dupResp.Body).Decode(&dupPayload)
	dupResp.Body.Close()
	if dupResp.StatusCode != http.StatusCreated || dupPayload.Data.RunID != runID {
		t.Fatalf("duplicate Idempotency-Key status=%d run_id=%q, want 201 and %q", dupResp.StatusCode, dupPayload.Data.RunID, runID)
	}
	var runnerCount int
	for _, ev := range child.snapshotEvents() {
		if ev.Event == "runner.run.entered" {
			runnerCount++
		}
	}
	if runnerCount != 1 {
		t.Fatalf("duplicate Idempotency-Key triggered %d runner runs, want 1", runnerCount)
	}

	// 5. Verify probe_runs row exists in SQLite with the exact runID.
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	var dbRunID, dbIdemKey string
	if err := db.QueryRow(`SELECT id, idempotency_key FROM probe_runs WHERE id = ?`, runID).Scan(&dbRunID, &dbIdemKey); err != nil {
		_ = db.Close()
		t.Fatalf("probe_runs row for %q missing: %v", runID, err)
	}
	_ = db.Close()
	if dbRunID != runID || dbIdemKey != idemKey {
		t.Fatalf("probe_runs row mismatch: id=%q key=%q", dbRunID, dbIdemKey)
	}

	client.CloseIdleConnections()
	return child
}

func eventIndex(events []lifecycleEvent, name string) int {
	for i, ev := range events {
		if ev.Event == name {
			return i
		}
	}
	return -1
}

type causalEdge struct {
	from string
	to   string
}

func verifyCausalLifecycleEvents(
	t *testing.T,
	events []lifecycleEvent,
	runID string,
	requiredEvents []string,
	edges []causalEdge,
) {
	t.Helper()
	eventMap := make(map[string]lifecycleEvent, len(events))
	indexMap := make(map[string]int, len(events))
	occurrences := make(map[string]int, len(events))

	for i, ev := range events {
		occurrences[ev.Event]++
		if _, exists := indexMap[ev.Event]; !exists {
			indexMap[ev.Event] = i
			eventMap[ev.Event] = ev
		}
	}

	for _, name := range requiredEvents {
		_, found := indexMap[name]
		if !found {
			t.Fatalf("missing required event %q in sequence %v", name, events)
		}
		ev := eventMap[name]
		if ev.RunID != runID {
			t.Fatalf("event %q has run_id=%q, want %q", name, ev.RunID, runID)
		}
		if occurrences[name] > 1 {
			t.Fatalf("expected unique event %q, but appeared %d times; events=%v", name, occurrences[name], events)
		}
	}

	for _, edge := range edges {
		idxFrom, foundFrom := indexMap[edge.from]
		if !foundFrom {
			t.Fatalf("causal prerequisite event %q not found in events=%v", edge.from, events)
		}
		idxTo, foundTo := indexMap[edge.to]
		if !foundTo {
			t.Fatalf("causal consequence event %q not found in events=%v", edge.to, events)
		}
		if idxFrom >= idxTo {
			t.Fatalf("causal order violation: event %q (index %d) must occur before event %q (index %d); events=%v",
				edge.from, idxFrom, edge.to, idxTo, events)
		}
		evFrom := eventMap[edge.from]
		evTo := eventMap[edge.to]
		if evFrom.Seq > 0 && evTo.Seq > 0 && evFrom.Seq >= evTo.Seq {
			t.Fatalf("child sequence monotonic violation: event %q (seq %d) must precede event %q (seq %d); events=%v",
				edge.from, evFrom.Seq, edge.to, evTo.Seq, events)
		}
	}
}

func TestServeLifecycleSubprocessGracefulDrainPersistsCallbackBeforeCleanExit(t *testing.T) {
	child := startLifecycleChild(t, "graceful")
	defer func() {
		if child.cmdWriter != nil {
			_ = child.cmdWriter.Close()
		}
	}()

	// Send SIGTERM after callback.entered has been proven via pipe barrier.
	child.recordEvent(lifecycleEvent{
		Seq:   0,
		Event: "parent.sigterm.sent",
		RunID: child.runID,
	})
	if err := child.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	// Wait until child enters drain before releasing blocked callback.
	drainBeginEv := child.waitForEvent(t, "drain.begin", 5*time.Second)
	if drainBeginEv.RunID != child.runID {
		t.Fatalf("drain.begin run_id=%q, want %q", drainBeginEv.RunID, child.runID)
	}

	child.recordEvent(lifecycleEvent{
		Seq:   0,
		Event: "parent.release.sent",
		RunID: child.runID,
	})
	if _, err := fmt.Fprintf(child.cmdWriter, "release:%s\n", child.runID); err != nil {
		t.Fatalf("write release command over pipe: %v", err)
	}
	_ = child.cmdWriter.Close()
	child.cmdWriter = nil

	var waitErr error
	select {
	case waitErr = <-child.wait:
	case <-time.After(10 * time.Second):
		_ = child.cmd.Process.Kill()
		t.Fatalf("graceful child did not exit within timeout; events=%v output=%s", child.snapshotEvents(), child.output.String())
	}
	if waitErr != nil {
		t.Fatalf("expected graceful child exit 0, got err=%v; events=%v output=%s", waitErr, child.snapshotEvents(), child.output.String())
	}

	select {
	case <-child.eventsEOF:
	case <-time.After(2 * time.Second):
		t.Fatalf("event pipe did not reach EOF after child exit")
	}

	child.recordEvent(lifecycleEvent{
		Seq:    0,
		Event:  "process.exit.0",
		RunID:  child.runID,
		Detail: "exit_code=0",
	})

	events := child.snapshotEvents()
	requiredEvents := []string{
		"http.201.created",
		"runner.run.entered",
		"scheduler.submitted",
		"execute.entered",
		"callback.entered",
		"parent.sigterm.sent",
		"drain.begin",
		"parent.release.sent",
		"callback.released",
		"callback.persisted",
		"callback.returned",
		"drain.ok",
		"db.close.attempt",
		"db.close.ok",
		"clean",
		"process.exit.0",
	}

	causalEdges := []causalEdge{
		{from: "http.201.created", to: "runner.run.entered"},
		{from: "runner.run.entered", to: "scheduler.submitted"},
		{from: "runner.run.entered", to: "execute.entered"},
		{from: "execute.entered", to: "callback.entered"},
		{from: "scheduler.submitted", to: "parent.sigterm.sent"},
		{from: "callback.entered", to: "parent.sigterm.sent"},
		{from: "parent.sigterm.sent", to: "drain.begin"},
		{from: "drain.begin", to: "parent.release.sent"},
		{from: "parent.release.sent", to: "callback.released"},
		{from: "callback.released", to: "callback.persisted"},
		{from: "callback.persisted", to: "callback.returned"},
		{from: "drain.begin", to: "drain.ok"},
		{from: "callback.returned", to: "drain.ok"},
		{from: "drain.ok", to: "db.close.attempt"},
		{from: "db.close.attempt", to: "db.close.ok"},
		{from: "db.close.ok", to: "clean"},
		{from: "clean", to: "process.exit.0"},
	}

	verifyCausalLifecycleEvents(t, events, child.runID, requiredEvents, causalEdges)

	dbCloseOkEv := events[eventIndex(events, "db.close.ok")]
	if !strings.HasPrefix(dbCloseOkEv.Detail, "db_closed=true:") {
		t.Fatalf("expected db.close.ok to confirm closed *sql.DB handle, got detail=%q", dbCloseOkEv.Detail)
	}

	if !strings.Contains(child.output.String(), "CSP control plane stopped cleanly") {
		t.Fatalf("clean shutdown log missing from stdout: %s", child.output.String())
	}

	db, err := sql.Open("sqlite", child.dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var persistedRunID, persistedVal, runState string
	if err := db.QueryRow(`SELECT run_id, value FROM lifecycle_callback WHERE run_id = ?`, child.runID).Scan(&persistedRunID, &persistedVal); err != nil {
		t.Fatalf("query lifecycle_callback for run_id=%s: %v", child.runID, err)
	}
	if err := db.QueryRow(`SELECT state FROM probe_runs WHERE id = ?`, child.runID).Scan(&runState); err != nil {
		t.Fatalf("query probe_runs state for run_id=%s: %v", child.runID, err)
	}
	if persistedRunID != child.runID || persistedVal != "completed" || runState != string(domain.ProbeRunStateSucceeded) {
		t.Fatalf("unexpected DB state: callback=(%q,%q) runState=%q", persistedRunID, persistedVal, runState)
	}
	t.Logf("GRACEFUL_EVIDENCE run_id=%s exit_code=0 db_row=(%s,%s,state=%s) events=%v", child.runID, persistedRunID, persistedVal, runState, events)
}

func TestServeLifecycleSubprocessUncooperativeTaskTimesOutWithoutCleanExit(t *testing.T) {
	child := startLifecycleChild(t, "stuck")
	defer func() {
		if child.cmdWriter != nil {
			_ = child.cmdWriter.Close()
		}
	}()

	// Send SIGTERM while callback is blocked in flight; never send release.
	child.recordEvent(lifecycleEvent{
		Seq:   0,
		Event: "parent.sigterm.sent",
		RunID: child.runID,
	})
	if err := child.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}

	var waitErr error
	select {
	case waitErr = <-child.wait:
	case <-time.After(10 * time.Second):
		_ = child.cmd.Process.Kill()
		t.Fatalf("stuck child did not exit naturally within timeout (watchdog kill is not acceptable); events=%v output=%s", child.snapshotEvents(), child.output.String())
	}
	if waitErr == nil {
		t.Fatalf("expected nonzero child exit on drain timeout; output=%s", child.output.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T (%v)", waitErr, waitErr)
	}
	exitCode := exitErr.ExitCode()
	if exitCode <= 0 {
		t.Fatalf("expected positive nonzero exit code from natural return (not signal kill), got %d (status=%v)", exitCode, exitErr.ProcessState)
	}

	select {
	case <-child.eventsEOF:
	case <-time.After(2 * time.Second):
		t.Fatalf("event pipe did not reach EOF after child exit")
	}

	child.recordEvent(lifecycleEvent{
		Seq:    0,
		Event:  "process.exit.nonzero",
		RunID:  child.runID,
		Detail: fmt.Sprintf("exit_code=%d", exitCode),
	})

	events := child.snapshotEvents()
	requiredEvents := []string{
		"http.201.created",
		"runner.run.entered",
		"scheduler.submitted",
		"execute.entered",
		"callback.entered",
		"parent.sigterm.sent",
		"drain.begin",
		"drain.timeout",
		"db.close.skipped.drain_timeout",
		"shutdown.failure.final",
		"process.exit.nonzero",
	}

	causalEdges := []causalEdge{
		{from: "http.201.created", to: "runner.run.entered"},
		{from: "runner.run.entered", to: "scheduler.submitted"},
		{from: "runner.run.entered", to: "execute.entered"},
		{from: "execute.entered", to: "callback.entered"},
		{from: "scheduler.submitted", to: "parent.sigterm.sent"},
		{from: "callback.entered", to: "parent.sigterm.sent"},
		{from: "parent.sigterm.sent", to: "drain.begin"},
		{from: "drain.begin", to: "drain.timeout"},
		{from: "drain.timeout", to: "db.close.skipped.drain_timeout"},
		{from: "db.close.skipped.drain_timeout", to: "shutdown.failure.final"},
		{from: "shutdown.failure.final", to: "process.exit.nonzero"},
	}

	verifyCausalLifecycleEvents(t, events, child.runID, requiredEvents, causalEdges)

	forbiddenEvents := []string{
		"db.close.attempt",
		"db.close.ok",
		"db.close.error",
		"callback.persisted",
		"clean",
	}
	for _, forbidden := range forbiddenEvents {
		if idx := eventIndex(events, forbidden); idx != -1 {
			t.Fatalf("forbidden event %q occurred at index %d on timeout path; events=%v", forbidden, idx, events)
		}
	}

	dbSkippedEv := events[eventIndex(events, "db.close.skipped.drain_timeout")]
	if !strings.Contains(dbSkippedEv.Detail, "db_open=true,state=running") {
		t.Fatalf("expected db.close.skipped.drain_timeout to prove in-process DB remains open at drain deadline, got detail=%q", dbSkippedEv.Detail)
	}

	if strings.Contains(child.output.String(), "CSP control plane stopped cleanly") {
		t.Fatalf("unexpected clean shutdown log on timeout path: %s", child.output.String())
	}
	if !strings.Contains(child.output.String(), "drain incomplete; terminating without closing database") {
		t.Fatalf("missing drain timeout diagnostic: %s", child.output.String())
	}

	// Verify post-exit SQLite corroboration.
	db, err := sql.Open("sqlite", child.dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var exists int
	_ = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='lifecycle_callback'").Scan(&exists)
	if exists != 0 {
		t.Fatal("callback database side effect happened despite unreleased worker")
	}
	var runState string
	if err := db.QueryRow(`SELECT state FROM probe_runs WHERE id = ?`, child.runID).Scan(&runState); err != nil {
		t.Fatalf("query probe_runs row for run_id=%s: %v", child.runID, err)
	}
	t.Logf("TIMEOUT_EVIDENCE run_id=%s exit_code=%d db_row=(id=%s,state=%s,callback_table=%d) events=%v", child.runID, exitCode, child.runID, runState, exists, events)
}

func TestServeShutdownCloseFailureReturnsNonZeroWithoutCleanLog(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	dbPath := filepath.Join(t.TempDir(), "close_fail.db")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var stdout, stderr synchronizedBuffer
	var stagesMu sync.Mutex
	var stages []string
	done := make(chan int, 1)

	go func() {
		done <- runServeWithDependencies(ctx, []string{"-addr", addr, "-db", dbPath, "-admin-token", "close-fail-token"}, &stdout, &stderr, &serveDependencies{
			closeDBFn: func(db *sql.DB) error {
				_ = db.Close()
				return errors.New("simulated sqlite close failure")
			},
			observeDBClose: func(stage string, _ error) {
				stagesMu.Lock()
				stages = append(stages, stage)
				stagesMu.Unlock()
			},
			observeCleanExit: func() {
				stagesMu.Lock()
				stages = append(stages, "clean")
				stagesMu.Unlock()
			},
		})
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
	}

	cancel()
	code := <-done
	if code == 0 {
		t.Fatalf("expected non-zero exit code when DB.Close fails, got 0; stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), "CSP control plane stopped cleanly") {
		t.Fatalf("unexpected clean shutdown log on DB.Close failure: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "database close failed") {
		t.Fatalf("expected database close failure diagnostic on stderr, got: %s", stderr.String())
	}
	stagesMu.Lock()
	stagesCopy := append([]string(nil), stages...)
	stagesMu.Unlock()
	for _, s := range stagesCopy {
		if s == "clean" || s == "db.close.ok" {
			t.Fatalf("unexpected stage %q on DB.Close failure: %v", s, stagesCopy)
		}
	}
}
