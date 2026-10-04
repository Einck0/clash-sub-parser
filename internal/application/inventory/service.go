// Package inventory orchestrates node ingestion, provenance tracking, and set-difference reconciliation.
package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/repository/sqlite"
)

// ReconcileResult summarizes the outcome of a subscription reconciliation.
type ReconcileResult struct {
	FetchID        string              `json:"fetch_id"`
	SubscriptionID string              `json:"subscription_id"`
	Outcome        domain.FetchOutcome `json:"outcome"`
	ContentDigest  string              `json:"content_digest"`
	RedactedError  string              `json:"redacted_error,omitempty"`
	NodesParsed    int                 `json:"nodes_parsed"`
	NodesValid     int                 `json:"nodes_valid"`
}

const defaultProbeFreshnessTTL = time.Hour

// CapabilityStatus represents the latest probe observation summary for a single probe dimension on a node.
type CapabilityStatus struct {
	Verdict    domain.ProbeVerdict                  `json:"verdict"`
	LatencyMS  int64                                `json:"latency_ms"`
	ObservedAt time.Time                            `json:"observed_at"`
	Summary    string                               `json:"summary,omitempty"`
	Stale      bool                                 `json:"stale"`
	Region     string                               `json:"region,omitempty"`
	SubTier    string                               `json:"sub_tier,omitempty"`
	Throughput *float64                             `json:"throughput,omitempty"`
	RiskScore  string                               `json:"risk_score,omitempty"`
	Platforms  map[string]domain.PlatformCapability `json:"platforms,omitempty"`
}

// NodeCapabilityView is an alias for CapabilityStatus.
type NodeCapabilityView = CapabilityStatus

// NodeDetail pairs a normalized node with its provenance sources, probe status, and API-safe risk summary.
type NodeDetail struct {
	Node               domain.Node                   `json:"node"`
	View               NodeView                      `json:"view"`
	Sources            []domain.NodeSource           `json:"sources"`
	IPRiskSummary      *domain.IPRiskSummary         `json:"ip_risk_summary,omitempty"`
	RecentObservations []domain.IPRiskObservation    `json:"recent_observations,omitempty"`
	LatencyMS          *int64                        `json:"latency_ms,omitempty"`
	LastProbedAt       *time.Time                    `json:"last_probed_at,omitempty"`
	HealthStatus       string                        `json:"health_status"`
	ProbeState         string                        `json:"probe_state"`
	ProbeMissing       bool                          `json:"probe_missing"`
	ProbeStale         bool                          `json:"probe_stale"`
	Capabilities       map[string]NodeCapabilityView `json:"capabilities,omitempty"`
}

// ToNodeView returns the enriched NodeView for this NodeDetail, with a fallback if View was not pre-populated.
func (d *NodeDetail) ToNodeView() NodeView {
	if d == nil {
		return NodeView{HealthStatus: "unknown", ProbeState: "idle", ProbeMissing: true}
	}
	v := d.View
	if v.LogicalID == "" {
		v = ToNodeView(d.Node)
		v.IPRiskSummary = d.IPRiskSummary
		v.Sources = d.Sources
		v.LatencyMS = d.LatencyMS
		v.LastProbedAt = d.LastProbedAt
		if d.HealthStatus != "" {
			v.HealthStatus = d.HealthStatus
		}
		if d.ProbeState != "" {
			v.ProbeState = d.ProbeState
		}
		v.ProbeMissing = d.ProbeMissing
		v.ProbeStale = d.ProbeStale
		v.Capabilities = d.Capabilities
	} else {
		if v.IPRiskSummary == nil && d.IPRiskSummary != nil {
			v.IPRiskSummary = d.IPRiskSummary
		}
		if len(v.Sources) == 0 && len(d.Sources) > 0 {
			v.Sources = d.Sources
		}
		if v.ProbeState == "" {
			if d.ProbeState != "" {
				v.ProbeState = d.ProbeState
			} else {
				v.ProbeState = "idle"
			}
		}
	}
	return v
}

// NodeView is the management API view of a Node including plaintext server, port, protocol credentials, probe status, and sources.
type NodeView struct {
	LogicalID          string                            `json:"logical_id"`
	Protocol           domain.Protocol                   `json:"protocol"`
	DisplayName        string                            `json:"display_name"`
	Server             string                            `json:"server,omitempty"`
	Port               int                               `json:"port,omitempty"`
	Credentials        *domain.InboundProtocolCredential `json:"credentials,omitempty"`
	Active             bool                              `json:"active"`
	ConnectionRevision int64                             `json:"connection_revision,omitempty"`
	CreatedAt          time.Time                         `json:"created_at"`
	UpdatedAt          time.Time                         `json:"updated_at"`
	IPRiskSummary      *domain.IPRiskSummary             `json:"ip_risk_summary,omitempty"`
	LatencyMS          *int64                            `json:"latency_ms,omitempty"`
	LastProbedAt       *time.Time                        `json:"last_probed_at,omitempty"`
	HealthStatus       string                            `json:"health_status"`
	ProbeState         string                            `json:"probe_state"`
	ProbeMissing       bool                              `json:"probe_missing"`
	ProbeStale         bool                              `json:"probe_stale"`
	Capabilities       map[string]NodeCapabilityView     `json:"capabilities,omitempty"`
	Sources            []domain.NodeSource               `json:"sources,omitempty"`
}

// ToNodeView converts a domain.Node to a NodeView.
func ToNodeView(n domain.Node) NodeView {
	creds := n.Credentials
	return NodeView{
		LogicalID:          n.LogicalID,
		Protocol:           n.Protocol,
		DisplayName:        n.DisplayName,
		Server:             n.Server,
		Port:               n.Port,
		Credentials:        &creds,
		Active:             n.Active,
		ConnectionRevision: n.ConnectionRevision,
		CreatedAt:          n.CreatedAt,
		UpdatedAt:          n.UpdatedAt,
		HealthStatus:       "unknown",
		ProbeState:         "idle",
		ProbeMissing:       true,
		ProbeStale:         false,
	}
}

// ToNodeViewFromReadModel converts a domain.NodeReadModel to a NodeView.
func ToNodeViewFromReadModel(rm domain.NodeReadModel) NodeView {
	v := ToNodeView(rm.Node)
	v.IPRiskSummary = rm.IPRiskSummary
	return v
}

// ToNodeViewsFromReadModels converts a slice of domain.NodeReadModel to a slice of NodeView.
func ToNodeViewsFromReadModels(models []domain.NodeReadModel) []NodeView {
	views := make([]NodeView, len(models))
	for i, rm := range models {
		views[i] = ToNodeViewFromReadModel(rm)
	}
	return views
}

// ToNodeViews converts a slice of domain.Node to a slice of NodeView.
func ToNodeViews(nodes []domain.Node) []NodeView {
	views := make([]NodeView, len(nodes))
	for i, n := range nodes {
		views[i] = ToNodeView(n)
	}
	return views
}

// NodePoolStateProvider is implemented by anything that can report the real-time
// probe-pool state for a given node ("probing", "queued", or "idle").
type NodePoolStateProvider interface {
	GetNodePoolState(logicalID string) string
}

// Service coordinates inventory ingestion, provenance reconciliation, and ledger queries.
type Service struct {
	db                *sql.DB
	subscriptions     domain.SubscriptionRepository
	fetches           domain.SubscriptionFetchRepository
	nodes             domain.NodeRepository
	sources           domain.NodeSourceRepository
	fetcher           fetch.Fetcher
	probeObsRepo      domain.ProbeObservationRepository
	auditRepo         domain.AuditRepository
	payloadRepo       domain.SubscriptionPayloadRepository
	entryRepo         domain.SubscriptionEntryRepository
	versionRepo       domain.NodeConnectionVersionRepository
	overrideRepo      domain.NodeOverrideRepository
	pubPayloadRefRepo domain.PublicationPayloadRefRepository
	sourceHistoryRepo domain.NodeSourceHistoryRepository
	poolProvider      NodePoolStateProvider
	defaultFetchProxy string
	clock             func() time.Time
	mu                sync.Mutex
}

// Reconciler is an alias for Service to satisfy reconciler role expectations.
type Reconciler = Service

// Option configures Service dependencies.
type Option func(*Service)

// WithSubscriptionPayloadRepository sets the subscription payload repository.
func WithSubscriptionPayloadRepository(repo domain.SubscriptionPayloadRepository) Option {
	return func(s *Service) {
		s.payloadRepo = repo
	}
}

// WithSubscriptionEntryRepository sets the subscription entry repository.
func WithSubscriptionEntryRepository(repo domain.SubscriptionEntryRepository) Option {
	return func(s *Service) {
		s.entryRepo = repo
	}
}

// WithNodeConnectionVersionRepository sets the node connection version repository.
func WithNodeConnectionVersionRepository(repo domain.NodeConnectionVersionRepository) Option {
	return func(s *Service) {
		s.versionRepo = repo
	}
}

// WithNodeOverrideRepository sets the node override repository.
func WithNodeOverrideRepository(repo domain.NodeOverrideRepository) Option {
	return func(s *Service) {
		s.overrideRepo = repo
	}
}

// WithNodeSourceHistoryRepository sets the node source history repository.
func WithNodeSourceHistoryRepository(repo domain.NodeSourceHistoryRepository) Option {
	return func(s *Service) {
		s.sourceHistoryRepo = repo
	}
}

// WithPublicationPayloadRefRepository sets the publication payload ref repository.
func WithPublicationPayloadRefRepository(repo domain.PublicationPayloadRefRepository) Option {
	return func(s *Service) {
		s.pubPayloadRefRepo = repo
	}
}

// WithProbeObservationRepository sets the probe observation repository.
func WithProbeObservationRepository(repo domain.ProbeObservationRepository) Option {
	return func(s *Service) {
		s.probeObsRepo = repo
	}
}

