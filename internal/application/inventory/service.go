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

// NodeDetail pairs a normalized node with its provenance sources and API-safe risk summary.
type NodeDetail struct {
	Node               domain.Node                `json:"node"`
	Sources            []domain.NodeSource        `json:"sources"`
	IPRiskSummary      *domain.IPRiskSummary      `json:"ip_risk_summary,omitempty"`
	RecentObservations []domain.IPRiskObservation `json:"recent_observations,omitempty"`
}

// NodeView is the management API view of a Node including plaintext server, port, and protocol credentials.
type NodeView struct {
	LogicalID     string                            `json:"logical_id"`
	Protocol      domain.Protocol                   `json:"protocol"`
	DisplayName   string                            `json:"display_name"`
	Server        string                            `json:"server,omitempty"`
	Port          int                               `json:"port,omitempty"`
	Credentials   *domain.InboundProtocolCredential `json:"credentials,omitempty"`
	Active        bool                              `json:"active"`
	CreatedAt     time.Time                         `json:"created_at"`
	UpdatedAt     time.Time                         `json:"updated_at"`
	IPRiskSummary *domain.IPRiskSummary             `json:"ip_risk_summary,omitempty"`
}

// ToNodeView converts a domain.Node to a NodeView.
func ToNodeView(n domain.Node) NodeView {
	creds := n.Credentials
	return NodeView{
		LogicalID:   n.LogicalID,
		Protocol:    n.Protocol,
		DisplayName: n.DisplayName,
		Server:      n.Server,
		Port:        n.Port,
		Credentials: &creds,
		Active:      n.Active,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
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

// Service coordinates inventory ingestion, provenance reconciliation, and ledger queries.
type Service struct {
	db            *sql.DB
	subscriptions domain.SubscriptionRepository
	fetches       domain.SubscriptionFetchRepository
	nodes         domain.NodeRepository
	sources       domain.NodeSourceRepository
	fetcher       fetch.Fetcher
	probeObsRepo  domain.ProbeObservationRepository
	auditRepo     domain.AuditRepository
	mu            sync.Mutex
}

// Reconciler is an alias for Service to satisfy reconciler role expectations.
type Reconciler = Service

// Option configures Service dependencies.
type Option func(*Service)

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
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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
	opts := fetch.OptionsFromPolicy(sub.SourceURLSecretRef, sub.RefreshPolicy, sub.RefreshPolicy.FetchProxyRef)
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

		return &ReconcileResult{
			FetchID:        fetchID,
			SubscriptionID: sub.ID,
			Outcome:        domain.FetchOutcomeFailed,
			ContentDigest:  fetchResp.ContentDigest,
			RedactedError:  redactedErr,
		}, fmt.Errorf("failed to parse subscription %s content: %w", sub.ID, extractErr)
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

		const upsertNodeSQL = `
		INSERT INTO nodes (logical_id, protocol, display_name, server, port, config_json, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(logical_id) DO UPDATE SET
			protocol = excluded.protocol,
			display_name = excluded.display_name,
			server = excluded.server,
			port = excluded.port,
			config_json = excluded.config_json,
			active = 1,
			updated_at = excluded.updated_at;`

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

		seenLogicalIDs := make(map[string]bool)
		for _, item := range extractResult.Items {
			node := item.Normalized.Node
			rawCreds, mErr := json.Marshal(node.Credentials)
			if mErr != nil {
				return fmt.Errorf("failed to marshal credentials for node %s: %w", node.LogicalID, mErr)
			}
			configJSON := string(rawCreds)
			if configJSON == "" {
				configJSON = "{}"
			}

			if _, err := nodeStmt.ExecContext(ctx,
				node.LogicalID,
				string(node.Protocol),
				node.DisplayName,
				node.Server,
				node.Port,
				configJSON,
				nowStr,
				nowStr,
			); err != nil {
				return fmt.Errorf("failed to upsert node %s: %w", node.LogicalID, err)
			}

			if !seenLogicalIDs[node.LogicalID] {
				seenLogicalIDs[node.LogicalID] = true
				if _, err := sourceStmt.ExecContext(ctx,
					node.LogicalID,
					sub.ID,
					fetchID,
				); err != nil {
					return fmt.Errorf("failed to upsert node source for node %s: %w", node.LogicalID, err)
				}
			}
		}

		const pruneSourcesSQL = `
		DELETE FROM node_sources
		WHERE subscription_id = ? AND last_seen_fetch_id != ?;`

		if _, err := tx.ExecContext(ctx, pruneSourcesSQL, sub.ID, fetchID); err != nil {
			return fmt.Errorf("failed to prune obsolete node sources: %w", err)
		}

		const deactivateOrphansSQL = `
		UPDATE nodes
		SET active = 0, updated_at = ?
		WHERE active = 1
		  AND NOT EXISTS (
			  SELECT 1 FROM node_sources WHERE node_sources.node_logical_id = nodes.logical_id
		  );`

		if _, err := tx.ExecContext(ctx, deactivateOrphansSQL, nowStr); err != nil {
			return fmt.Errorf("failed to deactivate orphan nodes: %w", err)
		}

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

// GetNodeDetailWithRisk returns a node detail including API-safe IPRiskSummary and recent observations.
func (s *Service) GetNodeDetailWithRisk(ctx context.Context, logicalID string, policyRevisionID string) (*NodeDetail, error) {
	rm, err := s.nodes.GetReadModel(ctx, logicalID, policyRevisionID)
	if err != nil {
		return nil, err
	}
	sources, err := s.sources.ListByNode(ctx, logicalID)
	if err != nil {
		return nil, err
	}
	if sources == nil {
		sources = make([]domain.NodeSource, 0)
	}
	detail := &NodeDetail{
		Node:               rm.Node,
		Sources:            sources,
		IPRiskSummary:      rm.IPRiskSummary,
		RecentObservations: make([]domain.IPRiskObservation, 0),
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
	return ToNodeViewsFromReadModels(models), total, nil
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
