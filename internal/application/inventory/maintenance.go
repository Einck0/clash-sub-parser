package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clash-sub-parser/internal/application/probe"
	"clash-sub-parser/internal/application/publication"
	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/fetch"
)

// MaintenanceOperation defines the submode for inventory maintenance.
type MaintenanceOperation string

const (
	OperationRefresh            MaintenanceOperation = "refresh"
	OperationProbe              MaintenanceOperation = "probe"
	OperationPreview            MaintenanceOperation = "preview"
	OperationPublish            MaintenanceOperation = "publish"
	OperationValidate           MaintenanceOperation = "validate"
	OperationDeleteRule         MaintenanceOperation = "delete-rule"
	OperationRestoreGroupFilters MaintenanceOperation = "restore-group-filters"
)

// IsValid checks if the operation is supported.
func (op MaintenanceOperation) IsValid() bool {
	switch op {
	case OperationRefresh, OperationProbe, OperationPreview, OperationPublish, OperationValidate, OperationDeleteRule, OperationRestoreGroupFilters:
		return true
	default:
		return false
	}
}

// ParseProbeKinds parses a comma-separated list of probe kinds with aliases.
func ParseProbeKinds(s string) ([]domain.ProbeKind, error) {
	if strings.TrimSpace(s) == "" {
		return []domain.ProbeKind{
			domain.ProbeKindBaseline,
			domain.ProbeKindStreaming,
			domain.ProbeKindAI,
			domain.ProbeKindIPRisk,
			domain.ProbeKindGeo,
			domain.ProbeKindSpeed,
		}, nil
	}

	parts := strings.Split(s, ",")
	seen := make(map[domain.ProbeKind]bool)
	var result []domain.ProbeKind

	for _, p := range parts {
		token := strings.ToLower(strings.TrimSpace(p))
		if token == "" {
			continue
		}
		var kind domain.ProbeKind
		switch token {
		case "alive", "baseline":
			kind = domain.ProbeKindBaseline
		case "media", "streaming":
			kind = domain.ProbeKindStreaming
		case "ai":
			kind = domain.ProbeKindAI
		case "ip_risk", "iprisk":
			kind = domain.ProbeKindIPRisk
		case "geo":
			kind = domain.ProbeKindGeo
		case "speed":
			kind = domain.ProbeKindSpeed
		default:
			return nil, fmt.Errorf("unsupported probe kind: %q (supported: alive, media, ai, ip_risk, geo, speed)", token)
		}

		if !seen[kind] {
			seen[kind] = true
			result = append(result, kind)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid probe kinds specified")
	}
	return result, nil
}

// MaintenanceConfig specifies parameters for the maintenance job.
type MaintenanceConfig struct {
	TargetDBPath                  string
	Operation                     MaintenanceOperation
	DryRun                        bool
	ReportDir                     string
	Kinds                         []domain.ProbeKind
	SnapshotPublish               bool
	OmitUnavailableOptionalGroups bool
	FetchProxy                    string
	AliveConcurrency       int
	AliveTimeout           time.Duration
	MediaConcurrency       int
	SpeedConcurrency       int
	SpeedTimeout           time.Duration
	SpeedMaxBytesPerNode   int64
	SpeedTotalBudget       int64
	GlobalTimeout          time.Duration
	Dialer                 probe.NodeDialer
	FetchClient            fetch.Client
	AllowConcurrentService bool
}

// MaintenanceReport provides the structured top-level report for the maintenance run.
type MaintenanceReport struct {
	RunID          string            `json:"run_id"`
	Operation      string            `json:"operation"`
	Status         string            `json:"status"` // PASS, PARTIAL, BLOCKED, FAILED
	Summary        string            `json:"summary"`
	DryRun         bool              `json:"dry_run"`
	TargetDB       string            `json:"target_db"`
	ReportFilePath string            `json:"report_file_path,omitempty"`
	StartedAt      time.Time         `json:"started_at"`
	FinishedAt     time.Time         `json:"finished_at"`
	Elapsed        string            `json:"elapsed"`
	Refresh        *RefreshSummary   `json:"refresh,omitempty"`
	Probe          *ProbeSummary     `json:"probe,omitempty"`
	Preview        *PreviewSummary   `json:"preview,omitempty"`
	Publish        *PublishSummary   `json:"publish,omitempty"`
}

// RefreshSummary summarizes subscription refreshes.
type RefreshSummary struct {
	TotalSources      int                   `json:"total_sources"`
	EnabledSources    int                   `json:"enabled_sources"`
	SuccessfulSources int                   `json:"successful_sources"`
	FailedSources     int                   `json:"failed_sources"`
	TotalNodesParsed  int                   `json:"total_nodes_parsed"`
	TotalNodesValid   int                   `json:"total_nodes_valid"`
	TotalNotices      int                   `json:"total_notices"`
	Sources           []SourceRefreshResult `json:"sources"`
	Summary           string                `json:"summary,omitempty"`
}

// SourceRefreshResult records reconciliation outcome for one subscription.
type SourceRefreshResult struct {
	SubscriptionID string `json:"subscription_id"`
	Name           string `json:"name"`
	Status         string `json:"status"` // success, partial, failed, skipped
	ContentDigest  string `json:"content_digest,omitempty"`
	NodesParsed    int    `json:"nodes_parsed"`
	NodesValid     int    `json:"nodes_valid"`
	NoticesCount   int    `json:"notices_count"`
	Elapsed        string `json:"elapsed"`
	SafeError      string `json:"safe_error,omitempty"`
}

// ProbeSummary summarizes node probing tasks.
type ProbeSummary struct {
	RunID                 string           `json:"run_id"`
	TotalNodesTargeted    int              `json:"total_nodes_targeted"`
	TotalTasksScheduled   int              `json:"total_tasks_scheduled"`
	CompletedTasks        int              `json:"completed_tasks"`
	FailedTasks           int              `json:"failed_tasks"`
	SkippedTasks          int              `json:"skipped_tasks"`
	AvailableNodes        int              `json:"available_nodes"`
	TotalBytesDownloaded  int64            `json:"total_bytes_downloaded"`
	KindsRequested        []string         `json:"kinds_requested"`
	Elapsed               string           `json:"elapsed"`
	Summary               string           `json:"summary,omitempty"`
	Rows                  []ProbeReportRow `json:"rows,omitempty"`
}

// ProbeReportRow records the specific status of a node under a capability kind.
type ProbeReportRow struct {
	NodeID             string             `json:"node_id"`
	Kind               string             `json:"kind"`
	Status             string             `json:"status"` // available, unavailable, skipped, cancelled, untested
	SkipReason         string             `json:"skip_reason,omitempty"`
	ErrorCode          string             `json:"error_code,omitempty"`
	SafeDetail         *domain.SafeDetail `json:"safe_detail,omitempty"`
	LatencyMS          int64              `json:"latency_ms,omitempty"`
	ConnectionRevision *int64             `json:"connection_revision,omitempty"`
	Engine             string             `json:"engine"`
	ObservedAt         string             `json:"observed_at,omitempty"`
}

// PreviewSummary summarizes compilation preview and policy preflight.
type PreviewSummary struct {
	SnapshotID       string   `json:"snapshot_id,omitempty"`
	Target           string   `json:"target"`
	Allowed          bool     `json:"allowed"`
	NodeCount        int      `json:"node_count"`
	DiagnosticsCount int      `json:"diagnostics_count"`
	Diagnostics      []string `json:"diagnostics,omitempty"`
	ContentDigest    string   `json:"content_digest,omitempty"`
	BytesSize        int      `json:"bytes_size"`
	Status           string   `json:"status"` // success, blocked, failed
	Summary          string   `json:"summary,omitempty"`
}

// PublishSummary summarizes snapshot activation.
type PublishSummary struct {
	PublicationID string `json:"publication_id,omitempty"`
	State         string `json:"state,omitempty"`
	Target        string `json:"target,omitempty"`
	ContentDigest string `json:"content_digest,omitempty"`
	Size          int    `json:"size,omitempty"`
	ExportURL     string `json:"export_url,omitempty"`
	Summary       string `json:"summary,omitempty"`
}

// MaintenanceOrchestrator coordinates offline inventory validation and maintenance.
type MaintenanceOrchestrator struct {
	db           *sql.DB
	subRepo      domain.SubscriptionRepository
	nodeRepo     domain.NodeRepository
	obsRepo      domain.ProbeObservationRepository
	runRepo      domain.ProbeRunRepository
	invService   *Service
	probeRunner  probe.Runner
	probeService *probe.Service
	pubService   *publication.Service
	auditRepo    domain.AuditRepository
}

// NewMaintenanceOrchestrator constructs a new orchestrator.
func NewMaintenanceOrchestrator(
	db *sql.DB,
	subRepo domain.SubscriptionRepository,
	nodeRepo domain.NodeRepository,
	obsRepo domain.ProbeObservationRepository,
	runRepo domain.ProbeRunRepository,
	invService *Service,
	probeRunner probe.Runner,
	probeService *probe.Service,
	pubService *publication.Service,
	auditRepo domain.AuditRepository,
) *MaintenanceOrchestrator {
	return &MaintenanceOrchestrator{
		db:           db,
		subRepo:      subRepo,
		nodeRepo:     nodeRepo,
		obsRepo:      obsRepo,
		runRepo:      runRepo,
		invService:   invService,
		probeRunner:  probeRunner,
		probeService: probeService,
		pubService:   pubService,
		auditRepo:    auditRepo,
	}
}

// Run executes the maintenance workflow according to the provided configuration.
func (o *MaintenanceOrchestrator) Run(ctx context.Context, cfg MaintenanceConfig) (*MaintenanceReport, error) {
	if cfg.Operation == "" {
		cfg.Operation = OperationValidate
	}
	if !cfg.Operation.IsValid() {
		return nil, fmt.Errorf("invalid operation: %q", cfg.Operation)
	}

	if len(cfg.Kinds) == 0 {
		var err error
		cfg.Kinds, err = ParseProbeKinds("")
		if err != nil {
			return nil, err
		}
	}

	globalTimeout := cfg.GlobalTimeout
	if globalTimeout <= 0 {
		globalTimeout = 20 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, globalTimeout)
	defer cancel()

	startedAt := domain.NowUTC()
	report := &MaintenanceReport{
		RunID:     domain.MustNewUUIDv7(),
		Operation: string(cfg.Operation),
		DryRun:    cfg.DryRun,
		TargetDB:  cfg.TargetDBPath,
		StartedAt: startedAt,
		Status:    "PASS",
	}

	var opErr error
	switch cfg.Operation {
	case OperationRefresh:
		opErr = o.executeRefresh(runCtx, cfg, report)
	case OperationProbe:
		opErr = o.executeProbe(runCtx, cfg, report)
	case OperationPreview:
		opErr = o.executePreview(runCtx, cfg, report)
	case OperationPublish:
		opErr = o.executePublish(runCtx, cfg, report)
	case OperationValidate:
		_ = o.executeRefresh(runCtx, cfg, report)
		_ = o.executeProbe(runCtx, cfg, report)
		_ = o.executePreview(runCtx, cfg, report)
		if cfg.SnapshotPublish {
			_ = o.executePublish(runCtx, cfg, report)
		}
	}

	report.FinishedAt = domain.NowUTC()
	report.Elapsed = report.FinishedAt.Sub(startedAt).String()

	// Determine aggregate status
	o.evaluateOverallStatus(report)

	if err := o.saveReportFile(report, cfg.ReportDir); err != nil {
		return report, fmt.Errorf("failed to save report file: %w", err)
	}

	return report, opErr
}

func (o *MaintenanceOrchestrator) evaluateOverallStatus(report *MaintenanceReport) {
	hasFailure := false
	hasBlocked := false

	if report.Refresh != nil && report.Refresh.FailedSources > 0 {
		hasFailure = true
	}

	if report.Probe != nil {
		if report.Probe.TotalNodesTargeted > 0 && report.Probe.AvailableNodes == 0 {
			hasFailure = true
		}
	}

	if report.Preview != nil {
		if !report.Preview.Allowed || report.Preview.Status == "blocked" {
			hasBlocked = true
		} else if report.Preview.Status == "failed" {
			hasFailure = true
		}
	}

	if report.Publish != nil && report.Publish.State == "failed" {
		hasFailure = true
	}

	if hasBlocked {
		report.Status = "BLOCKED"
		report.Summary = "Maintenance workflow encountered blocked publication policies"
	} else if hasFailure {
		report.Status = "PARTIAL"
		report.Summary = "Maintenance workflow completed with partial failures recorded"
	} else {
		report.Status = "PASS"
		report.Summary = "Maintenance workflow completed successfully"
	}
}

func (o *MaintenanceOrchestrator) executeRefresh(ctx context.Context, cfg MaintenanceConfig, report *MaintenanceReport) error {
	subs, _, err := o.subRepo.List(ctx, domain.SubscriptionFilter{
		Pagination: domain.Pagination{Page: 1, PageSize: 1000},
	})
	if err != nil {
		return fmt.Errorf("failed to list subscriptions: %w", err)
	}

	summary := &RefreshSummary{
		TotalSources: len(subs),
	}

	var enabledSubs []domain.Subscription
	for _, s := range subs {
		if s.Enabled {
			enabledSubs = append(enabledSubs, s)
		}
	}
	summary.EnabledSources = len(enabledSubs)

	if cfg.DryRun {
		for _, s := range enabledSubs {
			summary.Sources = append(summary.Sources, SourceRefreshResult{
				SubscriptionID: s.ID,
				Name:           s.Name,
				Status:         "dry_run_inspected",
				Elapsed:        "0s",
			})
		}
		summary.Summary = fmt.Sprintf("dry run: %d enabled subscriptions inspected (no fetches performed)", len(enabledSubs))
		report.Refresh = summary
		return nil
	}

	if len(enabledSubs) == 0 {
		summary.Summary = "no enabled subscriptions found in database"
		report.Refresh = summary
		return nil
	}

	for _, s := range enabledSubs {
		start := time.Now()
		res, recErr := o.invService.ReconcileSubscription(ctx, s.ID)
		elapsed := time.Since(start).String()

		srcRes := SourceRefreshResult{
			SubscriptionID: s.ID,
			Name:           s.Name,
			Elapsed:        elapsed,
		}

		if recErr != nil {
			srcRes.Status = "failed"
			srcRes.SafeError = domain.RedactSensitiveInfo(recErr.Error())
			if res != nil {
				srcRes.NodesParsed = res.NodesParsed
				srcRes.NodesValid = res.NodesValid
				srcRes.ContentDigest = res.ContentDigest
			}
			summary.FailedSources++
		} else if res != nil {
			srcRes.ContentDigest = res.ContentDigest
			srcRes.NodesParsed = res.NodesParsed
			srcRes.NodesValid = res.NodesValid
			srcRes.NoticesCount = res.NodesParsed - res.NodesValid
			summary.TotalNodesParsed += res.NodesParsed
			summary.TotalNodesValid += res.NodesValid
			summary.TotalNotices += srcRes.NoticesCount

			if res.NodesValid == 0 {
				srcRes.Status = "failed"
				srcRes.SafeError = "extracted 0 valid nodes from source"
				summary.FailedSources++
			} else if res.Outcome == domain.FetchOutcomePartial {
				srcRes.Status = "partial"
				summary.SuccessfulSources++
			} else {
				srcRes.Status = "success"
				summary.SuccessfulSources++
			}
		}

		summary.Sources = append(summary.Sources, srcRes)
	}

	summary.Summary = fmt.Sprintf("refreshed %d enabled sources (%d successful, %d failed, %d valid nodes)",
		summary.EnabledSources, summary.SuccessfulSources, summary.FailedSources, summary.TotalNodesValid)
	report.Refresh = summary
	return nil
}

func (o *MaintenanceOrchestrator) executeProbe(ctx context.Context, cfg MaintenanceConfig, report *MaintenanceReport) error {
	nodes, total, err := o.nodeRepo.List(ctx, domain.NodeFilter{
		Scope:          domain.NodeScopeEnabledSubscriptions,
		ActiveOnly:     true,
		ExcludeNotices: true,
		Pagination:     domain.Pagination{Page: 1, PageSize: 10000},
	})
	if err != nil {
		return fmt.Errorf("failed to list nodes for probe: %w", err)
	}

	kindsStr := make([]string, len(cfg.Kinds))
	for i, k := range cfg.Kinds {
		kindsStr[i] = string(k)
	}

	summary := &ProbeSummary{
		TotalNodesTargeted:  total,
		TotalTasksScheduled: total * len(cfg.Kinds),
		KindsRequested:      kindsStr,
	}

	if cfg.DryRun {
		summary.RunID = "dry-run"
		summary.Summary = fmt.Sprintf("dry run: planned %d probe tasks across %d nodes", summary.TotalTasksScheduled, total)
		report.Probe = summary
		return nil
	}

	if total == 0 || len(nodes) == 0 {
		summary.RunID = "none"
		summary.Summary = "no active nodes in inventory to probe"
		report.Probe = summary
		return nil
	}

	nodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.LogicalID
	}

	probeStart := time.Now()
	runCmd := probe.CreateRunCommand{
		ActorScope:     "maintenance",
		IdempotencyKey: "maintain-probe-" + report.RunID,
		ConfigRevision: "rev-maintenance",
		NodeLogicalIDs: nodeIDs,
		Kinds:          cfg.Kinds,
	}

	run, err := o.probeService.Create(ctx, runCmd)
	if err != nil {
		return fmt.Errorf("failed to create probe run: %w", err)
	}
	summary.RunID = run.ID

	runErr := o.probeRunner.Run(ctx, run, nodeIDs, cfg.Kinds)
	summary.Elapsed = time.Since(probeStart).String()
	if runErr != nil {
		summary.Summary = fmt.Sprintf("probe execution returned: %v", runErr)
	}

	// Query recorded observations
	obsList, err := o.obsRepo.ListByRun(ctx, run.ID)
	if err != nil {
		obsList = nil
	}

	obsMap := make(map[string]map[domain.ProbeKind]domain.ProbeObservation)
	var totalSpeedBytes int64
	for _, obs := range obsList {
		if _, ok := obsMap[obs.NodeLogicalID]; !ok {
			obsMap[obs.NodeLogicalID] = make(map[domain.ProbeKind]domain.ProbeObservation)
		}
		obsMap[obs.NodeLogicalID][obs.Kind] = obs
		if obs.Kind == domain.ProbeKindSpeed && obs.Throughput != nil {
			var b int64
			if idx := strings.Index(obs.RedactedSummary, "bytes_read="); idx != -1 {
				_, _ = fmt.Sscanf(obs.RedactedSummary[idx:], "bytes_read=%d", &b)
			}
			totalSpeedBytes += b
		}
	}
	summary.TotalBytesDownloaded = totalSpeedBytes

	var rows []ProbeReportRow
	var availableNodesCount int
	var completedTasks int
	var failedTasks int
	var skippedTasks int

	for _, n := range nodes {
		nodeObs := obsMap[n.LogicalID]
		aliveObs, hasAlive := nodeObs[domain.ProbeKindBaseline]
		passedAlive := hasAlive && aliveObs.Verdict == domain.VerdictAvailable
		if passedAlive {
			availableNodesCount++
		}

		for _, kind := range cfg.Kinds {
			if obs, exists := nodeObs[kind]; exists {
				status := "unavailable"
				if obs.Verdict == domain.VerdictAvailable {
					status = "available"
					completedTasks++
				} else {
					failedTasks++
				}
				errCode := ""
				if obs.SafeDetail != nil && obs.SafeDetail.Code != "" {
					errCode = obs.SafeDetail.Code
				} else if status == "unavailable" {
					errCode = "probe_failed"
				}

				rows = append(rows, ProbeReportRow{
					NodeID:             n.LogicalID,
					Kind:               string(kind),
					Status:             status,
					ErrorCode:          errCode,
					SafeDetail:         obs.SafeDetail,
					LatencyMS:          obs.LatencyMS,
					ConnectionRevision: obs.ConnectionRevision,
					Engine:             "mihomo",
					ObservedAt:         obs.ObservedAt.Format(time.RFC3339),
				})
			} else {
				// No observation recorded
				if kind != domain.ProbeKindBaseline && !passedAlive {
					skippedTasks++
					rows = append(rows, ProbeReportRow{
						NodeID:     n.LogicalID,
						Kind:       string(kind),
						Status:     "skipped",
						SkipReason: "alive_gating_dependency_failed",
						ErrorCode:  "dependency_skipped",
						Engine:     "mihomo",
					})
				} else {
					failedTasks++
					rows = append(rows, ProbeReportRow{
						NodeID:     n.LogicalID,
						Kind:       string(kind),
						Status:     "cancelled",
						SkipReason: "task_not_completed",
						ErrorCode:  "incomplete",
						Engine:     "mihomo",
					})
				}
			}
		}
	}

	summary.CompletedTasks = completedTasks
	summary.FailedTasks = failedTasks
	summary.SkippedTasks = skippedTasks
	summary.AvailableNodes = availableNodesCount
	summary.Rows = rows
	summary.Summary = fmt.Sprintf("probed %d nodes: %d available, %d completed tasks, %d skipped, %d failed",
		total, availableNodesCount, completedTasks, skippedTasks, failedTasks)

	report.Probe = summary
	return nil
}