// WithAuditRepository sets the audit repository for recording node connection mutations.
func WithAuditRepository(repo domain.AuditRepository) Option {
	return func(s *Service) {
		s.auditRepo = repo
	}
}

// WithNodePoolStateProvider injects a real-time node-pool state provider so that
// NodeView can reflect "probing" / "queued" / "idle" live probe-pool states.
func WithNodePoolStateProvider(provider NodePoolStateProvider) Option {
	return func(s *Service) {
		s.poolProvider = provider
	}
}

// WithDefaultFetchProxy configures the fallback outbound proxy for subscriptions without an explicit proxy ref.
func WithDefaultFetchProxy(proxyURL string) Option {
	return func(s *Service) {
		s.defaultFetchProxy = strings.TrimSpace(proxyURL)
	}
}

// WithClock configures a deterministic clock for read-model freshness evaluation in tests.
func WithClock(clock func() time.Time) Option {
	return func(s *Service) {
		if clock != nil {
			s.clock = clock
		}
	}
}

func (s *Service) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock().UTC()
	}
	return domain.NowUTC()
}

// SetNodePoolStateProvider sets or replaces the real-time node-pool state provider.
func (s *Service) SetNodePoolStateProvider(provider NodePoolStateProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.poolProvider = provider
}

// NewService constructs an inventory application service.
func NewService(
	db *sql.DB,
	subscriptions domain.SubscriptionRepository,
	fetches domain.SubscriptionFetchRepository,
	nodes domain.NodeRepository,
	sources domain.NodeSourceRepository,
	fetcher fetch.Fetcher,
	opts ...Option,
) *Service {
	if fetcher == nil {
		fetcher = fetch.NewClient()
	}
	s := &Service{
		db:            db,
		subscriptions: subscriptions,
		fetches:       fetches,
		nodes:         nodes,
		sources:       sources,
		fetcher:       fetcher,
	}
	if db != nil {
		s.auditRepo = sqlite.NewAuditRepository(db)
		s.probeObsRepo = sqlite.NewProbeObservationRepository(db)
		s.payloadRepo = sqlite.NewSubscriptionPayloadRepository(db)
		s.entryRepo = sqlite.NewSubscriptionEntryRepository(db)
		s.versionRepo = sqlite.NewNodeConnectionVersionRepository(db)
		s.overrideRepo = sqlite.NewNodeOverrideRepository(db)
		s.pubPayloadRefRepo = sqlite.NewPublicationPayloadRefRepository(db)
		s.sourceHistoryRepo = sqlite.NewNodeSourceHistoryRepository(db)
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// SourceHistoryRepository returns the node source history repository handle.
func (s *Service) SourceHistoryRepository() domain.NodeSourceHistoryRepository {
	return s.sourceHistoryRepo
}

// EntryRepository returns the entry repository handle.
func (s *Service) EntryRepository() domain.SubscriptionEntryRepository {
	return s.entryRepo
}

// PayloadRepository returns the payload repository handle.
func (s *Service) PayloadRepository() domain.SubscriptionPayloadRepository {
	return s.payloadRepo
}

// VersionRepository returns the version repository handle.
func (s *Service) VersionRepository() domain.NodeConnectionVersionRepository {
	return s.versionRepo
}

// OverrideRepository returns the override repository handle.
func (s *Service) OverrideRepository() domain.NodeOverrideRepository {
	return s.overrideRepo
}

// ReconcileSubscription executes the safe fetch, in-memory parse, and atomic transaction convergence.
func (s *Service) ReconcileSubscription(ctx context.Context, subID string) (*ReconcileResult, error) {
	sub, err := s.subscriptions.GetByID(ctx, subID)
	if err != nil {
		return nil, err
	}
	if !sub.Enabled {
		return nil, domain.NewValidationError("subscription_disabled", "disabled subscriptions cannot be refreshed")
	}

	startedAt := domain.NowUTC()
	proxyURL := sub.RefreshPolicy.FetchProxyRef
	if proxyURL == "" {
		proxyURL = s.defaultFetchProxy
	}
	opts := fetch.OptionsFromPolicy(sub.SourceURLSecretRef, sub.RefreshPolicy, proxyURL)
	fetchResp, fetchErr := s.fetcher.Fetch(ctx, opts)
	if fetchErr != nil {
		finishedAt := domain.NowUTC()
		fetchID := domain.MustNewUUIDv7()
		redactedErr := fetch.RedactError(fetchErr)

		failedFetch := domain.SubscriptionFetch{
			ID:             fetchID,
			SubscriptionID: sub.ID,
			StartedAt:      startedAt,
			FinishedAt:     &finishedAt,
			Outcome:        domain.FetchOutcomeFailed,
			ContentDigest:  "",
			RedactedError:  redactedErr,
			NodesParsed:    0,
			NodesValid:     0,
		}
		_ = s.fetches.Create(ctx, &failedFetch)
		if s.auditRepo != nil {
			_ = s.auditRepo.Record(ctx, &domain.AuditEvent{
				ID:              domain.MustNewUUIDv7(),
				ActorKind:       domain.ActorKindSystem,
				Action:          "subscription.fetch.failed",
				RedactedSummary: redactedErr,
				Result:          domain.AuditResultFailure,
				CreatedAt:       finishedAt,
			})
		}

		return &ReconcileResult{
			FetchID:        fetchID,
			SubscriptionID: sub.ID,
			Outcome:        domain.FetchOutcomeFailed,
			RedactedError:  redactedErr,
		}, fmt.Errorf("failed to fetch subscription %s: %w", sub.ID, fetchErr)
	}

	extractResult, extractErr := parser.ExtractWithCredentials(fetchResp.Body)
	if extractErr != nil {
		finishedAt := domain.NowUTC()
		fetchID := domain.MustNewUUIDv7()
		redactedErr := domain.RedactSensitiveInfo(extractErr.Error())

		failedFetch := domain.SubscriptionFetch{
			ID:             fetchID,
			SubscriptionID: sub.ID,
			StartedAt:      startedAt,
			FinishedAt:     &finishedAt,
			Outcome:        domain.FetchOutcomeFailed,
			ContentDigest:  fetchResp.ContentDigest,
			RedactedError:  redactedErr,
			NodesParsed:    0,
			NodesValid:     0,
		}
		_ = s.fetches.Create(ctx, &failedFetch)
		if s.auditRepo != nil {
			_ = s.auditRepo.Record(ctx, &domain.AuditEvent{
				ID:              domain.MustNewUUIDv7(),
				ActorKind:       domain.ActorKindSystem,
				Action:          "subscription.parse.failed",
				RedactedSummary: redactedErr,
				Result:          domain.AuditResultFailure,
				CreatedAt:       finishedAt,
			})
		}

		return &ReconcileResult{
			FetchID:        fetchID,
			SubscriptionID: sub.ID,
			Outcome:        domain.FetchOutcomeFailed,
			ContentDigest:  fetchResp.ContentDigest,
			RedactedError:  redactedErr,
		}, fmt.Errorf("failed to parse subscription %s content: %w", sub.ID, extractErr)
	}

	if len(extractResult.Items) == 0 {
		finishedAt := domain.NowUTC()
		fetchID := domain.MustNewUUIDv7()
		redactedErr := "extracted 0 valid nodes from subscription content"

		failedFetch := domain.SubscriptionFetch{
			ID:             fetchID,
			SubscriptionID: sub.ID,
			StartedAt:      startedAt,
			FinishedAt:     &finishedAt,
			Outcome:        domain.FetchOutcomeFailed,
			ContentDigest:  fetchResp.ContentDigest,
			RedactedError:  redactedErr,
			NodesParsed:    extractResult.Rejected,
			NodesValid:     0,
		}
		_ = s.fetches.Create(ctx, &failedFetch)
		if s.auditRepo != nil {
			_ = s.auditRepo.Record(ctx, &domain.AuditEvent{
				ID:              domain.MustNewUUIDv7(),
				ActorKind:       domain.ActorKindSystem,
				Action:          "subscription.extract.empty",
				RedactedSummary: redactedErr,
				Result:          domain.AuditResultFailure,
				CreatedAt:       finishedAt,
			})
		}

		return &ReconcileResult{
			FetchID:        fetchID,
			SubscriptionID: sub.ID,
			Outcome:        domain.FetchOutcomeFailed,
			ContentDigest:  fetchResp.ContentDigest,
			RedactedError:  redactedErr,
			NodesParsed:    extractResult.Rejected,
			NodesValid:     0,
		}, fmt.Errorf("failed to reconcile subscription %s: %s", sub.ID, redactedErr)
	}

	nodesParsed := len(extractResult.Items) + extractResult.Rejected
	nodesValid := len(extractResult.Items)
	outcome := domain.FetchOutcomeSuccess
	if extractResult.Rejected > 0 {
		outcome = domain.FetchOutcomePartial
	}

	fetchID := domain.MustNewUUIDv7()
	finishedAt := domain.NowUTC()
	nowStr := domain.NowUTC().Format(time.RFC3339)

	s.mu.Lock()
	defer s.mu.Unlock()

	err = sqlite.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		const insertFetchSQL = `
		INSERT INTO subscription_fetches (
			id, subscription_id, started_at, finished_at, outcome,
			content_digest, redacted_error, nodes_parsed, nodes_valid
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);`

		finishedStr := sql.NullString{String: finishedAt.Format(time.RFC3339), Valid: true}
		if _, err := tx.ExecContext(ctx, insertFetchSQL,
			fetchID,
			sub.ID,
			startedAt.Format(time.RFC3339),
			finishedStr,
			string(outcome),
			fetchResp.ContentDigest,
			"",
			nodesParsed,
			nodesValid,
		); err != nil {
			return fmt.Errorf("failed to insert subscription fetch: %w", err)
		}

		payloadID := "payload_" + domain.MustNewUUIDv7()
		headersMap := map[string]string{
			"content_type":  fetchResp.ContentType,
			"etag":          fetchResp.ETag,
			"last_modified": fetchResp.LastModified,
		}
		headersJSON, _ := json.Marshal(headersMap)
		httpStatus := fetchResp.StatusCode
		if httpStatus <= 0 {
			httpStatus = 200
		}
		const insertPayloadSQL = `
		INSERT INTO subscription_payloads (
			id, subscription_id, fetch_id, content_digest, body_blob,
			http_status, headers_json, pinned, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?);`
		if _, pErr := tx.ExecContext(ctx, insertPayloadSQL,
			payloadID,
			sub.ID,
			fetchID,
			fetchResp.ContentDigest,
			fetchResp.Body,
			httpStatus,
			string(headersJSON),
			nowStr,
		); pErr != nil {
			if !strings.Contains(pErr.Error(), "no such table") {
				return fmt.Errorf("failed to insert subscription payload: %w", pErr)
			}
		}

		type existingNodeRow struct {
			logicalID          string
			protocol           domain.Protocol
			displayName        string
			server             string
			port               int
			configJSON         string
			creds              domain.InboundProtocolCredential
			active             bool
			connectionRevision int64
		}
		existingNodes := make(map[string]existingNodeRow)
		existingByNameProto := make(map[string][]existingNodeRow)

		selectExistingSQL := `
		SELECT n.logical_id, n.protocol, n.display_name, n.server, n.port, n.config_json, n.active, n.connection_revision
		FROM nodes n
		INNER JOIN node_sources ns ON n.logical_id = ns.node_logical_id
		WHERE ns.subscription_id = ?;`

		var histTableExistsPre int
		_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='node_source_history';").Scan(&histTableExistsPre)
		var rows *sql.Rows
		var qErr error
		if histTableExistsPre > 0 {
			selectExistingSQL = `
			SELECT n.logical_id, n.protocol, n.display_name, n.server, n.port, n.config_json, n.active, n.connection_revision
			FROM nodes n
			WHERE n.logical_id IN (
				SELECT node_logical_id FROM node_sources WHERE subscription_id = ?
				UNION
				SELECT node_logical_id FROM node_source_history WHERE subscription_id = ?
			);`
			rows, qErr = tx.QueryContext(ctx, selectExistingSQL, sub.ID, sub.ID)
		} else {
			rows, qErr = tx.QueryContext(ctx, selectExistingSQL, sub.ID)
		}
		if qErr != nil {
			return fmt.Errorf("failed to query existing nodes for subscription: %w", qErr)
		}
		for rows.Next() {
			var r existingNodeRow
			var protoStr string
			var activeInt int
			if scanErr := rows.Scan(&r.logicalID, &protoStr, &r.displayName, &r.server, &r.port, &r.configJSON, &activeInt, &r.connectionRevision); scanErr != nil {
				rows.Close()
				return fmt.Errorf("failed to scan existing node: %w", scanErr)
			}
			r.protocol = domain.Protocol(protoStr)
			r.active = activeInt == 1
			_ = json.Unmarshal([]byte(r.configJSON), &r.creds)
			existingNodes[r.logicalID] = r
			npKey := r.displayName + "|" + string(r.protocol)
			existingByNameProto[npKey] = append(existingByNameProto[npKey], r)
		}
		rows.Close()

		// Load previous user overrides for this subscription
		type prevOverride struct {
			kind   domain.EntryKind
			anchor string
			reason string
			at     *time.Time
			actor  string
		}
		prevOverridesByAnchor := make(map[string]prevOverride)
		prevOverridesByName := make(map[string]prevOverride)
		ambiguousAnchors := make(map[string]bool)

		overrideRows, oErr := tx.QueryContext(ctx, `
			SELECT source_key, raw_name, user_kind_override, override_anchor, override_reason, override_at, actor_ref
			FROM subscription_entries
			WHERE subscription_id = ? AND user_kind_override IS NOT NULL;
		`, sub.ID)
		if oErr == nil {
			for overrideRows.Next() {
				var sk, rn, uko, oa, or, ar string
				var oAt sql.NullString
				if scanErr := overrideRows.Scan(&sk, &rn, &uko, &oa, &or, &oAt, &ar); scanErr == nil {
					k := domain.EntryKind(uko)
					var t *time.Time
					if oAt.Valid {
						parsed, _ := time.Parse(time.RFC3339, oAt.String)
						t = &parsed
					}
					po := prevOverride{
						kind:   k,
						anchor: oa,
						reason: or,
						at:     t,
						actor:  ar,
					}
					if existing, ok := prevOverridesByAnchor[oa]; ok && existing.kind != k {
						ambiguousAnchors[oa] = true
					} else {
						prevOverridesByAnchor[oa] = po
					}
					if sk != "" {
						if existing, ok := prevOverridesByAnchor[sk]; ok && existing.kind != k {
							ambiguousAnchors[sk] = true
						} else {
							prevOverridesByAnchor[sk] = po
						}
					}
					if rn != "" {
						if existing, ok := prevOverridesByName[rn]; ok && existing.kind != k {
							ambiguousAnchors[rn] = true
						} else {
							prevOverridesByName[rn] = po
						}
					}
				}
			}
			overrideRows.Close()
		}

		// Load field overrides from node_overrides table
		nodeFieldOverrides := make(map[string]map[string]string)
		noRows, noErr := tx.QueryContext(ctx, "SELECT node_logical_id, field_path, override_value_json FROM node_overrides;")
		if noErr == nil {
			for noRows.Next() {
				var nid, fp, val string
				if scanErr := noRows.Scan(&nid, &fp, &val); scanErr == nil {
					if nodeFieldOverrides[nid] == nil {
						nodeFieldOverrides[nid] = make(map[string]string)
					}
					nodeFieldOverrides[nid][fp] = val
				}
			}
			noRows.Close()
		}

		incomingByNameProto := make(map[string]int)
		for _, item := range extractResult.Items {
			node := item.Normalized.Node
			npKey := node.DisplayName + "|" + string(node.Protocol)
			incomingByNameProto[npKey]++
		}

		const upsertNodeSQL = `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at, connection_revision)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(logical_id) DO UPDATE SET
			protocol = excluded.protocol,
			display_name = excluded.display_name,
			server = excluded.server,
			port = excluded.port,
			config_json = excluded.config_json,
			active = excluded.active,
			updated_at = excluded.updated_at,
			connection_revision = excluded.connection_revision;`

		nodeStmt, err := tx.PrepareContext(ctx, upsertNodeSQL)
		if err != nil {
			return fmt.Errorf("failed to prepare upsert node statement: %w", err)
		}
		defer nodeStmt.Close()

		const upsertSourceSQL = `
		INSERT INTO node_sources (node_logical_id, subscription_id, last_seen_fetch_id)
		VALUES (?, ?, ?)
		ON CONFLICT(node_logical_id, subscription_id) DO UPDATE SET
			last_seen_fetch_id = excluded.last_seen_fetch_id;`

		sourceStmt, err := tx.PrepareContext(ctx, upsertSourceSQL)
		if err != nil {
			return fmt.Errorf("failed to prepare upsert node source statement: %w", err)
		}
		defer sourceStmt.Close()

		const upsertVersionSQL = `
		INSERT INTO node_connection_versions (
			node_logical_id, connection_revision, effective_config_json,
			config_fingerprint, source_entry_id, schema_version, created_at
		) VALUES (?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(node_logical_id, connection_revision) DO UPDATE SET
			effective_config_json = excluded.effective_config_json,
			config_fingerprint = excluded.config_fingerprint,
			source_entry_id = COALESCE(node_connection_versions.source_entry_id, excluded.source_entry_id);`

		var versionStmt *sql.Stmt
		vStmt, vErr := tx.PrepareContext(ctx, upsertVersionSQL)
		if vErr == nil {
			versionStmt = vStmt
			defer versionStmt.Close()
		}

		const upsertHeadSQL = `
		INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(logical_id) DO UPDATE SET
			connection_revision = excluded.connection_revision,
			updated_at = excluded.updated_at;`

		var headStmt *sql.Stmt
		hStmt, hErr := tx.PrepareContext(ctx, upsertHeadSQL)
		if hErr == nil {
			headStmt = hStmt
			defer headStmt.Close()
		}

		const insertEntrySQL = `
		INSERT INTO subscription_entries (
			id, payload_id, subscription_id, ordinal, source_key, raw_name,
			protocol, server, port, entry_kind, classification_reason,
			classification_version, source_provenance_json, user_kind_override,
			override_anchor, override_reason, override_at, actor_ref,
			parsed_config_json, parser_version, warnings_json, node_logical_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(payload_id, ordinal) DO UPDATE SET
			source_key = excluded.source_key,
			raw_name = excluded.raw_name,
			protocol = excluded.protocol,
			server = excluded.server,
			port = excluded.port,
			entry_kind = excluded.entry_kind,
			classification_reason = excluded.classification_reason,
			classification_version = excluded.classification_version,
			source_provenance_json = excluded.source_provenance_json,
			user_kind_override = excluded.user_kind_override,
			override_anchor = excluded.override_anchor,
			override_reason = excluded.override_reason,
			override_at = excluded.override_at,
			actor_ref = excluded.actor_ref,
			parsed_config_json = excluded.parsed_config_json,
			warnings_json = excluded.warnings_json,
			node_logical_id = excluded.node_logical_id;`

		var entryStmt *sql.Stmt
		eStmt, eErr := tx.PrepareContext(ctx, insertEntrySQL)
		if eErr == nil {
			entryStmt = eStmt
			defer entryStmt.Close()
		}

		isVerifiedSource := sub.ID == "01a0b9af-c116-7967-bc7f-197aa43a2e49" ||
			strings.Contains(strings.ToLower(sub.Name), "dogegg") ||
			strings.Contains(strings.ToLower(sub.SourceURLSecretRef), "dogegg")

		seenLogicalIDs := make(map[string]bool)
		for i, item := range extractResult.Items {
			node := item.Normalized.Node
			npKey := node.DisplayName + "|" + string(node.Protocol)
			ordinal := i
			entryID := "entry_" + domain.MustNewUUIDv7()

			sourceKey := fmt.Sprintf("%s|%s|%d|%s", node.Protocol, node.Server, node.Port, node.DisplayName)
			kind, classReason, classVer := domain.ClassifySubscriptionEntry(isVerifiedSource, node.Server, node.Port, node.Credentials, node.DisplayName)

			var userKindOverride *domain.EntryKind
			var overrideAnchor, overrideReason, actorRef string
			var overrideAt *time.Time
			var conflict string

			if po, ok := prevOverridesByAnchor[sourceKey]; ok {
				if ambiguousAnchors[sourceKey] {
					conflict = "ambiguous_anchor_override"
				} else {
					userKindOverride = &po.kind
					overrideAnchor = po.anchor
					overrideReason = po.reason
					overrideAt = po.at
					actorRef = po.actor
				}
			} else if po, ok := prevOverridesByName[node.DisplayName]; ok {
				if ambiguousAnchors[node.DisplayName] {
					conflict = "ambiguous_name_override"
				} else {
					userKindOverride = &po.kind
					overrideAnchor = po.anchor
					overrideReason = po.reason
					overrideAt = po.at
					actorRef = po.actor
				}
			}

			effectiveKind := kind
			if userKindOverride != nil && userKindOverride.IsValid() {
				effectiveKind = *userKindOverride
			}

			parsedConfigMap := map[string]any{
				"protocol":    node.Protocol,
				"server":      node.Server,
				"port":        node.Port,
				"credentials": node.Credentials,
			}
			parsedConfigBytes, _ := json.Marshal(parsedConfigMap)
			parsedConfigJSON := string(parsedConfigBytes)

			var targetLogicalIDPtr *string

			if effectiveKind == domain.EntryKindNotice {
				// Notices do NOT enter active proxy inventory!
				var matchedNoticeNode *existingNodeRow
				if node.LogicalID != "" {
					if ex, ok := existingNodes[node.LogicalID]; ok {
						matchedNoticeNode = &ex
					}
				}
				if matchedNoticeNode == nil && len(existingByNameProto[npKey]) == 1 {
					m := existingByNameProto[npKey][0]
					matchedNoticeNode = &m
				}
				if matchedNoticeNode != nil {
					// Notice pseudo-node linked for provenance; do NOT mutate nodes.active (lifecycle separated from classification)
					targetLogicalIDPtr = &matchedNoticeNode.logicalID
				}
			} else {
				// Proxy or Unknown: normal inventory ingestion
				candidateID := node.LogicalID
				if candidateID == "" {
					candidateID = domain.ComputeNodeLogicalID(node.Protocol, node.Server, node.Port, item.Normalized.Transport)
				}
				scopedCandidateID := computeScopedLogicalID(sub.ID, candidateID)

				var targetLogicalID string
				var currentRevision int64 = 1
				targetActive := 1

				// 1. Try to match an existing node already associated with this subscription
				var matchedNode *existingNodeRow
				if node.LogicalID != "" {
					if ex, ok := existingNodes[node.LogicalID]; ok {
						matchedNode = &ex
					}
				}

				existingList := existingByNameProto[npKey]
				isUniqueInExisting := len(existingList) == 1
				isUniqueInIncoming := incomingByNameProto[npKey] == 1

				if matchedNode == nil && isUniqueInExisting && isUniqueInIncoming {
					m := existingList[0]
					matchedNode = &m
				}

				if matchedNode == nil {
					if ex, ok := existingNodes[scopedCandidateID]; ok {
						matchedNode = &ex
					} else if ex, ok := existingNodes[candidateID]; ok {
						matchedNode = &ex
					}
				}

				if matchedNode != nil {
					targetLogicalID = matchedNode.logicalID
					if !matchedNode.active {
						targetActive = 0 // Preserve user disabled state
					}

					// Apply field overrides from node_overrides if present
					if fieldOverrides, ok := nodeFieldOverrides[targetLogicalID]; ok {
						for fp, val := range fieldOverrides {
							applyFieldOverride(&node, fp, val)
						}
					}

					connEqual := areConnectionParametersEqual(
						matchedNode.protocol, node.Protocol,
						matchedNode.server, node.Server,
						matchedNode.port, node.Port,
						matchedNode.creds, node.Credentials,
					)

					if connEqual {
						currentRevision = matchedNode.connectionRevision
						if versionStmt != nil {
							effJSONBytes, _ := json.Marshal(map[string]any{
								"server":      node.Server,
								"port":        node.Port,
								"credentials": node.Credentials,
							})
							fp := domain.ComputeConnectionFingerprint(node.Server, node.Port, node.Credentials)
							_, _ = versionStmt.ExecContext(ctx, targetLogicalID, currentRevision, string(effJSONBytes), fp, entryID, nowStr)
						}
					} else {
						// Connection parameters changed!
						var otherOwnersCount int
						if err := tx.QueryRowContext(ctx, `
							SELECT COUNT(*) FROM node_sources
							WHERE node_logical_id = ? AND subscription_id != ?;
						`, matchedNode.logicalID, sub.ID).Scan(&otherOwnersCount); err != nil {
							return fmt.Errorf("failed to check node sources count: %w", err)
						}

						if otherOwnersCount > 0 {
							// Copy-on-write isolation
							targetLogicalID = scopedCandidateID
							currentRevision = matchedNode.connectionRevision + 1

							// Snapshot old shared association before unlinking
							var histTableExists int
							_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='node_source_history';").Scan(&histTableExists)
							if histTableExists > 0 {
								histID := domain.MustNewUUIDv7()
								evidenceKey := fmt.Sprintf("cow_unlinked:%s:%s", matchedNode.logicalID, sub.ID)
								evidenceJSON := fmt.Sprintf(`{"action":"cow_unlinked","subscription_id":%q,"subscription_name":%q}`, sub.ID, sub.Name)
								if _, execErr := tx.ExecContext(ctx, `
									INSERT INTO node_source_history (
										id, node_logical_id, subscription_id, source_identity, source_label,
										connection_revision, relation_state, cause, first_observed_at,
										last_observed_at, evidence_kind, evidence_key, evidence_json, created_at
									) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
									ON CONFLICT(node_logical_id, evidence_key) DO UPDATE SET
										last_observed_at = excluded.last_observed_at;
								`, histID, matchedNode.logicalID, sub.ID, "sub:"+sub.ID, sub.Name, matchedNode.connectionRevision, string(domain.RelationStateVerified), string(domain.CauseRefreshRemoved), nowStr, nowStr, "cow_unlinked", evidenceKey, evidenceJSON, nowStr); execErr != nil {
									return fmt.Errorf("failed to insert cow history snapshot (node=%s): %w", matchedNode.logicalID, execErr)
								}
							}

							if _, err := tx.ExecContext(ctx, `
								DELETE FROM node_sources
								WHERE node_logical_id = ? AND subscription_id = ?;
							`, matchedNode.logicalID, sub.ID); err != nil {
								return fmt.Errorf("failed to unlink old shared node source: %w", err)
							}
						} else {
							targetLogicalID = matchedNode.logicalID
							currentRevision = matchedNode.connectionRevision + 1
						}

						// Insert new version & update head
						if versionStmt != nil && headStmt != nil {
							effJSONBytes, _ := json.Marshal(map[string]any{
								"server":      node.Server,
								"port":        node.Port,
								"credentials": node.Credentials,
							})
							fp := domain.ComputeConnectionFingerprint(node.Server, node.Port, node.Credentials)
							_, _ = versionStmt.ExecContext(ctx, targetLogicalID, currentRevision, string(effJSONBytes), fp, entryID, nowStr)
							_, _ = headStmt.ExecContext(ctx, targetLogicalID, currentRevision, nowStr)
						}
					}
				} else {
					// Not matched to any existing node in this subscription.
					var existingActive int
					var existingRev int64
					var existingConfig string
					var existingProto string
					var existingServer string
					var existingPort int

					checkErr := tx.QueryRowContext(ctx, `
						SELECT n.active, n.connection_revision, n.config_json, n.protocol, n.server, n.port
						FROM nodes n WHERE n.logical_id = ?;
					`, candidateID).Scan(&existingActive, &existingRev, &existingConfig, &existingProto, &existingServer, &existingPort)

					if checkErr == sql.ErrNoRows {
						targetLogicalID = candidateID
						currentRevision = 1
						targetActive = 1
					} else if checkErr == nil {
						var sameSubCount int
						var otherSubCount int
						_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_sources WHERE node_logical_id = ? AND subscription_id = ?;`, candidateID, sub.ID).Scan(&sameSubCount)
						_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM node_sources WHERE node_logical_id = ? AND subscription_id != ?;`, candidateID, sub.ID).Scan(&otherSubCount)

						if sameSubCount > 0 {
							targetLogicalID = candidateID
							currentRevision = existingRev
							if existingActive == 0 {
								targetActive = 0
							}
						} else if otherSubCount == 0 {
							var exCreds domain.InboundProtocolCredential
							_ = json.Unmarshal([]byte(existingConfig), &exCreds)
							if areConnectionParametersEqual(domain.Protocol(existingProto), node.Protocol, existingServer, node.Server, existingPort, node.Port, exCreds, node.Credentials) {
								targetLogicalID = candidateID
								currentRevision = existingRev
								if existingActive == 0 {
									targetActive = 1
								}
							} else {
								targetLogicalID = scopedCandidateID
							}
						} else {
							var exCreds domain.InboundProtocolCredential
							_ = json.Unmarshal([]byte(existingConfig), &exCreds)
							if areConnectionParametersEqual(domain.Protocol(existingProto), node.Protocol, existingServer, node.Server, existingPort, node.Port, exCreds, node.Credentials) {
								targetLogicalID = candidateID
								currentRevision = existingRev
								if existingActive == 0 {
									targetActive = 0
								}
							} else {
								targetLogicalID = scopedCandidateID
							}
						}

						if targetLogicalID == scopedCandidateID {
							var scActive int
							var scRev int64
							var scConfig string
							var scProto string
							var scServer string
							var scPort int
							scErr := tx.QueryRowContext(ctx, `
								SELECT n.active, n.connection_revision, n.config_json, n.protocol, n.server, n.port
								FROM nodes n WHERE n.logical_id = ?;
							`, scopedCandidateID).Scan(&scActive, &scRev, &scConfig, &scProto, &scServer, &scPort)
							if scErr == nil {
								if scActive == 0 {
									targetActive = 0
								}
								var scCreds domain.InboundProtocolCredential
								_ = json.Unmarshal([]byte(scConfig), &scCreds)
								if areConnectionParametersEqual(domain.Protocol(scProto), node.Protocol, scServer, node.Server, scPort, node.Port, scCreds, node.Credentials) {
									currentRevision = scRev
								} else {
									currentRevision = scRev + 1
								}
							} else {
								currentRevision = 1
								targetActive = 1
							}
						}
					} else {
						return fmt.Errorf("failed to query node %s: %w", candidateID, checkErr)
					}

					// New node initial version & head
					if versionStmt != nil && headStmt != nil {
						effJSONBytes, _ := json.Marshal(map[string]any{
							"server":      node.Server,
							"port":        node.Port,
							"credentials": node.Credentials,
						})
						fp := domain.ComputeConnectionFingerprint(node.Server, node.Port, node.Credentials)
						_, _ = versionStmt.ExecContext(ctx, targetLogicalID, currentRevision, string(effJSONBytes), fp, entryID, nowStr)
						_, _ = headStmt.ExecContext(ctx, targetLogicalID, currentRevision, nowStr)
					}
				}

				rawCreds, mErr := json.Marshal(node.Credentials)
				if mErr != nil {
					return fmt.Errorf("failed to marshal credentials for node %s: %w", targetLogicalID, mErr)
				}
				configJSON := string(rawCreds)
				if configJSON == "" {
					configJSON = "{}"
				}

				if _, err := nodeStmt.ExecContext(ctx,
					targetLogicalID,
					string(node.Protocol),
					node.DisplayName,
					node.Server,
					node.Port,
					configJSON,
					targetActive,
					nowStr,
					nowStr,
					currentRevision,
				); err != nil {
					return fmt.Errorf("failed to upsert node %s: %w", targetLogicalID, err)
				}

				if !seenLogicalIDs[targetLogicalID] {
					seenLogicalIDs[targetLogicalID] = true
					if _, err := sourceStmt.ExecContext(ctx,
						targetLogicalID,
						sub.ID,
						fetchID,
					); err != nil {
						return fmt.Errorf("failed to upsert node source for node %s: %w", targetLogicalID, err)
					}
				}

				targetLogicalIDPtr = &targetLogicalID
			}

			if entryStmt != nil {
				var userOverrideVal sql.NullString
				if userKindOverride != nil {
					userOverrideVal = sql.NullString{String: string(*userKindOverride), Valid: true}
				}
				var overrideAtVal sql.NullString
				if overrideAt != nil {
					overrideAtVal = sql.NullString{String: overrideAt.Format(time.RFC3339), Valid: true}
				}
				var targetLogicalIDVal sql.NullString
				if targetLogicalIDPtr != nil && *targetLogicalIDPtr != "" {
					targetLogicalIDVal = sql.NullString{String: *targetLogicalIDPtr, Valid: true}
				}

				warningsJSON := "[]"
				if conflict != "" {
					warningsJSON = fmt.Sprintf("[%q]", conflict)
				}

				_, _ = entryStmt.ExecContext(ctx,
					entryID,
					payloadID,
					sub.ID,
					ordinal,
					sourceKey,
					node.DisplayName,
					string(node.Protocol),
					node.Server,
					node.Port,
					string(kind),
					classReason,
					classVer,
					"{}",
					userOverrideVal,
					overrideAnchor,
					overrideReason,
					overrideAtVal,
					actorRef,
					parsedConfigJSON,
					"1.0.0",
					warningsJSON,
					targetLogicalIDVal,
					nowStr,
				)
			}
		}

		// 4. Archive obsolete node sources under current subscription before pruning
		var histTableExists int
		_ = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='node_source_history';").Scan(&histTableExists)
		if histTableExists > 0 {
			type pruneRow struct {
				nodeID    string
				fetchID   string
				rev       int64
				startedAt sql.NullString
			}
			rows, qErr := tx.QueryContext(ctx, `
				SELECT ns.node_logical_id, ns.last_seen_fetch_id, COALESCE(n.connection_revision, 1), sf.started_at
				FROM node_sources ns
				JOIN nodes n ON ns.node_logical_id = n.logical_id
				LEFT JOIN subscription_fetches sf ON ns.last_seen_fetch_id = sf.id
				WHERE ns.subscription_id = ? AND ns.last_seen_fetch_id != ?;
			`, sub.ID, fetchID)
			if qErr == nil {
				var toPrune []pruneRow
				for rows.Next() {
					var p pruneRow
					if scanErr := rows.Scan(&p.nodeID, &p.fetchID, &p.rev, &p.startedAt); scanErr == nil {
						toPrune = append(toPrune, p)
					}
				}
				rows.Close()

				if len(toPrune) > 0 {
					histStmt, prepErr := tx.PrepareContext(ctx, `
						INSERT INTO node_source_history (
							id, node_logical_id, subscription_id, source_identity, source_label,
							connection_revision, relation_state, cause, first_observed_at,
							last_observed_at, evidence_kind, evidence_key, evidence_json, created_at
						) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
						ON CONFLICT(node_logical_id, evidence_key) DO UPDATE SET
							last_observed_at = excluded.last_observed_at;
					`)
					if prepErr == nil {
						defer histStmt.Close()
						for _, p := range toPrune {
							histID := domain.MustNewUUIDv7()
							evidenceKey := fmt.Sprintf("refresh_removed:%s:%s", sub.ID, p.fetchID)
							evidenceJSON := fmt.Sprintf(`{"subscription_id":%q,"subscription_name":%q,"pruned_by_fetch_id":%q,"last_seen_fetch_id":%q}`, sub.ID, sub.Name, fetchID, p.fetchID)
							var firstObs any = nil
							if p.startedAt.Valid && strings.TrimSpace(p.startedAt.String) != "" {
								firstObs = p.startedAt.String
							}
							if _, execErr := histStmt.ExecContext(ctx,
								histID, p.nodeID, sub.ID, "sub:"+sub.ID, sub.Name,
								p.rev, string(domain.RelationStateVerified), string(domain.CauseRefreshRemoved),
								firstObs, nowStr, "subscription_refresh_prune", evidenceKey,
								evidenceJSON, nowStr,
							); execErr != nil {
								return fmt.Errorf("failed to insert history snapshot on refresh prune (node=%s): %w", p.nodeID, execErr)
							}
						}
					}
				}
			}
		}

		// Prune obsolete node_sources associations from current subscription
		const pruneSourcesSQL = `
		DELETE FROM node_sources
		WHERE subscription_id = ? AND last_seen_fetch_id != ?;`

		if _, err := tx.ExecContext(ctx, pruneSourcesSQL, sub.ID, fetchID); err != nil {
			return fmt.Errorf("failed to prune obsolete node sources for sub %s: %w", sub.ID, err)
		}

		// NON-DESTRUCTIVE INVARIANT: Nodes themselves are NEVER deactivated or deleted on refresh pruning.
		// node.active remains intact even when omitted from the latest fetch.
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("reconcile transaction failed: %w", err)
	}

	return &ReconcileResult{
		FetchID:        fetchID,
		SubscriptionID: sub.ID,
		Outcome:        outcome,
		ContentDigest:  fetchResp.ContentDigest,
		NodesParsed:    nodesParsed,
		NodesValid:     nodesValid,
	}, nil
}

