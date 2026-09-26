// Package inventory orchestrates node ingestion, provenance tracking, and set-difference reconciliation.
package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"reflect"
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
	Connection         *domain.SafeNodeConnection `json:"connection,omitempty"`
	IPRiskSummary      *domain.IPRiskSummary      `json:"ip_risk_summary,omitempty"`
	RecentObservations []domain.IPRiskObservation `json:"recent_observations,omitempty"`
	CredentialMismatch bool                       `json:"credential_mismatch,omitempty"`
}

// NodeView is the API-safe view of a Node without internal secret references.
type NodeView struct {
	LogicalID          string                     `json:"logical_id"`
	Protocol           domain.Protocol            `json:"protocol"`
	DisplayName        string                     `json:"display_name"`
	Active             bool                       `json:"active"`
	CredentialVersion  int                        `json:"credential_version"`
	CredentialMismatch bool                       `json:"credential_mismatch,omitempty"`
	Connection         *domain.SafeNodeConnection `json:"connection,omitempty"`
	CreatedAt          time.Time                  `json:"created_at"`
	UpdatedAt          time.Time                  `json:"updated_at"`
	IPRiskSummary      *domain.IPRiskSummary      `json:"ip_risk_summary,omitempty"`
}

// ToNodeView converts a domain.Node to an API-safe NodeView, stripping secret references.
func ToNodeView(n domain.Node) NodeView {
	return NodeView{
		LogicalID:         n.LogicalID,
		Protocol:          n.Protocol,
		DisplayName:       n.DisplayName,
		Active:            n.Active,
		CredentialVersion: n.CredentialVersion,
		CreatedAt:         n.CreatedAt,
		UpdatedAt:         n.UpdatedAt,
	}
}