func (o *MaintenanceOrchestrator) executePreview(ctx context.Context, cfg MaintenanceConfig, report *MaintenanceReport) error {
	summary := &PreviewSummary{
		Target: string(domain.TargetMihomo),
	}

	if cfg.DryRun {
		summary.Status = "dry_run_inspected"
		summary.Summary = "dry run: preview preflight inspected (no publication created)"
		report.Preview = summary
		return nil
	}

	if o.pubService == nil {
		summary.Status = "skipped"
		summary.Summary = "publication service not configured"
		report.Preview = summary
		return nil
	}

	preflight, _ := o.pubService.Preflight(ctx, publication.PreflightCommand{
		Target:                         domain.TargetMihomo,
		PruneUnavailableOptionalGroups: cfg.OmitUnavailableOptionalGroups,
		OmitUnavailableOptionalGroups:  cfg.OmitUnavailableOptionalGroups,
	})

	prev, err := o.pubService.Preview(ctx, publication.PreviewQuery{
		Target:                         domain.TargetMihomo,
		CompatMode:                     "strict",
		PruneUnavailableOptionalGroups: cfg.OmitUnavailableOptionalGroups,
		OmitUnavailableOptionalGroups:  cfg.OmitUnavailableOptionalGroups,
	})
	if err != nil {
		summary.Allowed = false
		summary.Status = "failed"
		summary.Diagnostics = []string{domain.RedactSensitiveInfo(err.Error())}
		summary.Summary = "preview generation failed"
		report.Preview = summary
		return nil
	}

	if prev != nil {
		allowed := (preflight != nil && preflight.Allowed) && len(prev.Content) > 0
		nodeCount := prev.Manifest.NodeCount

		summary.SnapshotID = prev.SnapshotID
		summary.Target = string(prev.Target)
		summary.Allowed = allowed
		summary.NodeCount = nodeCount
		summary.DiagnosticsCount = len(prev.Diagnostics)
		summary.ContentDigest = prev.ContentDigest
		summary.BytesSize = len(prev.Content)

		diagStrs := make([]string, 0, len(prev.Diagnostics))
		for _, d := range prev.Diagnostics {
			diagStrs = append(diagStrs, fmt.Sprintf("[%s] %s: %s", d.Severity, d.Code, domain.RedactSensitiveInfo(d.Message)))
		}
		summary.Diagnostics = diagStrs

		if allowed {
			summary.Status = "success"
			summary.Summary = fmt.Sprintf("generated valid preview snapshot %s (%d nodes, %d bytes)",
				prev.SnapshotID, nodeCount, len(prev.Content))
		} else {
			summary.Status = "blocked"
			summary.Summary = "preview generated but policy preflight blocked publication"
		}
	}

	report.Preview = summary
	return nil
}

