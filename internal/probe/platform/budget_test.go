package platform

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBodyBudgetConcurrentResponsesRefundAndCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, strings.Repeat("x", 4096))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	budget := NewBodyBudget(10001)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder := &ResponseRecorder{}
			client := BudgetClient(server.Client(), ctx, budget, recorder, nil)
			resp, err := client.Get(server.URL)
			if err == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	limit, used := budget.Snapshot()
	if used != limit {
		t.Fatalf("consumed=%d limit=%d", used, limit)
	}
	budget.mu.Lock()
	reserved := budget.reserved
	budget.mu.Unlock()
	if reserved != 0 {
		t.Fatalf("leaked reservations: %d", reserved)
	}
	// A short EOF read must refund all unused allocation.
	b := NewBodyBudget(100)
	n, err := b.reserve(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	b.settle(n, 3)
	n, err = b.reserve(ctx, 100)
	if err != nil || n != 97 {
		t.Fatalf("refund reservation %d %v", n, err)
	}
	b.settle(n, 0)
	cancelled, c := context.WithCancel(ctx)
	c()
	if _, err = b.reserve(cancelled, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled reserve: %v", err)
	}
}

func TestBodyBudgetCancelsInflightAndBlocksNewRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "abcdef") }))
	defer server.Close()
	recorder := &ResponseRecorder{}
	budget := NewBodyBudget(3)
	client := BudgetClient(server.Client(), ctx, budget, recorder, cancel)
	resp, err := client.Get(server.URL + "/sub/private?token=secret")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if len(data) != 3 || !errors.Is(err, ErrBodyBudget) {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("allocation exhaustion did not cancel")
	}
	rows, err := recorder.Snapshot()
	if !errors.Is(err, ErrBodyBudget) || len(rows) != 1 || rows[0].Bytes != 3 || strings.Contains(rows[0].TargetDigest, "secret") {
		t.Fatalf("unsafe or incomplete evidence: %+v %v", rows, err)
	}
	if _, err = client.Get(server.URL); err == nil {
		t.Fatal("request after budget exhaustion accepted")
	}
}

func TestAny2xxAliveWithRealLoopbackBodies(t *testing.T) {
	for _, status := range []int{200, 204, 299, 403, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				if status != 204 {
					_, _ = io.WriteString(w, "nonempty captcha login")
				}
			}))
			defer server.Close()
			recorder := &ResponseRecorder{}
			budget := NewBodyBudget(1000)
			client := BudgetClient(server.Client(), context.Background(), budget, recorder, nil)
			ok, result := CheckAlive(context.Background(), client, server.URL)
			if ok != (status >= 200 && status < 300) {
				t.Fatalf("status=%d result=%+v", status, result)
			}
		})
	}
}
