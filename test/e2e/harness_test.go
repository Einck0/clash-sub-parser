package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"clash-sub-parser/internal/application/inventory"
	"clash-sub-parser/internal/application/iprisk"
	"clash-sub-parser/internal/application/policy"
	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/application/revision"
	"clash-sub-parser/internal/application/subscription"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/probe/queue"
	"clash-sub-parser/internal/repository/sqlite"
	transporthttp "clash-sub-parser/internal/transport/http"
	"clash-sub-parser/migrations"
)

type memoryFetcher struct {
	responses map[string]*fetch.Response
	errors    map[string]error
}

func newMemoryFetcher() *memoryFetcher {
	return &memoryFetcher{
		responses: make(map[string]*fetch.Response),
		errors:    make(map[string]error),
	}
}

func (f *memoryFetcher) setResponse(url string, resp *fetch.Response) {
	f.responses[url] = resp
}

func (f *memoryFetcher) Fetch(_ context.Context, opts fetch.Options) (*fetch.Response, error) {
	if err, ok := f.errors[opts.URL]; ok && err != nil {
		return nil, err
	}
	if resp, ok := f.responses[opts.URL]; ok {
		return resp, nil
	}
	// Default mock response
	return &fetch.Response{
		StatusCode:    200,
		ContentType:   "text/yaml",
		ContentDigest: "mock-digest-default",
		Body: []byte(`proxies:
  - name: "Harness Default SS"
    type: ss
    server: 198.51.100.99
    port: 8388
    cipher: aes-128-gcm
    password: default-pass
`),
	}, nil
}

type TestHarness struct {
	DB                  *sql.DB
	Router              http.Handler
	Server              *httptest.Server
	Fetcher             *memoryFetcher
	SubService          *subscription.Service
	InvService          *inventory.Service
	ProbeService        *probe.Service
	PolicyService       *policy.Service
	RevisionService     *revision.Service
	PublicationService  *publication.Service
	IPRiskService       *iprisk.Service
	SettingsRepo        domain.SettingsRepository
	TokenHolder         *transporthttp.DynamicTokenHolder
	SubRepo             domain.SubscriptionRepository
	FetchRepo           domain.SubscriptionFetchRepository
	NodeRepo            domain.NodeRepository
	SourceRepo          domain.NodeSourceRepository
	ProbeRunRepo        domain.ProbeRunRepository
	ProbeObsRepo        domain.ProbeObservationRepository
	PolicyRepo          domain.PolicyRepository
	RevisionRepo        domain.RevisionRepository
	PublicationRepo     domain.PublicationRepository
	RiskPolicyRepo      domain.RiskPolicyRevisionRepository
	RiskBindingRepo     domain.RiskPolicyGroupBindingRepository
	AuditRepo           domain.AuditRepository
	InitialAdminToken   string
}