// ToNodeViewFromReadModel converts a domain.NodeReadModel to an API-safe NodeView.
func ToNodeViewFromReadModel(rm domain.NodeReadModel) NodeView {
	return NodeView{
		LogicalID:         rm.Node.LogicalID,
		Protocol:          rm.Node.Protocol,
		DisplayName:       rm.Node.DisplayName,
		Active:            rm.Node.Active,
		CredentialVersion: rm.Node.CredentialVersion,
		CreatedAt:         rm.Node.CreatedAt,
		UpdatedAt:         rm.Node.UpdatedAt,
		IPRiskSummary:     rm.IPRiskSummary,
	}
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
type credentialTransactionRepository interface {
	GetLatestByLogicalIDTx(ctx context.Context, tx *sql.Tx, logicalID string) (*domain.NodeCredentialRecord, error)
}

type Service struct {
	db            *sql.DB
	subscriptions domain.SubscriptionRepository
	fetches       domain.SubscriptionFetchRepository
	nodes         domain.NodeRepository
	sources       domain.NodeSourceRepository
	fetcher       fetch.Fetcher
	vault         *domain.NodeCredentialVault
	credRepo      domain.NodeCredentialRepository
	probeObsRepo  domain.ProbeObservationRepository
	auditRepo     domain.AuditRepository
	mu            sync.Mutex
}

// Reconciler is an alias for Service to satisfy reconciler role expectations.
type Reconciler = Service

// Option configures Service dependencies.
type Option func(*Service)

// WithCredentialVault sets the vault and repository for encrypted node credential storage.
func WithCredentialVault(vault *domain.NodeCredentialVault, credRepo domain.NodeCredentialRepository) Option {
	return func(s *Service) {
		s.vault = vault
		s.credRepo = credRepo
	}
}

// WithProbeObservationRepository sets the probe observation repository for credential mismatch detection.
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
// Lifecycle steps:
// 1. Retrieve subscription snapshot and verify enabled state.
// 2. Outside transaction: safely fetch subscription content under SSRF and resource limits.
// 3. Outside transaction: parse and normalize nodes in memory, deriving stable logical_ids.
// 4. In a single SQLite short transaction (sqlite.WithTx):
//   - Record subscription_fetches audit entry.
//   - Upsert nodes table (active = 1).
//   - Update node_sources table with last_seen_fetch_id.
//   - Prune obsolete node_sources for this subscription (last_seen_fetch_id != currentFetchID).
//   - Deactivate nodes that have lost all valid sources (active = 0).
//
// 5. Commit transaction.
// Failure protection: On network or parser errors, no ledger changes are committed, only an audit record is saved.
func (s *Service) ReconcileSubscription(ctx context.Context, subID string) (*ReconcileResult, error) {
	// Step 1: Preflight subscription snapshot and revision check
	sub, err := s.subscriptions.GetByID(ctx, subID)
	if err != nil {
		return nil, err
	}
	if !sub.Enabled {
		return nil, domain.NewValidationError("subscription_disabled", "disabled subscriptions cannot be refreshed")
	}

	// Step 2: Outside transaction - safe fetch under SSRF policy
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

	// Step 3: Outside transaction - in-memory extract and normalize
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

	// Step 4: In a single SQLite short transaction - atomic convergence
	fetchID := domain.MustNewUUIDv7()
	finishedAt := domain.NowUTC()
	nowStr := domain.NowUTC().Format(time.RFC3339)

	s.mu.Lock()
	defer s.mu.Unlock()

	err = sqlite.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		// 1. Record subscription_fetches audit entry
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

		// 2. Batch upsert nodes table
		const upsertNodeSQL = `
		INSERT INTO nodes (logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(logical_id) DO UPDATE SET
			protocol = excluded.protocol,
			display_name = excluded.display_name,
			normalized_config_secret_ref = excluded.normalized_config_secret_ref,
			credential_version = excluded.credential_version,
			active = 1,
			updated_at = excluded.updated_at;`

		nodeStmt, err := tx.PrepareContext(ctx, upsertNodeSQL)
		if err != nil {
			return fmt.Errorf("failed to prepare upsert node statement: %w", err)
		}
		defer nodeStmt.Close()

		// 3. Update node_sources table
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

		const queryNodeVersionSQL = `
		SELECT credential_version
		FROM nodes
		WHERE logical_id = ?;`

		queryNodeVersionStmt, err := tx.PrepareContext(ctx, queryNodeVersionSQL)
		if err != nil {
			return fmt.Errorf("failed to prepare query node version statement: %w", err)
		}
		defer queryNodeVersionStmt.Close()

		seenLogicalIDs := make(map[string]bool)
		for _, item := range extractResult.Items {
			node := item.Normalized.Node

			// Determine node's credential_version and encrypted credential payload
			targetVersion := 0
			needEncrypt := false

			// If vault & credRepo configured, persist AEAD encrypted credentials in the same transaction
			if s.vault != nil {
				// Query current node row credential_version in this transaction
				currentNodeVersion := 0
				var vRow int
				if qvErr := queryNodeVersionStmt.QueryRowContext(ctx, node.LogicalID).Scan(&vRow); qvErr == nil {
					currentNodeVersion = vRow
				} else if !errors.Is(qvErr, sql.ErrNoRows) {
					return fmt.Errorf("failed to query current version for node %s: %w", node.LogicalID, qvErr)
				}

				// Query latest credential version within this transaction to avoid cross-connection race and lock conflicts
				var existingRecord *domain.NodeCredentialRecord
				if s.credRepo != nil {
					txRepo, ok := s.credRepo.(credentialTransactionRepository)
					if !ok {
						return fmt.Errorf("credential repository does not support transaction-scoped reads")
					}
					rec, qErr := txRepo.GetLatestByLogicalIDTx(ctx, tx, node.LogicalID)
					if qErr != nil {
						var de *domain.DomainError
						if !errors.Is(qErr, sql.ErrNoRows) && (!errors.As(qErr, &de) || de.Category != domain.CategoryNotFound) {
							return fmt.Errorf("failed to query existing credentials for node %s: %w", node.LogicalID, qErr)
						}
					} else {
						existingRecord = rec
					}
				}

				// High watermark version across existing credential record and node row
				baseVersion := currentNodeVersion
				if existingRecord != nil && existingRecord.Version > baseVersion {
					baseVersion = existingRecord.Version
				}

				shouldIncrement := false

				if existingRecord != nil {
					existingPayload, decErr := s.vault.Decrypt(existingRecord, node.Protocol)
					if decErr != nil {
						// Existing payload cannot be decrypted with current keys/AAD or is corrupted.
						// Fail closed: do NOT silently mask decryption errors or bump version.
						// Abort transaction and return a redacted error.
						redactedErr := domain.RedactSensitiveInfo(decErr.Error())
						return fmt.Errorf("failed to decrypt existing credentials for node %s: %s", node.LogicalID, redactedErr)
					}
					// Decrypted successfully, check if endpoint, identity binding, or credentials changed.
					// Whole-subscription ContentDigest change alone MUST NOT bump single node version.
					expectedIdentity := domain.NewVerifiedNodeIdentity(
						node.LogicalID,
						node.Protocol,
						item.Normalized.Server,
						item.Normalized.Port,
						existingRecord.Version,
						item.Normalized.Transport,
					)
					identityMissingOrChanged := existingPayload.Identity == nil ||
						existingPayload.Identity.ValidateNonEmpty() != nil ||
						!existingPayload.Identity.MatchesEndpoint(node.Protocol, item.Normalized.Server, item.Normalized.Port) ||
						existingPayload.Identity.TransportDigest != expectedIdentity.TransportDigest

					credsChanged := !reflect.DeepEqual(existingPayload.Credentials, item.Credentials)
					serverChanged := existingPayload.Server != item.Normalized.Server || existingPayload.Port != item.Normalized.Port
					protocolChanged := existingPayload.Protocol != node.Protocol

					if credsChanged || serverChanged || protocolChanged || identityMissingOrChanged {
						shouldIncrement = true
						needEncrypt = true
						targetVersion = baseVersion + 1
					} else {
						// Reusing existing credentials with identical connection configuration
						targetVersion = existingRecord.Version
					}
				} else {
					// First time ingestion with vault configured
					shouldIncrement = true
					needEncrypt = true
					if baseVersion > 0 {
						targetVersion = baseVersion + 1
					} else {
						targetVersion = 1
					}
				}
				_ = shouldIncrement
			}

			// 1) Upsert node first so that node_credentials foreign key constraint is satisfied
			if _, err := nodeStmt.ExecContext(ctx,
				node.LogicalID,
				string(node.Protocol),
				node.DisplayName,
				node.NormalizedConfigSecretRef,
				targetVersion,
				nowStr,
				nowStr,
			); err != nil {
				return fmt.Errorf("failed to upsert node %s: %w", node.LogicalID, err)
			}

			// 2) If needEncrypt, persist encrypted credential with verified identity binding
			if needEncrypt {
				identity := domain.NewVerifiedNodeIdentity(
					node.LogicalID,
					node.Protocol,
					item.Normalized.Server,
					item.Normalized.Port,
					targetVersion,
					item.Normalized.Transport,
				)
				payload := &domain.NodeCredentialPayload{
					LogicalID:   node.LogicalID,
					Protocol:    node.Protocol,
					Server:      item.Normalized.Server,
					Port:        item.Normalized.Port,
					Version:     targetVersion,
					Digest:      fetchResp.ContentDigest,
					Identity:    &identity,
					Credentials: item.Credentials,
				}
				record, encErr := s.vault.Encrypt(payload)
				if encErr != nil {
					return fmt.Errorf("failed to encrypt node credential for %s: %w", node.LogicalID, encErr)
				}

				const upsertCredSQL = `
				INSERT INTO node_credentials (logical_id, version, key_id, nonce, ciphertext, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(logical_id, version) DO UPDATE SET
					key_id = excluded.key_id,
					nonce = excluded.nonce,
					ciphertext = excluded.ciphertext,
					updated_at = excluded.updated_at;`

				if _, err := tx.ExecContext(ctx, upsertCredSQL,
					record.LogicalID,
					record.Version,
					record.KeyID,
					record.Nonce,
					record.Ciphertext,
					nowStr,
					nowStr,
				); err != nil {
					return fmt.Errorf("failed to persist encrypted credential for node %s: %w", node.LogicalID, err)
				}
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

		// 4. Clean up association records under current subscription where last_seen_fetch_id != fetchID
		const pruneSourcesSQL = `
		DELETE FROM node_sources
		WHERE subscription_id = ? AND last_seen_fetch_id != ?;`

		if _, err := tx.ExecContext(ctx, pruneSourcesSQL, sub.ID, fetchID); err != nil {
			return fmt.Errorf("failed to prune obsolete node sources for sub %s: %w", sub.ID, err)
		}

		// 5. Deactivate nodes that have lost all valid sources (no records in node_sources)
		// and clean up credentials of deactivated nodes
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

		const cleanOrphanCredsSQL = `
		DELETE FROM node_credentials
		WHERE logical_id IN (
			SELECT logical_id FROM nodes WHERE active = 0
		);`

		if _, err := tx.ExecContext(ctx, cleanOrphanCredsSQL); err != nil {
			return fmt.Errorf("failed to clean orphan credentials: %w", err)
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
	conn := s.projectNodeConnection(ctx, rm.Node)
	detail := &NodeDetail{
		Node:               rm.Node,
		Sources:            sources,
		Connection:         &conn,
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
	if s.probeObsRepo != nil {
		if latest, err := s.probeObsRepo.ListLatestByNodes(ctx, []string{logicalID}, nil); err == nil {
			if kindMap, ok := latest[logicalID]; ok {
				for _, obs := range kindMap {
					if obs.CredentialVersion != nil && *obs.CredentialVersion != rm.Node.CredentialVersion {
						detail.CredentialMismatch = true
						break
					}
				}
			}
		}
	}
	return detail, nil
}

// ListNodes queries the node ledger with server-side filtering and pagination.
func (s *Service) ListNodes(ctx context.Context, filter domain.NodeFilter) ([]domain.Node, int, error) {
	return s.nodes.List(ctx, filter)
}

// ListNodesReadModel queries the node projection with server-side risk filtering and returns API-safe NodeViews.
func (s *Service) ListNodesReadModel(ctx context.Context, filter domain.NodeFilter) ([]NodeView, int, error) {
	models, total, err := s.nodes.ListReadModel(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := ToNodeViewsFromReadModels(models)
	if s.probeObsRepo != nil && len(views) > 0 {
		nodeIDs := make([]string, len(views))
		for i, v := range views {
			nodeIDs[i] = v.LogicalID
		}
		if latest, err := s.probeObsRepo.ListLatestByNodes(ctx, nodeIDs, nil); err == nil {
			for i := range views {
				if kindMap, ok := latest[views[i].LogicalID]; ok {
					for _, obs := range kindMap {
						if obs.CredentialVersion != nil && *obs.CredentialVersion != views[i].CredentialVersion {
							views[i].CredentialMismatch = true
							break
						}
					}
				}
			}
		}
	}
	return views, total, nil
}

func (s *Service) projectNodeConnection(ctx context.Context, node domain.Node) domain.SafeNodeConnection {
	if s.vault == nil || s.credRepo == nil {
		return domain.UnavailableNodeConnection("vault_unconfigured")
	}
	if node.CredentialVersion <= 0 {
		return domain.UnavailableNodeConnection("credential_unavailable")
	}
	record, err := s.credRepo.GetByLogicalID(ctx, node.LogicalID, node.CredentialVersion)
	if err != nil || record == nil {
		return domain.UnavailableNodeConnection("credential_unavailable")
	}
	payload, decErr := s.vault.Decrypt(record, node.Protocol)
	if decErr != nil || payload == nil {
		return domain.UnavailableNodeConnection("credential_decryption_failed")
	}
	if err := verifyCredentialIdentity(node, payload); err != nil {
		return domain.UnavailableNodeConnection("identity_verification_failed")
	}
	return domain.ProjectSafeNodeConnection(payload)
}

func verifyCredentialIdentity(node domain.Node, payload *domain.NodeCredentialPayload) error {
	if payload == nil {
		return domain.NewValidationError("missing_credential_payload", "credential payload is nil")
	}
	if payload.LogicalID != node.LogicalID || payload.Protocol != node.Protocol || payload.Version != node.CredentialVersion {
		return domain.NewConflictError("identity_binding_invalid", "credential envelope does not match node metadata")
	}
	if payload.Identity == nil {
		return domain.NewConflictError("identity_binding_missing", "credential record lacks verified identity binding")
	}
	if err := payload.Identity.ValidateNonEmpty(); err != nil {
		return domain.NewConflictError("identity_binding_invalid", "credential verified identity binding is incomplete")
	}
	if payload.Identity.LogicalID != node.LogicalID || payload.Identity.Protocol != node.Protocol || payload.Identity.Version != node.CredentialVersion {
		return domain.NewConflictError("identity_binding_invalid", "credential verified identity binding does not match node version")
	}
	if !payload.Identity.MatchesEndpoint(node.Protocol, payload.Server, payload.Port) {
		return domain.NewConflictError("identity_binding_invalid", "credential endpoint does not match verified identity binding")
	}
	if strings.TrimSpace(payload.Identity.TransportDigest) == "" {
		return domain.NewConflictError("identity_binding_invalid", "credential verified identity binding lacks transport digest")
	}
	if payload.Credentials.Transport != nil && payload.Identity.TransportDigest != domain.CanonicalTransportDigest(payload.Credentials.Transport) {
		return domain.NewConflictError("identity_binding_invalid", "credential transport digest does not match verified identity binding")
	}
	if strings.HasPrefix(node.LogicalID, "node_") && domain.IsValidLogicalID(node.LogicalID) {
		idWithTransport := domain.ComputeNodeLogicalID(payload.Protocol, payload.Server, payload.Port, payload.Credentials.Transport)
		idWithoutTransport := domain.ComputeNodeLogicalID(payload.Protocol, payload.Server, payload.Port, nil)
		if idWithTransport != node.LogicalID && idWithoutTransport != node.LogicalID {
			return domain.NewConflictError("identity_binding_invalid", "credential endpoint does not match canonical node logical identity")
		}
	}
	return nil
}

// UpdateNodeConnectionCommand encapsulates a CAS-protected node connection update and optional secret rotation.
type UpdateNodeConnectionCommand struct {
	LogicalID string
	Patch     domain.NodeConnectionPatchRequest
	RequestID string
	ActorKind domain.ActorKind
}

// UpdateNodeConnection validates and persists non-identity connection edits and write-only secret rotations.
func (s *Service) UpdateNodeConnection(ctx context.Context, cmd UpdateNodeConnectionCommand) (*NodeDetail, error) {
	logicalID := strings.TrimSpace(cmd.LogicalID)
	if logicalID == "" {
		return nil, domain.NewValidationError("missing_logical_id", "logical_id is required")
	}
	if cmd.Patch.ExpectedCredentialVersion < 1 {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "node.connection.update", domain.AuditResultFailure,
			fmt.Sprintf("rejected node %s connection update: missing expected_credential_version", logicalID))
		return nil, domain.NewValidationError("missing_expected_credential_version", "expected_credential_version must be >= 1")
	}
	if s.vault == nil || s.credRepo == nil || s.db == nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "node.connection.update", domain.AuditResultFailure,
			fmt.Sprintf("rejected node %s connection update: credential vault unavailable", logicalID))
		return nil, domain.NewConflictError("credential_vault_unavailable", "node credential vault is not configured")
	}

	s.mu.Lock()
	var oldVersion, newVersion int
	var rotatedCount int
	txErr := sqlite.WithTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		var node domain.Node
		var activeInt int
		var createdStr, updatedStr string
		err := tx.QueryRowContext(ctx, `
			SELECT logical_id, protocol, display_name, normalized_config_secret_ref, credential_version, active, created_at, updated_at
			FROM nodes WHERE logical_id = ?;`, logicalID).Scan(
			&node.LogicalID, &node.Protocol, &node.DisplayName, &node.NormalizedConfigSecretRef,
			&node.CredentialVersion, &activeInt, &createdStr, &updatedStr,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NewNotFoundError("node_not_found", fmt.Sprintf("node %s not found", logicalID))
			}
			return fmt.Errorf("failed to query node %s: %w", logicalID, err)
		}
		node.Active = activeInt == 1
		oldVersion = node.CredentialVersion
		if node.CredentialVersion <= 0 {
			return domain.NewNotFoundError("node_credential_not_found", "node credentials are unavailable")
		}
		if cmd.Patch.ExpectedCredentialVersion != node.CredentialVersion {
			return domain.NewConflictError("credential_version_conflict",
				fmt.Sprintf("expected credential version %d does not match current version %d", cmd.Patch.ExpectedCredentialVersion, node.CredentialVersion))
		}

		var rec domain.NodeCredentialRecord
		var recCreated, recUpdated string
		err = tx.QueryRowContext(ctx, `
			SELECT logical_id, version, key_id, nonce, ciphertext, created_at, updated_at
			FROM node_credentials WHERE logical_id = ? AND version = ?;`, logicalID, node.CredentialVersion).Scan(
			&rec.LogicalID, &rec.Version, &rec.KeyID, &rec.Nonce, &rec.Ciphertext, &recCreated, &recUpdated,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.NewNotFoundError("node_credential_not_found", "node credentials are unavailable")
			}
			return fmt.Errorf("failed to query node credentials: %w", err)
		}

		currentPayload, decErr := s.vault.Decrypt(&rec, node.Protocol)
		if decErr != nil || currentPayload == nil {
			return domain.NewSecurityError("credential_decryption_failed", "failed to authenticate or decrypt existing node credentials")
		}
		if err := verifyCredentialIdentity(node, currentPayload); err != nil {
			return err
		}
		if err := cmd.Patch.ValidateAgainstBaseline(*currentPayload.Identity, currentPayload); err != nil {
			return err
		}

		updatedDisplayName := node.DisplayName
		if cmd.Patch.DisplayName != nil {
			trimmed := strings.TrimSpace(*cmd.Patch.DisplayName)
			if trimmed == "" {
				return domain.NewValidationError("invalid_display_name", "display_name cannot be empty")
			}
			updatedDisplayName = trimmed
		}

		updatedCreds := currentPayload.Credentials
		if currentPayload.Credentials.Transport != nil {
			copiedTransport := make(map[string]string, len(currentPayload.Credentials.Transport))
			for k, v := range currentPayload.Credentials.Transport {
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
					return domain.NewValidationError("invalid_local_address", "WireGuard local_address must be valid CIDR")
				}
				addrs = append(addrs, prefix.String())
			}
			if node.Protocol == domain.ProtocolWireGuard && len(addrs) == 0 {
				return domain.NewValidationError("invalid_local_address", "WireGuard local_address cannot be empty")
			}
			updatedCreds.LocalAddress = addrs
		}
		if cmd.Patch.PublicKey != nil {
			pk := strings.TrimSpace(*cmd.Patch.PublicKey)
			if node.Protocol == domain.ProtocolWireGuard && pk == "" {
				return domain.NewValidationError("invalid_public_key", "WireGuard public_key cannot be empty")
			}
			updatedCreds.PublicKey = pk
		}
		if cmd.Patch.Reserved != nil {
			updatedCreds.Reserved = append([]uint8(nil), cmd.Patch.Reserved...)
		}
		if cmd.Patch.MTU != nil {
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
				return domain.NewValidationError("invalid_uuid", "uuid cannot be empty")
			}
			updatedCreds.UUID = u
		}
		if cmd.Patch.CongestionControl != nil {
			cc := strings.TrimSpace(*cmd.Patch.CongestionControl)
			if node.Protocol == domain.ProtocolTUIC && cc != "" && cc != "bbr" && cc != "cubic" && cc != "new_reno" {
				return domain.NewValidationError("invalid_congestion_control", "TUIC congestion_control must be bbr, cubic, or new_reno")
			}
			updatedCreds.CongestionControl = cc
		}
		if cmd.Patch.UDPRelayMode != nil {
			urm := strings.TrimSpace(*cmd.Patch.UDPRelayMode)
			if node.Protocol == domain.ProtocolTUIC && urm != "" && urm != "native" && urm != "quic" {
				return domain.NewValidationError("invalid_udp_relay_mode", "TUIC udp_relay_mode must be native or quic")
			}
			updatedCreds.UDPRelayMode = urm
		}
		if cmd.Patch.Method != nil {
			m := strings.TrimSpace(*cmd.Patch.Method)
			if node.Protocol == domain.ProtocolSS && m == "" {
				return domain.NewValidationError("invalid_method", "Shadowsocks cipher method cannot be empty")
			}
			updatedCreds.Method = m
		}

		if pkIn := strings.TrimSpace(cmd.Patch.PrivateKeyInput); pkIn != "" {
			if pkIn == "***" {
				return domain.NewValidationError("invalid_private_key", "private_key_input must not be a masked placeholder")
			}
			updatedCreds.PrivateKey = pkIn
			rotatedCount++
		}
		if pskIn := strings.TrimSpace(cmd.Patch.PreSharedKeyInput); pskIn != "" {
			if pskIn == "***" {
				return domain.NewValidationError("invalid_pre_shared_key", "pre_shared_key_input must not be a masked placeholder")
			}
			updatedCreds.PreSharedKey = pskIn
			updatedCreds.PresharedKey = pskIn
			rotatedCount++
		}
		if pwIn := strings.TrimSpace(cmd.Patch.PasswordInput); pwIn != "" {
			if pwIn == "***" {
				return domain.NewValidationError("invalid_password", "password_input must not be a masked placeholder")
			}
			updatedCreds.Password = pwIn
			rotatedCount++
		}

		newVersion = node.CredentialVersion + 1
		newIdentity := *currentPayload.Identity
		newIdentity.Version = newVersion
		newPayload := &domain.NodeCredentialPayload{
			LogicalID:   node.LogicalID,
			Protocol:    node.Protocol,
			Server:      currentPayload.Server,
			Port:        currentPayload.Port,
			Version:     newVersion,
			Digest:      currentPayload.Digest,
			Identity:    &newIdentity,
			Credentials: updatedCreds,
		}
		newRec, encErr := s.vault.Encrypt(newPayload)
		if encErr != nil {
			return encErr
		}

		nowStr := domain.NowUTC().Format(time.RFC3339)
		res, err := tx.ExecContext(ctx, `
			UPDATE nodes
			SET display_name = ?, credential_version = ?, updated_at = ?
			WHERE logical_id = ? AND credential_version = ?;`,
			updatedDisplayName, newVersion, nowStr, node.LogicalID, node.CredentialVersion,
		)
		if err != nil {
			return fmt.Errorf("failed to update node %s: %w", node.LogicalID, err)
		}
		if rows, _ := res.RowsAffected(); rows != 1 {
			return domain.NewConflictError("credential_version_conflict", "concurrent credential version update detected")
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO node_credentials (logical_id, version, key_id, nonce, ciphertext, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?);`,
			newRec.LogicalID, newRec.Version, newRec.KeyID, newRec.Nonce, newRec.Ciphertext, nowStr, nowStr,
		); err != nil {
			return fmt.Errorf("failed to insert rotated credential for node %s: %w", node.LogicalID, err)
		}
		return nil
	})
	s.mu.Unlock()

	if txErr != nil {
		s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "node.connection.update", domain.AuditResultFailure,
			fmt.Sprintf("failed to update node %s connection: %s", logicalID, domain.RedactSensitiveInfo(txErr.Error())))
		return nil, txErr
	}

	s.recordAudit(ctx, cmd.ActorKind, cmd.RequestID, "node.connection.update", domain.AuditResultSuccess,
		fmt.Sprintf("updated node %s connection from v%d to v%d (rotated_secrets=%d)", logicalID, oldVersion, newVersion, rotatedCount))

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