// GetNode returns a node by its logical identity.
func (s *Service) GetNode(ctx context.Context, logicalID string) (*domain.Node, error) {
	return s.nodes.GetByLogicalID(ctx, logicalID)
}

// GetNodeDetail returns a node along with all associated provenance sources and current risk summary if available.
func (s *Service) GetNodeDetail(ctx context.Context, logicalID string) (*NodeDetail, error) {
	return s.GetNodeDetailWithRisk(ctx, logicalID, "")
}

// GetNodeDetailWithRisk returns a node detail including API-safe IPRiskSummary, probe status, sources, and recent observations.
func (s *Service) GetNodeDetailWithRisk(ctx context.Context, logicalID string, policyRevisionID string) (*NodeDetail, error) {
	rm, err := s.nodes.GetReadModel(ctx, logicalID, policyRevisionID)
	if err != nil {
		return nil, err
	}
	views := []NodeView{ToNodeViewFromReadModel(*rm)}
	s.enrichNodeViews(ctx, views)
	view := views[0]

	sources := view.Sources
	if len(sources) == 0 && s.sources != nil {
		if byNode, listErr := s.sources.ListByNode(ctx, logicalID); listErr == nil && byNode != nil {
			sources = byNode
			view.Sources = sources
		}
	}
	if sources == nil {
		sources = make([]domain.NodeSource, 0)
	}

	detail := &NodeDetail{
		Node:               rm.Node,
		View:               view,
		Sources:            sources,
		IPRiskSummary:      rm.IPRiskSummary,
		RecentObservations: make([]domain.IPRiskObservation, 0),
		LatencyMS:          view.LatencyMS,
		LastProbedAt:       view.LastProbedAt,
		HealthStatus:       view.HealthStatus,
		ProbeState:         view.ProbeState,
		ProbeMissing:       view.ProbeMissing,
		ProbeStale:         view.ProbeStale,
		Capabilities:       view.Capabilities,
	}
	if s.db != nil {
		obsRepo := sqlite.NewIPRiskObservationRepository(s.db)
		obsList, _, err := obsRepo.List(ctx, domain.IPRiskObservationFilter{
			NodeLogicalID: logicalID,
			Page:          1,
			PageSize:      10,
		})
		if err == nil && obsList != nil {
			detail.RecentObservations = obsList
		}
	}
	return detail, nil
}