func setupTestHarness(t *testing.T) *TestHarness {
	t.Helper()
	ctx := context.Background()

	db, err := sqlite.Open(sqlite.Config{
		Path:        fmt.Sprintf("file:e2e_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		BusyTimeout: 5 * time.Second,
		ForeignKeys: true,
	})
	if err != nil {
		t.Fatalf("open e2e test sqlite database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := sqlite.NewMigrationRunner(db, migrations.FS).Run(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	initialToken := "e2e-test-admin-secret-token-32b"

	// Repositories
	subRepo := sqlite.NewSubscriptionRepository(db)
	fetchRepo := sqlite.NewSubscriptionFetchRepository(db)
	nodeRepo := sqlite.NewNodeRepository(db)
	sourceRepo := sqlite.NewNodeSourceRepository(db)
	probeRunRepo := sqlite.NewProbeRunRepository(db)
	probeObsRepo := sqlite.NewProbeObservationRepository(db)
	policyRepo := sqlite.NewPolicyRepository(db)
	revisionRepo := sqlite.NewRevisionRepository(db)
	publicationRepo := sqlite.NewPublicationRepository(db)
	riskPolicyRepo := sqlite.NewRiskPolicyRevisionRepository(db)
	riskBindingRepo := sqlite.NewRiskPolicyGroupBindingRepository(db)
	riskObsRepo := sqlite.NewIPRiskObservationRepository(db)
	auditRepo := sqlite.NewAuditRepository(db)
	settingsRepo := sqlite.NewSettingsRepository(db)
	nodeFilterRepo := sqlite.NewNodeFilterRepository(db)
	probeScheduleRepo := sqlite.NewProbeScheduleRepository(db)

	fetcher := newMemoryFetcher()

	// Services
	subService := subscription.NewService(subRepo, auditRepo)
	invService := inventory.NewService(db, subRepo, fetchRepo, nodeRepo, sourceRepo, fetcher,
		inventory.WithProbeObservationRepository(probeObsRepo),
	)
	subService.SetReconciler(invService)

	probeScheduler, err := queue.NewScheduler(queue.Config{
		Concurrency: queue.DefaultConcurrency,
		Context:     ctx,
	})
	if err != nil {
		t.Fatalf("create probe scheduler: %v", err)
	}
	t.Cleanup(func() { probeScheduler.Close() })

	probeRunner := probe.NewDefaultRunner(nodeRepo, probeObsRepo, probeScheduler, probeRunRepo)
	probeService := probe.NewService(
		probeRunRepo,
		probe.WithRunner(probeRunner),
		probe.WithNodeRepository(nodeRepo),
		probe.WithObservationRepository(probeObsRepo),
		probe.WithScheduler(probeScheduler),
		probe.WithScheduleRepository(probeScheduleRepo),
	)

	policyService := policy.NewService(policyRepo, revisionRepo, nodeRepo, auditRepo, nodeFilterRepo)
	revisionService := revision.NewService(revisionRepo, auditRepo, revision.WithPolicyRepository(policyRepo))

	if _, err := policyService.EnsureActiveRevision(ctx); err != nil {
		t.Fatalf("ensure active revision: %v", err)
	}

	ipriskService := iprisk.NewService(
		riskObsRepo,
		riskPolicyRepo,
		iprisk.WithBindingRepository(riskBindingRepo),
		iprisk.WithGroupRepository(policyRepo),
		iprisk.WithNodeRepository(nodeRepo),
	)

	pubService := publication.NewService(
		publicationRepo,
		auditRepo,
		publication.WithNodeRepository(nodeRepo),
		publication.WithPolicyRepository(policyRepo),
		publication.WithRevisionRepository(revisionRepo),
		publication.WithIPRiskService(ipriskService),
		publication.WithRiskPolicyRepository(riskPolicyRepo),
		publication.WithRiskBindingRepository(riskBindingRepo),
		publication.WithRiskObservationRepository(riskObsRepo),
	)

	sessionStore := transporthttp.NewMemorySessionStore(24 * time.Hour)

	initialAdminAuth := false
	initialExportAuth := false
	tokenHolder := transporthttp.NewDynamicTokenHolder(transporthttp.TokenHolderConfig{
		InitialToken:      initialToken,
		SettingsRepo:      settingsRepo,
		SessionStore:      sessionStore,
		HashCost:          transporthttp.MinHashCost,
		AdminAuthEnabled:  &initialAdminAuth,
		ExportAuthEnabled: &initialExportAuth,
	})

	routerConfig := transporthttp.RouterConfig{
		TokenHolder:         tokenHolder,
		SessionStore:        sessionStore,
		SettingsRepository:  settingsRepo,
		ReadinessChecker: func(c context.Context) (*sqlite.ReadinessReport, error) {
			return sqlite.CheckReadiness(c, db)
		},
		PublicationService:  pubService,
		SubscriptionService: subService,
		InventoryService:    invService,
		ProbeService:        probeService,
		PolicyService:       policyService,
		RevisionService:     revisionService,
		IPRiskService:       ipriskService,
		ProbeRunRepository:  probeRunRepo,
		ProbeObservationRepository: probeObsRepo,
		RiskPolicyRepository: riskPolicyRepo,
		RiskBindingRepository: riskBindingRepo,
		AuditRepository:     auditRepo,
	}

	router := transporthttp.NewRouter(routerConfig)
	server := httptest.NewServer(router)
	t.Cleanup(func() { server.Close() })

	return &TestHarness{
		DB:                  db,
		Router:              router,
		Server:              server,
		Fetcher:             fetcher,
		SubService:          subService,
		InvService:          invService,
		ProbeService:        probeService,
		PolicyService:       policyService,
		RevisionService:     revisionService,
		PublicationService:  pubService,
		IPRiskService:       ipriskService,
		SettingsRepo:        settingsRepo,
		TokenHolder:         tokenHolder,
		SubRepo:             subRepo,
		FetchRepo:           fetchRepo,
		NodeRepo:            nodeRepo,
		SourceRepo:          sourceRepo,
		ProbeRunRepo:        probeRunRepo,
		ProbeObsRepo:        probeObsRepo,
		PolicyRepo:          policyRepo,
		RevisionRepo:        revisionRepo,
		PublicationRepo:     publicationRepo,
		RiskPolicyRepo:      riskPolicyRepo,
		RiskBindingRepo:     riskBindingRepo,
		AuditRepo:           auditRepo,
		InitialAdminToken:   initialToken,
	}
}

type HTTPResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Cookies    []*http.Cookie
}

func (r *HTTPResponse) JSON(v any) error {
	return json.Unmarshal(r.Body, v)
}

func (h *TestHarness) Request(method, path string, body any, headers map[string]string, cookies []*http.Cookie) (*HTTPResponse, error) {
	var bodyReader io.Reader
	if body != nil {
		switch b := body.(type) {
		case string:
			bodyReader = strings.NewReader(b)
		case []byte:
			bodyReader = bytes.NewReader(b)
		default:
			raw, err := json.Marshal(b)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(raw)
		}
	}

	url := h.Server.URL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	for _, c := range cookies {
		req.AddCookie(c)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       respBody,
		Cookies:    resp.Cookies(),
	}, nil
}

func (h *TestHarness) AuthRequest(method, path string, body any, headers map[string]string) (*HTTPResponse, error) {
	if headers == nil {
		headers = make(map[string]string)
	}
	headers["Authorization"] = "Bearer " + h.InitialAdminToken
	return h.Request(method, path, body, headers, nil)
}
