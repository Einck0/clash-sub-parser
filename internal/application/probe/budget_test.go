package probe

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type countedBody struct {
	io.Reader
	read   int64
	closed atomic.Bool
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += int64(n)
	return n, err
}
func (b *countedBody) Close() error { b.closed.Store(true); return nil }

type trackedHTTPBody struct {
	io.ReadCloser
	closed bool
}

func (b *trackedHTTPBody) Close() error { b.closed = true; return b.ReadCloser.Close() }

func TestReadBoundedResponsePerTaskLimit(t *testing.T) {
	const limit = 8
	for _, tc := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "limit minus one", body: strings.Repeat("a", limit-1)},
		{name: "exact limit", body: strings.Repeat("b", limit)},
		{name: "limit plus one", body: strings.Repeat("c", limit+1), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, tc.body) }))
			defer server.Close()
			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			tracked := &trackedHTTPBody{ReadCloser: resp.Body}
			body, readErr := readBoundedResponse(tracked, limit)
			closeErr := tracked.Close()
			if !tracked.closed {
				t.Fatal("response body was not closed")
			}
			if (readErr != nil) != tc.wantErr {
				t.Fatalf("read error = %v, wantErr %v", readErr, tc.wantErr)
			}
			if len(body) > limit {
				t.Fatalf("read %d bytes, limit %d", len(body), limit)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
		})
	}
}

func TestReadBoundedResponseReadsAtMostLimitPlusOneAndClosesBody(t *testing.T) {
	body := &countedBody{Reader: strings.NewReader(strings.Repeat("x", 100))}
	_, err := readBoundedResponse(body, 7)
	if !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("error = %v", err)
	}
	if body.read != 8 {
		t.Fatalf("read %d bytes; want limit+1", body.read)
	}
	if body.closed.Load() {
		t.Fatal("reader should be closed by response owner, not helper")
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if !body.closed.Load() {
		t.Fatal("body was not closed")
	}
}

type contextReader struct{ ctx context.Context }

func (r contextReader) Read([]byte) (int, error) { <-r.ctx.Done(); return 0, r.ctx.Err() }

func TestReadBoundedResponseStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { _, err := readBoundedResponse(contextReader{ctx: ctx}, 64); finished <- err }()
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("read error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("response read did not stop after cancellation")
	}
}