// ListNodes queries the node ledger with server-side filtering and pagination.
func (s *Service) ListNodes(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, int, error) {
	return s.nodes.List(ctx, filter)
}

// ListNodesReadModel queries the node projection with server-side risk filtering and returns NodeViews.
func (s *Service) ListNodesReadModel(ctx context.Context, filter domain.NodeFilter) ([]NodeView, int, error) {
	models, total, err := s.nodes.ListReadModel(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := ToNodeViewsFromReadModels(models)
	s.enrichNodeViews(ctx, views)
	return views, total, nil
}

func (s *Service) enrichNodeViews(ctx context.Context, views []NodeView) {
	if len(views) == 0 {
		return
	}
	nodeIDs := make([]string, 0, len(views))
	for i := range views {
		if views[i].HealthStatus == "" {
			views[i].HealthStatus = "unknown"
			views[i].ProbeMissing = true
		}
		if views[i].LogicalID != "" {
			nodeIDs = append(nodeIDs, views[i].LogicalID)
		}
	}
	if len(nodeIDs) == 0 {
		return
	}

	if s.sources != nil {
		if sourcesByNode, err := s.sources.ListByNodes(ctx, nodeIDs); err == nil {
			for i := range views {
				if srcs, ok := sourcesByNode[views[i].LogicalID]; ok && srcs != nil {
					views[i].Sources = srcs
				} else {
					views[i].Sources = make([]domain.NodeSource, 0)
				}
			}
		}
	}

	if s.probeObsRepo != nil {
		if latestByNode, err := s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil); err == nil {
			now := s.now()
			for i := range views {
				applyProbeObservationsToView(&views[i], latestByNode[views[i].LogicalID], now)
			}
		}
	}

	for i := range views {
		if views[i].ProbeState == "" {
			views[i].ProbeState = "idle"
		}
		if s.poolProvider != nil && views[i].LogicalID != "" {
			state := s.poolProvider.GetNodePoolState(views[i].LogicalID)
			switch state {
			case "probing":
				views[i].ProbeState = "probing"
				views[i].HealthStatus = "probing"
			case "queued":
				views[i].ProbeState = "queued"
			default:
				views[i].ProbeState = "idle"
			}
		}
	}
}

