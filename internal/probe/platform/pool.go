package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"time"
)

var bodyBufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 64<<10))
	},
}

func getPooledBuf() *bytes.Buffer {
	buf := bodyBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putPooledBuf(buf *bytes.Buffer) {
	if buf == nil || buf.Cap() > 4<<20 {
		return
	}
	bodyBufPool.Put(buf)
}

// ErrBodyBudget is a typed, run-wide application response-body limit. It does
// not measure TLS, HTTP headers, socket prefetch, or NIC traffic.
var ErrBodyBudget = errors.New("probe response body budget exhausted")

// BodyBudget reserves before every read, settles actual consumption, and refunds
// unused reservations. One instance may be shared by all stages and AB/BA sides.
type BodyBudget struct {
	mu                    sync.Mutex
	limit, used, reserved int64
	changed               chan struct{}
	ledger                *ledgerClient
}

// NewLedgerBodyBudget keeps node-local fairness and reserves every actual read
// against the experiment's transactionally shared SQLite ledger via loopback IPC.
func NewLedgerBodyBudget(limit int64, endpoint, token, scope string) (*BodyBudget, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Path != "" || token == "" || scope == "" || limit <= 0 {
		return nil, errors.New("invalid ledger endpoint")
	}
	b := NewBodyBudget(limit)
	b.ledger = &ledgerClient{endpoint: endpoint, token: token, scope: scope, client: &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	return b, nil
}

type ledgerClient struct {
	endpoint, token, scope string
	client                 *http.Client
}

func (l *ledgerClient) call(ctx context.Context, path string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoint+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+l.token)
	resp, err := l.client.Do(req)
	if err != nil {
		return errors.New("ledger unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ErrBodyBudget
	}
	if output != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(output)
	}
	return nil
}

func NewBodyBudget(limit int64) *BodyBudget {
	return &BodyBudget{limit: limit, changed: make(chan struct{})}
}

func (b *BodyBudget) Snapshot() (limit, used int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.limit, b.used
}

func (b *BodyBudget) reserve(ctx context.Context, n int64) (int64, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		b.mu.Lock()
		free := b.limit - b.used - b.reserved
		if free > 0 {
			if n > free {
				n = free
			}
			b.reserved += n
			b.mu.Unlock()
			return n, nil
		}
		if b.reserved == 0 {
			b.mu.Unlock()
			return 0, ErrBodyBudget
		}
		ch := b.changed
		b.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

func (b *BodyBudget) settle(reserved, used int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reserved -= reserved
	b.used += used
	close(b.changed)
	b.changed = make(chan struct{})
}

// ResponseEvidence contains no raw URLs, credentials, headers, or response body.
type ResponseEvidence struct {
	TargetDigest string `json:"target_digest"`
	Status       int    `json:"status"`
	Bytes        int64  `json:"body_bytes"`
	Reason       string `json:"reason,omitempty"`
}

type ResponseRecorder struct {
	mu   sync.Mutex
	rows []*ResponseEvidence
	err  error
}

func (r *ResponseRecorder) Snapshot() ([]ResponseEvidence, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := make([]ResponseEvidence, len(r.rows))
	for i, v := range r.rows {
		rows[i] = *v
	}
	return rows, r.err
}

// ResponseReason classifies wrapped typed errors without exposing addresses or secrets.
func ResponseReason(err error) string {
	var dns *net.DNSError
	var hostname x509.HostnameError
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var network net.Error
	switch {
	case errors.Is(err, ErrBodyBudget):
		return "budget_not_executed"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.As(err, &dns):
		return "dns_error"
	case errors.As(err, &hostname), errors.As(err, &unknown), errors.As(err, &invalid):
		return "tls_certificate_error"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.As(err, &network) && network.Timeout():
		return "timeout"
	case err != nil:
		return "transport_error"
	default:
		return ""
	}
}

// BudgetClient clones the client without mutating shared session transports.
// Every response, including redirects and unsuccessful statuses, is metered.
func BudgetClient(client *http.Client, ctx context.Context, budget *BodyBudget, recorder *ResponseRecorder, exhausted context.CancelFunc) *http.Client {
	copy := *client
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = &budgetTransport{base: base, ctx: ctx, budget: budget, recorder: recorder, exhausted: exhausted}
	return &copy
}

type budgetTransport struct {
	base      http.RoundTripper
	ctx       context.Context
	budget    *BodyBudget
	recorder  *ResponseRecorder
	exhausted context.CancelFunc
}

func (t *budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(t.ctx, cancel)
	h := sha256.Sum256([]byte(req.URL.String()))
	row := &ResponseEvidence{TargetDigest: "sha256:" + hex.EncodeToString(h[:])}
	t.recorder.mu.Lock()
	t.recorder.rows = append(t.recorder.rows, row)
	t.recorder.mu.Unlock()
	_, used := t.budget.Snapshot()
	limit, _ := t.budget.Snapshot()
	var resp *http.Response
	var err error
	if used >= limit {
		err = ErrBodyBudget
	} else {
		resp, err = t.base.RoundTrip(req.WithContext(ctx))
	}
	if err != nil {
		t.recorder.mu.Lock()
		row.Reason = responseReason(err)
		t.recorder.err = err
		t.recorder.mu.Unlock()
		stop()
		cancel()
		if errors.Is(err, ErrBodyBudget) && t.exhausted != nil {
			t.exhausted()
		}
		return nil, err
	}
	t.recorder.mu.Lock()
	row.Status = resp.StatusCode
	t.recorder.mu.Unlock()
	resp.Body = &budgetBody{ReadCloser: resp.Body, ctx: ctx, budget: t.budget, recorder: t.recorder, row: row, stop: stop, cancel: cancel, exhausted: t.exhausted}
	return resp, nil
}

func (t *budgetTransport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

type budgetBody struct {
	io.ReadCloser
	ctx       context.Context
	budget    *BodyBudget
	recorder  *ResponseRecorder
	row       *ResponseEvidence
	stop      func() bool
	cancel    context.CancelFunc
	exhausted context.CancelFunc
	once      sync.Once
}

func (b *budgetBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	reserved, err := b.budget.reserve(b.ctx, int64(len(p)))
	var n int
	if err == nil {
		var ticket struct {
			ID     string `json:"id"`
			Amount int64  `json:"amount"`
		}
		if b.budget.ledger != nil {
			err = b.budget.ledger.call(b.ctx, "/reserve", map[string]any{"scope": b.budget.ledger.scope, "amount": reserved}, &ticket)
			if err == nil && (ticket.Amount <= 0 || ticket.Amount > reserved || ticket.ID == "") {
				err = errors.New("invalid ledger reservation")
			}
		}
		if err == nil {
			amount := reserved
			if b.budget.ledger != nil {
				amount = ticket.Amount
			}
			n, err = b.ReadCloser.Read(p[:amount])
			if b.budget.ledger != nil {
				settleCtx, cancel := context.WithTimeout(context.WithoutCancel(b.ctx), 2*time.Second)
				settleErr := b.budget.ledger.call(settleCtx, "/settle", map[string]any{"id": ticket.ID, "used": n}, nil)
				cancel()
				if settleErr != nil {
					err = settleErr
				} // server conservatively keeps missing settlements charged
			}
		}
		b.budget.settle(reserved, int64(n))
	}
	b.recorder.mu.Lock()
	b.row.Bytes += int64(n)
	if err != nil && !errors.Is(err, io.EOF) {
		b.row.Reason = responseReason(err)
		b.recorder.err = err
	}
	b.recorder.mu.Unlock()
	if errors.Is(err, ErrBodyBudget) && b.exhausted != nil {
		b.exhausted()
	}
	return n, err
}

func (b *budgetBody) Close() error {
	b.once.Do(func() { b.stop(); b.cancel() })
	return b.ReadCloser.Close()
}

func responseReason(err error) string { return ResponseReason(err) }

func readJSONPooled(r io.Reader, v any) error {
	buf := getPooledBuf()
	defer putPooledBuf(buf)
	if _, err := buf.ReadFrom(r); err != nil {
		return err
	}
	return json.Unmarshal(buf.Bytes(), v)
}