func (o *MaintenanceOrchestrator) executePublish(ctx context.Context, cfg MaintenanceConfig, report *MaintenanceReport) error {
	summary := &PublishSummary{
		Target: string(domain.TargetMihomo),
	}

	if !cfg.SnapshotPublish {
		summary.State = "skipped"
		summary.Summary = "publication activation not requested (run with --snapshot-publish to activate)"
		report.Publish = summary
		return nil
	}

	if report.Preview == nil || report.Preview.SnapshotID == "" {
		summary.State = "failed"
		summary.Summary = "no valid preview snapshot available to publish"
		report.Publish = summary
		return nil
	}

	if !report.Preview.Allowed {
		summary.State = "blocked"
		summary.Summary = "preview preflight was not allowed; publication blocked"
		report.Publish = summary
		return nil
	}

	pubRes, err := o.pubService.Publish(ctx, publication.PublishCommand{
		Target:                         domain.TargetMihomo,
		SnapshotID:                     report.Preview.SnapshotID,
		ActorKind:                      domain.ActorKindAdmin,
		RequestID:                      report.RunID,
		PruneUnavailableOptionalGroups: cfg.OmitUnavailableOptionalGroups,
		OmitUnavailableOptionalGroups:  cfg.OmitUnavailableOptionalGroups,
	})
	if err != nil {
		summary.State = "failed"
		summary.Summary = fmt.Sprintf("publication failed: %s", domain.RedactSensitiveInfo(err.Error()))
		report.Publish = summary
		return nil
	}

	exportURL := pubRes.ExportURL
	if idx := strings.Index(exportURL, "?token="); idx != -1 {
		exportURL = exportURL[:idx] + "?token=***"
	}

	summary.PublicationID = pubRes.Publication.ID
	summary.State = string(pubRes.Publication.State)
	summary.ContentDigest = pubRes.ContentDigest
	summary.Size = pubRes.Size
	summary.ExportURL = exportURL
	summary.Summary = fmt.Sprintf("activated immutable publication %s", pubRes.Publication.ID)

	report.Publish = summary
	return nil
}

func (o *MaintenanceOrchestrator) saveReportFile(report *MaintenanceReport, reportDir string) error {
	if reportDir == "" {
		reportDir = "./reports"
	}
	if err := os.MkdirAll(reportDir, 0700); err != nil {
		return fmt.Errorf("failed to create report directory %q: %w", reportDir, err)
	}
	_ = os.Chmod(reportDir, 0700)

	timestamp := report.StartedAt.Format("20060102T150405Z")
	filename := fmt.Sprintf("maintenance-report-%s.json", timestamp)
	filePath := filepath.Join(reportDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report json: %w", err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open report file %q: %w", filePath, err)
	}
	defer f.Close()
	_ = f.Chmod(0600)

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write report file %q: %w", filePath, err)
	}

	report.ReportFilePath = filePath
	return nil
}