func verdictToHealthStatus(verdict domain.ProbeVerdict) string {
	switch strings.ToLower(strings.TrimSpace(string(verdict))) {
	case string(domain.VerdictAvailable), "healthy":
		return "healthy"
	case string(domain.VerdictRestricted), string(domain.VerdictStale), "degraded":
		return "degraded"
	case string(domain.VerdictError), "unreachable", "unhealthy":
		return "unhealthy"
	default:
		return "unknown"
	}
}

func applyProbeObservationsToView(v *NodeView, obsByKind map[domain.ProbeKind]domain.ProbeObservation, now time.Time) {
	if v == nil {
		return
	}
	if len(obsByKind) == 0 {
		v.LatencyMS = nil
		v.LastProbedAt = nil
		v.HealthStatus = string(domain.BaselineUnknown)
		v.ProbeMissing = true
		v.ProbeStale = false
		v.Capabilities = nil
		return
	}

	v.ProbeMissing = false
	caps := make(map[string]NodeCapabilityView, len(obsByKind))

	var baselinePtr *domain.ProbeObservation
	if baselineObs, hasBaseline := obsByKind[domain.ProbeKindBaseline]; hasBaseline {
		obsCopy := baselineObs
		baselinePtr = &obsCopy
	}
	baseHealth, baseLatencyValid := domain.EvaluateBaselineHealth(baselinePtr, v.ConnectionRevision, now, defaultProbeFreshnessTTL)

	var latestAt time.Time
	for kind, obs := range obsByKind {
		revisionMismatch := obs.ConnectionRevision == nil || v.ConnectionRevision < 1 || *obs.ConnectionRevision != v.ConnectionRevision
		ageExpired := obs.ObservedAt.IsZero() || obs.ObservedAt.After(now) || now.Sub(obs.ObservedAt) >= defaultProbeFreshnessTTL
		stale := obs.Verdict == domain.VerdictStale || ageExpired || revisionMismatch

		var capLatency int64
		if kind == domain.ProbeKindBaseline {
			if baseLatencyValid && obs.LatencyMS > 0 {
				capLatency = obs.LatencyMS
			}
		} else {
			status := verdictToHealthStatus(obs.Verdict)
			if !stale && (status == "healthy" || status == "degraded") && obs.LatencyMS > 0 {
				capLatency = obs.LatencyMS
			}
		}

		caps[string(kind)] = NodeCapabilityView{
			Verdict:    obs.Verdict,
			LatencyMS:  capLatency,
			ObservedAt: obs.ObservedAt,
			Summary:    obs.RedactedSummary,
			Stale:      stale,
			Region:     obs.Region,
			SubTier:    obs.SubTier,
			Throughput: obs.Throughput,
			RiskScore:  obs.RiskScore,
			Platforms:  obs.Platforms,
		}
		if obs.ObservedAt.After(latestAt) {
			latestAt = obs.ObservedAt
		}
	}

	v.Capabilities = caps
	if !latestAt.IsZero() {
		ts := latestAt.UTC()
		v.LastProbedAt = &ts
	} else {
		v.LastProbedAt = nil
	}

	if baselineCap, hasBaseline := caps[string(domain.ProbeKindBaseline)]; hasBaseline {
		v.ProbeStale = baselineCap.Stale
	} else if !latestAt.IsZero() {
		v.ProbeStale = latestAt.After(now) || now.Sub(latestAt) >= defaultProbeFreshnessTTL
	} else {
		v.ProbeStale = false
	}

	v.HealthStatus = string(baseHealth)
	if baseLatencyValid && baselinePtr != nil && baselinePtr.LatencyMS > 0 {
		lat := baselinePtr.LatencyMS
		v.LatencyMS = &lat
	} else {
		v.LatencyMS = nil
	}
}

// NodePatchRequest represents a direct plaintext patch payload for a node.
type NodePatchRequest struct {
	DisplayName       *string                           `json:"display_name,omitempty"`
	Server            *string                           `json:"server,omitempty"`
	Port              *int                              `json:"port,omitempty"`
	Credentials       *domain.InboundProtocolCredential `json:"credentials,omitempty"`
	LocalAddress      []string                          `json:"local_address,omitempty"`
	PublicKey         *string                           `json:"public_key,omitempty"`
	PrivateKey        *string                           `json:"private_key,omitempty"`
	PreSharedKey      *string                           `json:"pre_shared_key,omitempty"`
	PresharedKey      *string                           `json:"preshared_key,omitempty"`
	Reserved          []uint8                           `json:"reserved,omitempty"`
	MTU               *int                              `json:"mtu,omitempty"`
	DNS               []string                          `json:"dns,omitempty"`
	UUID              *string                           `json:"uuid,omitempty"`
	Password          *string                           `json:"password,omitempty"`
	Method            *string                           `json:"method,omitempty"`
	AlterID           *int                              `json:"alter_id,omitempty"`
	CongestionControl *string                           `json:"congestion_control,omitempty"`
	UDPRelayMode      *string                           `json:"udp_relay_mode,omitempty"`
	ALPN              []string                          `json:"alpn,omitempty"`
	SNI               *string                           `json:"sni,omitempty"`
	DisableSNI        *bool                             `json:"disable_sni,omitempty"`
	Username          *string                           `json:"username,omitempty"`
	Transport         map[string]string                 `json:"transport,omitempty"`
}

// UpdateNodeConnectionCommand encapsulates a plaintext node update request.
type UpdateNodeConnectionCommand struct {
	LogicalID string
	Patch     NodePatchRequest
	RequestID string
	ActorKind domain.ActorKind
}

// UpdateNodeConnection directly updates plaintext node fields in the nodes table.
func (s *Service) UpdateNodeConnection(ctx context.Context, cmd UpdateNodeConnectionCommand) (*NodeDetail, error) {
	logicalID := strings.TrimSpace(cmd.LogicalID)
	if logicalID == "" {
		return nil, domain.NewValidationError("missing_logical_id", "logical_id is required")
	}

	s.mu.Lock()
	node, err := s.nodes.GetByLogicalID(ctx, logicalID)
	if err != nil {
		s.mu.Unlock()
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "node.connection.update", domain.AuditResultFailure,
			fmt.Sprintf("failed to load node %s: %s", logicalID, domain.RedactSensitiveInfo(err.Error())))
		return nil, err
	}

	updated := *node
	if cmd.Patch.DisplayName != nil {
		trimmed := strings.TrimSpace(*cmd.Patch.DisplayName)
		if trimmed == "" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_display_name", "display_name cannot be empty")
		}
		updated.DisplayName = trimmed
	}
	if cmd.Patch.Server != nil {
		trimmed := strings.TrimSpace(*cmd.Patch.Server)
		if trimmed == "" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_server", "server cannot be empty")
		}
		updated.Server = trimmed
	}
	if cmd.Patch.Port != nil {
		if *cmd.Patch.Port <= 0 || *cmd.Patch.Port > 65535 {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_port", "port must be between 1 and 65535")
		}
		updated.Port = *cmd.Patch.Port
	}

	updatedCreds := updated.Credentials
	if cmd.Patch.Credentials != nil {
		updatedCreds = *cmd.Patch.Credentials
	}
	if updatedCreds.Transport != nil {
		copiedTransport := make(map[string]string, len(updatedCreds.Transport))
		for k, v := range updatedCreds.Transport {
			copiedTransport[k] = v
		}
		updatedCreds.Transport = copiedTransport
	}
	if cmd.Patch.Transport != nil {
		copiedTransport := make(map[string]string, len(cmd.Patch.Transport))
		for k, v := range cmd.Patch.Transport {
			copiedTransport[k] = v
		}
		updatedCreds.Transport = copiedTransport
	}

	if cmd.Patch.LocalAddress != nil {
		addrs := make([]string, 0, len(cmd.Patch.LocalAddress))
		for _, a := range cmd.Patch.LocalAddress {
			trimmed := strings.TrimSpace(a)
			if trimmed == "" {
				continue
			}
			prefix, pErr := netip.ParsePrefix(trimmed)
			if pErr != nil {
				s.mu.Unlock()
				return nil, domain.NewValidationError("invalid_local_address", "WireGuard local_address must be valid CIDR")
			}
			addrs = append(addrs, prefix.String())
		}
		if node.Protocol == domain.ProtocolWireGuard && len(addrs) == 0 {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_local_address", "WireGuard local_address cannot be empty")
		}
		updatedCreds.LocalAddress = addrs
	}
	if cmd.Patch.PublicKey != nil {
		pk := strings.TrimSpace(*cmd.Patch.PublicKey)
		if node.Protocol == domain.ProtocolWireGuard && pk == "" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_public_key", "WireGuard public_key cannot be empty")
		}
		updatedCreds.PublicKey = pk
	}
	if cmd.Patch.PrivateKey != nil {
		pk := strings.TrimSpace(*cmd.Patch.PrivateKey)
		if node.Protocol == domain.ProtocolWireGuard && pk == "" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_private_key", "WireGuard private_key cannot be empty")
		}
		updatedCreds.PrivateKey = pk
	}
	if cmd.Patch.PreSharedKey != nil {
		psk := strings.TrimSpace(*cmd.Patch.PreSharedKey)
		updatedCreds.PreSharedKey = psk
		updatedCreds.PresharedKey = psk
	}
	if cmd.Patch.PresharedKey != nil {
		psk := strings.TrimSpace(*cmd.Patch.PresharedKey)
		updatedCreds.PreSharedKey = psk
		updatedCreds.PresharedKey = psk
	}
	if cmd.Patch.Reserved != nil {
		if len(cmd.Patch.Reserved) > 0 && len(cmd.Patch.Reserved) != 3 {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_reserved", "WireGuard reserved must contain 3 bytes")
		}
		updatedCreds.Reserved = append([]uint8(nil), cmd.Patch.Reserved...)
	}
	if cmd.Patch.MTU != nil {
		if *cmd.Patch.MTU != 0 && (*cmd.Patch.MTU < 576 || *cmd.Patch.MTU > 9000) {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_mtu", "WireGuard mtu must be between 576 and 9000")
		}
		updatedCreds.MTU = *cmd.Patch.MTU
	}
	if cmd.Patch.DNS != nil {
		dnsList := make([]string, 0, len(cmd.Patch.DNS))
		for _, d := range cmd.Patch.DNS {
			if td := strings.TrimSpace(d); td != "" {
				dnsList = append(dnsList, td)
			}
		}
		updatedCreds.DNS = dnsList
	}
	if cmd.Patch.UUID != nil {
		u := strings.TrimSpace(*cmd.Patch.UUID)
		if (node.Protocol == domain.ProtocolTUIC || node.Protocol == domain.ProtocolVMess || node.Protocol == domain.ProtocolVLESS) && u == "" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_uuid", "uuid cannot be empty")
		}
		updatedCreds.UUID = u
	}
	if cmd.Patch.Password != nil {
		pw := strings.TrimSpace(*cmd.Patch.Password)
		if (node.Protocol == domain.ProtocolTUIC || node.Protocol == domain.ProtocolSS || node.Protocol == domain.ProtocolTrojan || node.Protocol == domain.ProtocolHysteria2) && pw == "" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_password", "password cannot be empty")
		}
		updatedCreds.Password = pw
	}
	if cmd.Patch.Method != nil {
		m := strings.TrimSpace(*cmd.Patch.Method)
		if node.Protocol == domain.ProtocolSS && m == "" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_method", "Shadowsocks cipher method cannot be empty")
		}
		updatedCreds.Method = m
	}
	if cmd.Patch.AlterID != nil {
		updatedCreds.AlterID = *cmd.Patch.AlterID
	}
	if cmd.Patch.CongestionControl != nil {
		cc := strings.TrimSpace(*cmd.Patch.CongestionControl)
		if node.Protocol == domain.ProtocolTUIC && cc != "" && cc != "bbr" && cc != "cubic" && cc != "new_reno" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_congestion_control", "TUIC congestion_control must be bbr, cubic, or new_reno")
		}
		updatedCreds.CongestionControl = cc
	}
	if cmd.Patch.UDPRelayMode != nil {
		urm := strings.TrimSpace(*cmd.Patch.UDPRelayMode)
		if node.Protocol == domain.ProtocolTUIC && urm != "" && urm != "native" && urm != "quic" {
			s.mu.Unlock()
			return nil, domain.NewValidationError("invalid_udp_relay_mode", "TUIC udp_relay_mode must be native or quic")
		}
		updatedCreds.UDPRelayMode = urm
	}
	if cmd.Patch.ALPN != nil {
		alpnList := make([]string, 0, len(cmd.Patch.ALPN))
		for _, a := range cmd.Patch.ALPN {
			if ta := strings.TrimSpace(a); ta != "" {
				alpnList = append(alpnList, ta)
			}
		}
		updatedCreds.ALPN = alpnList
	}
	if cmd.Patch.SNI != nil {
		updatedCreds.SNI = strings.TrimSpace(*cmd.Patch.SNI)
	}
	if cmd.Patch.DisableSNI != nil {
		updatedCreds.DisableSNI = *cmd.Patch.DisableSNI
	}
	if cmd.Patch.Username != nil {
		updatedCreds.Username = strings.TrimSpace(*cmd.Patch.Username)
	}

	updated.Credentials = updatedCreds
	updated.UpdatedAt = domain.NowUTC()

	connEqual := areConnectionParametersEqual(
		node.Protocol, updated.Protocol,
		node.Server, updated.Server,
		node.Port, updated.Port,
		node.Credentials, updated.Credentials,
	)

	nowStr := domain.NowUTC().Format(time.RFC3339)

	if !connEqual {
		var currentRev int64 = 1
		if s.db != nil {
			_ = s.db.QueryRowContext(ctx, "SELECT connection_revision FROM node_connection_heads WHERE logical_id = ?;", logicalID).Scan(&currentRev)
			if currentRev <= 0 {
				currentRev = node.ConnectionRevision
				if currentRev <= 0 {
					currentRev = 1
				}
			}
			newRev := currentRev + 1
			updated.ConnectionRevision = newRev

			rawCreds, _ := json.Marshal(updatedCreds)
			configJSON := string(rawCreds)
			effectiveJSONBytes, _ := json.Marshal(map[string]any{
				"server":      updated.Server,
				"port":        updated.Port,
				"credentials": updatedCreds,
			})
			effectiveJSON := string(effectiveJSONBytes)
			fp := domain.ComputeConnectionFingerprint(updated.Server, updated.Port, updatedCreds)

			_ = sqlite.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
				if cmd.Patch.Server != nil {
					sBytes, _ := json.Marshal(*cmd.Patch.Server)
					_, _ = tx.ExecContext(ctx, `
						INSERT INTO node_overrides (node_logical_id, field_path, override_value_json, created_at, updated_at)
						VALUES (?, 'server', ?, ?, ?)
						ON CONFLICT(node_logical_id, field_path) DO UPDATE SET override_value_json = excluded.override_value_json, updated_at = excluded.updated_at;
					`, logicalID, string(sBytes), nowStr, nowStr)
				}
				if cmd.Patch.Port != nil {
					pBytes, _ := json.Marshal(*cmd.Patch.Port)
					_, _ = tx.ExecContext(ctx, `
						INSERT INTO node_overrides (node_logical_id, field_path, override_value_json, created_at, updated_at)
						VALUES (?, 'port', ?, ?, ?)
						ON CONFLICT(node_logical_id, field_path) DO UPDATE SET override_value_json = excluded.override_value_json, updated_at = excluded.updated_at;
					`, logicalID, string(pBytes), nowStr, nowStr)
				}
				if cmd.Patch.SNI != nil {
					sniBytes, _ := json.Marshal(*cmd.Patch.SNI)
					_, _ = tx.ExecContext(ctx, `
						INSERT INTO node_overrides (node_logical_id, field_path, override_value_json, created_at, updated_at)
						VALUES (?, 'sni', ?, ?, ?)
						ON CONFLICT(node_logical_id, field_path) DO UPDATE SET override_value_json = excluded.override_value_json, updated_at = excluded.updated_at;
					`, logicalID, string(sniBytes), nowStr, nowStr)
				}

				_, _ = tx.ExecContext(ctx, `
					INSERT INTO node_connection_versions (node_logical_id, connection_revision, effective_config_json, config_fingerprint, schema_version, created_at)
					VALUES (?, ?, ?, ?, 1, ?)
					ON CONFLICT(node_logical_id, connection_revision) DO UPDATE SET
						effective_config_json = excluded.effective_config_json,
						config_fingerprint = excluded.config_fingerprint;
				`, logicalID, newRev, effectiveJSON, fp, nowStr)

				_, _ = tx.ExecContext(ctx, `
					INSERT INTO node_connection_heads (logical_id, connection_revision, updated_at)
					VALUES (?, ?, ?)
					ON CONFLICT(logical_id) DO UPDATE SET
						connection_revision = excluded.connection_revision,
						updated_at = excluded.updated_at;
				`, logicalID, newRev, nowStr)

				_, _ = tx.ExecContext(ctx, `
					UPDATE nodes SET
						display_name = ?,
						server = ?,
						port = ?,
						config_json = ?,
						updated_at = ?,
						connection_revision = ?
					WHERE logical_id = ?;
				`, updated.DisplayName, updated.Server, updated.Port, configJSON, nowStr, newRev, logicalID)

				return nil
			})
		} else {
			updated.ConnectionRevision = node.ConnectionRevision + 1
		}
	} else {
		updated.ConnectionRevision = node.ConnectionRevision
	}

	upsertErr := s.nodes.UpsertBatch(ctx, []domain.Node{updated})
	s.mu.Unlock()

	if upsertErr != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "node.connection.update", domain.AuditResultFailure,
			fmt.Sprintf("failed to update node %s connection: %s", logicalID, domain.RedactSensitiveInfo(upsertErr.Error())))
		return nil, upsertErr
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "node.connection.update", domain.AuditResultSuccess,
		fmt.Sprintf("updated node %s plaintext connection", logicalID))

	return s.GetNodeDetailWithRisk(ctx, logicalID, "")
}

func applyFieldOverride(node *domain.Node, fieldPath, valJSON string) {
	switch strings.ToLower(fieldPath) {
	case "server":
		var s string
		if json.Unmarshal([]byte(valJSON), &s) == nil && s != "" {
			node.Server = s
		}
	case "port":
		var p int
		if json.Unmarshal([]byte(valJSON), &p) == nil && p > 0 {
			node.Port = p
		}
	case "sni":
		var s string
		if json.Unmarshal([]byte(valJSON), &s) == nil {
			node.Credentials.SNI = s
		}
	case "password":
		var s string
		if json.Unmarshal([]byte(valJSON), &s) == nil {
			node.Credentials.Password = s
		}
	case "uuid":
		var s string
		if json.Unmarshal([]byte(valJSON), &s) == nil {
			node.Credentials.UUID = s
		}
	}
}

// ListLatestEntries returns entries from the latest successful payload for a subscription.
func (s *Service) ListLatestEntries(ctx context.Context, subscriptionID string) ([]domain.SubscriptionEntry, error) {
	if s.entryRepo != nil {
		return s.entryRepo.ListLatestBySubscription(ctx, subscriptionID)
	}
	if s.db != nil {
		repo := sqlite.NewSubscriptionEntryRepository(s.db)
		return repo.ListLatestBySubscription(ctx, subscriptionID)
	}
	return nil, nil
}

// OverrideEntryKind sets or clears a user kind override on an entry.
func (s *Service) OverrideEntryKind(ctx context.Context, entryID string, userKindOverride *domain.EntryKind, reason, actorRef string) (*domain.SubscriptionEntry, error) {
	var repo domain.SubscriptionEntryRepository
	if s.entryRepo != nil {
		repo = s.entryRepo
	} else if s.db != nil {
		repo = sqlite.NewSubscriptionEntryRepository(s.db)
	}
	if repo == nil {
		return nil, domain.NewInternalError("entries_unavailable", "entry repository is not configured")
	}

	entry, err := repo.GetByID(ctx, entryID)
	if err != nil {
		return nil, err
	}

	anchor := entry.SourceKey
	if anchor == "" {
		anchor = fmt.Sprintf("%s|%s|%d|%s", entry.Protocol, entry.Server, entry.Port, entry.RawName)
	}
	now := domain.NowUTC()
	if err := repo.SetUserOverride(ctx, entryID, userKindOverride, anchor, reason, actorRef, now); err != nil {
		return nil, err
	}

	updatedEntry, err := repo.GetByID(ctx, entryID)
	if err != nil {
		return nil, err
	}

	return updatedEntry, nil
}

func (s *Service) recordAudit(ctx context.Context, actorKind domain.ActorKind, requestID, action string, result domain.AuditResult, summary string) {
	if s.auditRepo == nil {
		return
	}
	id, err := domain.NewUUIDv7()
	if err != nil {
		return
	}
	if actorKind == "" {
		actorKind = domain.ActorKindAdmin
	}
	_ = s.auditRepo.Record(ctx, &domain.AuditEvent{
		ID:              id,
		ActorKind:       actorKind,
		RequestID:       requestID,
		Action:          action,
		Result:          result,
		RedactedSummary: domain.RedactSensitiveInfo(summary),
		CreatedAt:       domain.NowUTC(),
	})
}

// CurrentSourceView represents a currently attached live subscription source.
type CurrentSourceView struct {
	SubscriptionID string `json:"subscription_id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
}

// NodeSourceHistoryItemView represents a safe, redacted historical observation item for a node.
type NodeSourceHistoryItemView struct {
	SourceLabel        string                  `json:"source_label"`
	SubscriptionID     *string                 `json:"subscription_id,omitempty"`
	RelationState      domain.RelationState    `json:"relation_state"`
	Cause              domain.AttributionCause `json:"cause"`
	EvidenceKind       string                  `json:"evidence_kind"`
	FirstObservedAt    *string                 `json:"first_observed_at,omitempty"`
	LastObservedAt     *string                 `json:"last_observed_at,omitempty"`
	ConnectionRevision *int64                  `json:"connection_revision,omitempty"`
	SourceDeleted      bool                    `json:"source_deleted"`
	SourceUnmapped     bool                    `json:"source_unmapped"`
}

// NodeSourceHistoryResponseData represents the payload for GET /api/v1/nodes/{id}/source-history.
type NodeSourceHistoryResponseData struct {
	CurrentSources    []CurrentSourceView         `json:"current_sources"`
	History           []NodeSourceHistoryItemView `json:"history"`
	AttributionStatus domain.AttributionStatus    `json:"attribution_status"`
}

// GetNodeSourceHistory retrieves current subscription sources and all historical provenance ledger entries for a node.
func (s *Service) GetNodeSourceHistory(ctx context.Context, logicalID string) (*NodeSourceHistoryResponseData, error) {
	logicalID = strings.TrimSpace(logicalID)
	if logicalID == "" {
		return nil, domain.NewValidationError("missing_logical_id", "node logical_id is required")
	}

	// 1. Verify node exists in database
	_, err := s.nodes.GetByLogicalID(ctx, logicalID)
	if err != nil {
		return nil, err
	}

	// 2. Query current live sources
	currentSources := make([]CurrentSourceView, 0)
	if s.db != nil {
		rows, qErr := s.db.QueryContext(ctx, `
			SELECT ns.subscription_id, COALESCE(s.name, ''), COALESCE(s.enabled, 0)
			FROM node_sources ns
			LEFT JOIN subscriptions s ON ns.subscription_id = s.id
			WHERE ns.node_logical_id = ?
			ORDER BY ns.subscription_id ASC;
		`, logicalID)
		if qErr == nil {
			for rows.Next() {
				var (
					subID   string
					subName string
					enabled int
				)
				if scanErr := rows.Scan(&subID, &subName, &enabled); scanErr == nil {
					currentSources = append(currentSources, CurrentSourceView{
						SubscriptionID: subID,
						Name:           subName,
						Enabled:        enabled == 1,
					})
				}
			}
			rows.Close()
		}
	}

	// 3. Query history ledger
	var historyRecords []domain.NodeSourceHistory
	if s.sourceHistoryRepo != nil {
		historyRecords, err = s.sourceHistoryRepo.ListByNodeLogicalID(ctx, logicalID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch node source history: %w", err)
		}
	} else if s.db != nil {
		repo := sqlite.NewNodeSourceHistoryRepository(s.db)
		historyRecords, err = repo.ListByNodeLogicalID(ctx, logicalID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch node source history: %w", err)
		}
	}

	// 4. Map history items and evaluate source_deleted
	existingSubs := make(map[string]bool)
	if s.subscriptions != nil {
		allSubs, _, subErr := s.subscriptions.List(ctx, domain.SubscriptionFilter{
			Pagination: domain.Pagination{Page: 1, PageSize: 1000},
		})
		if subErr == nil {
			for _, sub := range allSubs {
				existingSubs[sub.ID] = true
			}
		}
	}

	historyViews := make([]NodeSourceHistoryItemView, 0, len(historyRecords))
	for _, rec := range historyRecords {
		var sourceDeleted bool
		var sourceUnmapped bool

		if rec.SubscriptionID == nil || *rec.SubscriptionID == "" {
			if rec.Cause == domain.CauseSubscriptionDeleted {
				sourceDeleted = true
				sourceUnmapped = false
			} else {
				sourceDeleted = false
				sourceUnmapped = true
			}
		} else {
			if rec.Cause == domain.CauseSubscriptionDeleted || !existingSubs[*rec.SubscriptionID] {
				sourceDeleted = true
				sourceUnmapped = false
			} else {
				sourceDeleted = false
				sourceUnmapped = false
			}
		}

		var firstObs *string
		if rec.FirstObservedAt != nil && !rec.FirstObservedAt.IsZero() {
			str := rec.FirstObservedAt.UTC().Format(time.RFC3339)
			firstObs = &str
		}
		var lastObs *string
		if rec.LastObservedAt != nil && !rec.LastObservedAt.IsZero() {
			str := rec.LastObservedAt.UTC().Format(time.RFC3339)
			lastObs = &str
		}

		historyViews = append(historyViews, NodeSourceHistoryItemView{
			SourceLabel:        rec.SourceLabel,
			SubscriptionID:     rec.SubscriptionID,
			RelationState:      rec.RelationState,
			Cause:              rec.Cause,
			EvidenceKind:       rec.EvidenceKind,
			FirstObservedAt:    firstObs,
			LastObservedAt:     lastObs,
			ConnectionRevision: rec.ConnectionRevision,
			SourceDeleted:      sourceDeleted,
			SourceUnmapped:     sourceUnmapped,
		})
	}

	// 5. Aggregate status
	hasCurrent := len(currentSources) > 0
	aggStatus := domain.AggregateAttributionStatus(hasCurrent, historyRecords)

	return &NodeSourceHistoryResponseData{
		CurrentSources:    currentSources,
		History:           historyViews,
		AttributionStatus: aggStatus,
	}, nil
}

