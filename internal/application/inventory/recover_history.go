package inventory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"clash-sub-parser/internal/domain"
	"clash-sub-parser/internal/parser"
	"clash-sub-parser/internal/repository/sqlite"
)

// RecoveryManifest models the cross-verified provenance manifest JSON.
type RecoveryManifest struct {
	SchemaVersion string           `json:"schema_version"`
	Metadata      ManifestMetadata `json:"metadata"`
	Summary       ManifestSummary  `json:"summary"`
	Nodes         []ManifestNode   `json:"nodes"`
}

type ManifestMetadata struct {
	RunID           string            `json:"run_id"`
	GeneratedAt     string            `json:"generated_at"`
	TargetDB        string            `json:"target_db"`
	ColdArchive     ManifestArchive   `json:"cold_archive"`
	V1BackupDigests map[string]string `json:"v1_backup_digests"`
}

type ManifestArchive struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ManifestSummary struct {
	TotalCoveredOrphans                      int            `json:"total_covered_orphans"`
	VerifiedNodes                            int            `json:"verified_nodes"`
	ConflictNodes                            int            `json:"conflict_nodes"`
	UnknownNodes                             int            `json:"unknown_nodes"`
	PrimaryVerifiedEdges                     int            `json:"primary_verified_edges"`
	SecondarySameCanonicalEdges              int            `json:"secondary_same_canonical_edges"`
	DiscardedConflictingCredentialCandidates int            `json:"discarded_conflicting_credential_candidates"`
	ByOrigin                                 map[string]int `json:"by_origin"`
	ByMatchType                              map[string]int `json:"by_match_type"`
	BySource                                 map[string]int `json:"by_source"`
}

type ManifestNode struct {
	NodeLogicalID                  string               `json:"node_logical_id"`
	Protocol                       string               `json:"protocol"`
	Server                         string               `json:"server"`
	Port                           int                  `json:"port"`
	DisplayName                    string               `json:"display_name"`
	ConnectionRevision             int64                `json:"connection_revision"`
	ProvenanceOrigin               string               `json:"provenance_origin"`
	NodeMatchType                  string               `json:"node_match_type"`
	SourceIdentityConfidence       string               `json:"source_identity_confidence"`
	Verdict                        string               `json:"verdict"`
	PrimaryHistoricalSource        *ManifestSourceEdge  `json:"primary_historical_source"`
	SecondarySameCanonicalSources []ManifestSourceEdge `json:"secondary_same_canonical_sources"`
	DiscardedCollisionNotes        []ManifestSourceEdge `json:"discarded_collision_notes"`
	Evidence                       []ManifestEvidence   `json:"evidence"`
}

type ManifestSourceEdge struct {
	RelationshipKind       string `json:"relationship_kind"`
	LegacyNodePK           int64  `json:"legacy_node_pk"`
	LegacyLinkPK           int64  `json:"legacy_link_pk"`
	LegacySourceID         int64  `json:"legacy_source_id"`
	HistoricalSourceName   string `json:"historical_source_name"`
	TargetSubscriptionID   string `json:"target_subscription_id"`
	TargetSubscriptionName string `json:"target_subscription_name"`
	ObservedAt             string `json:"observed_at"`
	RelationState          string `json:"relation_state"`
}

type ManifestEvidence struct {
	ArchivePath string `json:"archive_path"`
	BackupFile  string `json:"backup_file"`
	SHA256      string `json:"sha256"`
	TableKeys   string `json:"table_keys"`
}

// InvariantsReport captures target database invariant baseline and post-run metrics.
type InvariantsReport struct {
	TotalNodes              int      `json:"total_nodes"`
	ActiveNodes             int      `json:"active_nodes"`
	NodeSources             int      `json:"node_sources"`
	ScopeENodes             []string `json:"scope_e_nodes,omitempty"`
	HeadRevisions           int      `json:"head_revisions"`
	NodeOverrides           int      `json:"node_overrides"`
	BusinessAssetsUntouched bool     `json:"business_assets_untouched"`
	AssetFingerprint        string   `json:"asset_fingerprint"`
	TablesVerified          []string `json:"tables_verified,omitempty"`
}

// RecoveryConfig defines the options for the recover-source-history tool.
type RecoveryConfig struct {
	ManifestPath           string
	ExpectedManifestSHA256 string
	DryRun                 bool
	TargetDBPath           string
	ArchiveDir             string
}

// RecoveryResult encapsulates the outcome report of the recovery operation.
type RecoveryResult struct {
	DryRun                      bool             `json:"dry_run"`
	PlannedSchema               string           `json:"planned_schema,omitempty"`
	ExistingHistoryRecords      int              `json:"existing_history_records"`
	ManifestPath                string           `json:"manifest_path"`
	ManifestSHA256              string           `json:"manifest_sha256"`
	TotalCoveredOrphans         int              `json:"total_covered_orphans"`
	PrimaryVerifiedEdges        int              `json:"primary_verified_edges"`
	SecondarySameCanonicalEdges int              `json:"secondary_same_canonical_edges"`
	DiscardedCandidates         int              `json:"discarded_candidates"`
	TotalEdgesToInsert          int              `json:"total_edges_to_insert"`
	InsertedRecords             int              `json:"inserted_records"`
	NewlyInserted               int              `json:"newly_inserted"`
	ExistingRecords             int              `json:"existing_records"`
	TotalRecords                int              `json:"total_records"`
	PredictedNewRecords         int              `json:"predicted_new_records,omitempty"`
	BaselineInvariants          InvariantsReport `json:"baseline_invariants"`
	PostInvariants              InvariantsReport `json:"post_invariants"`
	InvariantsVerified          bool             `json:"invariants_verified"`
	Status                      string           `json:"status"`
}

type queryExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CanonicalSourceURL normalizes a subscription URL strictly and minimally:
// - scheme and host (hostname only) are lowercased;
// - default scheme ports (:80 for http, :443 for https) are normalized;
// - raw userinfo, path (including case and trailing slashes), and raw query (token/signature sensitive) are strictly preserved;
// - fragment is stripped as it does not participate in actual HTTP requests;
// - different tokens on the same host/path produce different identities.
func CanonicalSourceURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	clean := *u
	clean.Scheme = strings.ToLower(u.Scheme)

	// Normalize host: lowercase hostname, normalize default ports only
	if h, p, err := net.SplitHostPort(u.Host); err == nil {
		hLower := strings.ToLower(h)
		if (clean.Scheme == "http" && p == "80") || (clean.Scheme == "https" && p == "443") {
			clean.Host = hLower
		} else {
			clean.Host = hLower + ":" + p
		}
	} else {
		clean.Host = strings.ToLower(u.Host)
	}

	// Fragment does not participate in actual HTTP requests and can be stripped
	clean.Fragment = ""
	clean.RawFragment = ""

	return clean.String()
}

func computeAssetsFingerprint(ctx context.Context, q queryExecutor) (string, InvariantsReport, map[string]int64, error) {
	var rep InvariantsReport
	headsMap := make(map[string]int64)

	// 1. Query available tables excluding ledger and sqlite internals
	tRows, err := q.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'node_source_history' AND name != 'schema_migrations' ORDER BY name ASC;")
	if err != nil {
		return "", rep, nil, fmt.Errorf("failed to query table list: %w", err)
	}
	defer tRows.Close()

	var tables []string
	for tRows.Next() {
		var name string
		if err := tRows.Scan(&name); err == nil {
			tables = append(tables, name)
		}
	}
	tRows.Close()
	rep.TablesVerified = tables

	hasher := sha256.New()

	for _, tbl := range tables {
		var count int
		if err := q.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s;", tbl)).Scan(&count); err != nil {
			return "", rep, nil, fmt.Errorf("failed to count rows in table %s: %w", tbl, err)
		}
		fmt.Fprintf(hasher, "tbl:%s:cnt:%d;", tbl, count)

		rows, err := q.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s ORDER BY rowid ASC;", tbl))
		if err != nil {
			return "", rep, nil, fmt.Errorf("failed to scan rows in table %s: %w", tbl, err)
		}
		cols, _ := rows.Columns()
		colCount := len(cols)
		vals := make([]any, colCount)
		valPtrs := make([]any, colCount)
		for i := range vals {
			valPtrs[i] = &vals[i]
		}

		for rows.Next() {
			if err := rows.Scan(valPtrs...); err == nil {
				for _, v := range vals {
					fmt.Fprintf(hasher, "%v;", v)
				}
			}
		}
		rows.Close()

		switch tbl {
		case "nodes":
			rep.TotalNodes = count
			_ = q.QueryRowContext(ctx, "SELECT COUNT(*) FROM nodes WHERE active = 1;").Scan(&rep.ActiveNodes)
		case "node_sources":
			rep.NodeSources = count
			sRows, sErr := q.QueryContext(ctx, "SELECT DISTINCT node_logical_id FROM node_sources ORDER BY node_logical_id ASC;")
			if sErr == nil {
				for sRows.Next() {
					var lid string
					if sRows.Scan(&lid) == nil {
						rep.ScopeENodes = append(rep.ScopeENodes, lid)
					}
				}
				sRows.Close()
			}
		case "node_connection_heads":
			hRows, hErr := q.QueryContext(ctx, "SELECT logical_id, connection_revision FROM node_connection_heads ORDER BY logical_id ASC;")
			if hErr == nil {
				for hRows.Next() {
					var lid string
					var rev int64
					if hRows.Scan(&lid, &rev) == nil {
						headsMap[lid] = rev
					}
				}
				hRows.Close()
			}
			rep.HeadRevisions = len(headsMap)
		case "node_overrides":
			rep.NodeOverrides = count
		}
	}

	fingerprint := hex.EncodeToString(hasher.Sum(nil))
	rep.AssetFingerprint = fingerprint
	rep.BusinessAssetsUntouched = true

	return fingerprint, rep, headsMap, nil
}

func extractArchiveCredentials(protocolStr string, payloadJSON string) (domain.InboundProtocolCredential, error) {
	var proxyMap map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &proxyMap); err != nil {
		return domain.InboundProtocolCredential{}, fmt.Errorf("failed to unmarshal archive normalized payload: %w", err)
	}
	if _, ok := proxyMap["type"]; !ok && protocolStr != "" {
		proxyMap["type"] = protocolStr
	}
	_, creds, err := parser.ExtractProxyMap(proxyMap)
	if err != nil {
		return domain.InboundProtocolCredential{}, fmt.Errorf("failed to extract proxy from archive map: %w", err)
	}
	return creds, nil
}

func credentialsMatch(target domain.InboundProtocolCredential, archive domain.InboundProtocolCredential, protocol domain.Protocol) bool {
	switch protocol {
	case domain.ProtocolVLESS, domain.ProtocolVMess, domain.ProtocolTUIC:
		if !strings.EqualFold(strings.TrimSpace(target.UUID), strings.TrimSpace(archive.UUID)) {
			return false
		}
		if protocol == domain.ProtocolVMess && target.AlterID != archive.AlterID && (target.AlterID != 0 && archive.AlterID != 0) {
			return false
		}
	case domain.ProtocolHysteria2, domain.ProtocolTrojan:
		if strings.TrimSpace(target.Password) != strings.TrimSpace(archive.Password) {
			return false
		}
	case domain.ProtocolSS:
		if strings.TrimSpace(target.Password) != strings.TrimSpace(archive.Password) {
			return false
		}
		if target.Method != "" && archive.Method != "" && !strings.EqualFold(strings.TrimSpace(target.Method), strings.TrimSpace(archive.Method)) {
			return false
		}
	case domain.ProtocolWireGuard:
		if strings.TrimSpace(target.PublicKey) != strings.TrimSpace(archive.PublicKey) {
			return false
		}
		if strings.TrimSpace(target.EffectivePreSharedKey()) != strings.TrimSpace(archive.EffectivePreSharedKey()) {
			return false
		}
		if archive.PrivateKey != "" && target.PrivateKey != "" && strings.TrimSpace(target.PrivateKey) != strings.TrimSpace(archive.PrivateKey) {
			return false
		}
		if target.MTU != 0 && archive.MTU != 0 && target.MTU != archive.MTU {
			return false
		}
	}

	// Username check
	if target.Username != "" && archive.Username != "" && target.Username != archive.Username {
		return false
	}

	// SNI check
	if target.SNI != "" && archive.SNI != "" && !strings.EqualFold(strings.TrimSpace(target.SNI), strings.TrimSpace(archive.SNI)) {
		return false
	}
	if target.DisableSNI != archive.DisableSNI && (target.DisableSNI || archive.DisableSNI) {
		return false
	}

	// CongestionControl & UDPRelayMode check
	if target.CongestionControl != "" && archive.CongestionControl != "" && !strings.EqualFold(strings.TrimSpace(target.CongestionControl), strings.TrimSpace(archive.CongestionControl)) {
		return false
	}
	if target.UDPRelayMode != "" && archive.UDPRelayMode != "" && !strings.EqualFold(strings.TrimSpace(target.UDPRelayMode), strings.TrimSpace(archive.UDPRelayMode)) {
		return false
	}

	tt := target.Transport
	at := archive.Transport
	if tt == nil {
		tt = make(map[string]string)
	}
	if at == nil {
		at = make(map[string]string)
	}

	// Normalize transport maps to lowercase keys
	normTT := make(map[string]string, len(tt))
	for k, v := range tt {
		normTT[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	normAT := make(map[string]string, len(at))
	for k, v := range at {
		normAT[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}

	// Specific transport attributes check
	// 1. TLS and certificate verification
	if tt["tls"] != "" && at["tls"] != "" {
		if domain.IsTruthy(tt["tls"]) != domain.IsTruthy(at["tls"]) {
			return false
		}
	}
	if domain.HasInsecureTransport(tt) != domain.HasInsecureTransport(at) {
		if (tt["skip_cert_verify"] != "" || tt["insecure"] != "") && (at["skip_cert_verify"] != "" || at["insecure"] != "") {
			return false
		}
	}

	// 2. Transport network / type
	netT := normTT["network"]
	if netT == "" {
		netT = normTT["type"]
	}
	netA := normAT["network"]
	if netA == "" {
		netA = normAT["type"]
	}
	if netT != "" && netA != "" && !strings.EqualFold(netT, netA) {
		return false
	}

	// 3. Transport headers and path (Websocket, HTTP, xHTTP)
	if normTT["path"] != "" && normAT["path"] != "" && normTT["path"] != normAT["path"] {
		return false
	}
	if normTT["host"] != "" && normAT["host"] != "" && !strings.EqualFold(normTT["host"], normAT["host"]) {
		return false
	}
	if normTT["headers"] != "" && normAT["headers"] != "" && normTT["headers"] != normAT["headers"] {
		return false
	}
	if normTT["xhttp-headers"] != "" && normAT["xhttp-headers"] != "" && normTT["xhttp-headers"] != normAT["xhttp-headers"] {
		return false
	}
	if normTT["service_name"] != "" && normAT["service_name"] != "" && !strings.EqualFold(normTT["service_name"], normAT["service_name"]) {
		return false
	}
	if normTT["mode"] != "" && normAT["mode"] != "" && !strings.EqualFold(normTT["mode"], normAT["mode"]) {
		return false
	}

	// 4. Reality parameters: SNI, PBK, SID, Flow
	if normTT["sni"] != "" && normAT["sni"] != "" && !strings.EqualFold(normTT["sni"], normAT["sni"]) {
		return false
	}
	if normTT["pbk"] != "" && normAT["pbk"] != "" && normTT["pbk"] != normAT["pbk"] {
		return false
	}
	if normTT["sid"] != "" && normAT["sid"] != "" && !strings.EqualFold(normTT["sid"], normAT["sid"]) {
		return false
	}
	if normTT["flow"] != "" && normAT["flow"] != "" && !strings.EqualFold(normTT["flow"], normAT["flow"]) {
		return false
	}

	// 5. Unknown extensions: check any other custom extension keys present in both
	knownKeys := map[string]bool{
		"network": true, "type": true, "tls": true, "insecure": true, "skip_cert_verify": true, "skipcert": true,
		"allow_insecure": true, "path": true, "host": true, "headers": true, "xhttp-headers": true, "xhttp_headers": true,
		"service_name": true, "mode": true, "sni": true, "pbk": true, "sid": true, "flow": true,
		"ports": true, "hy2_ports": true, "server_ports": true,
		"fp": true, "client-fingerprint": true, "client_fingerprint": true, "fingerprint": true,
		"udp": true,
	}
	for k, tv := range normTT {
		if knownKeys[k] {
			continue
		}
		if av, ok := normAT[k]; ok {
			if tv != av {
				return false
			}
		}
	}

	// Hysteria2 port hop comparison
	hyPortsTarget := domain.ExtractHy2Ports(tt)
	hyPortsArchive := domain.ExtractHy2Ports(at)
	if hyPortsTarget != "" && hyPortsArchive != "" && hyPortsTarget != hyPortsArchive {
		return false
	}

	return true
}

func resolveArchivePath(baseDir, rawPath string) string {
	if rawPath == "" {
		return ""
	}
	if filepath.IsAbs(rawPath) {
		if _, err := os.Stat(rawPath); err == nil {
			return rawPath
		}
	}
	if baseDir != "" {
		candidate := filepath.Join(baseDir, filepath.Base(rawPath))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	defaultBackups := "/home/service/backups"
	candidate := filepath.Join(defaultBackups, filepath.Base(rawPath))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return rawPath
}

// RecoverSourceHistory executes the recovery procedure according to strict invariant and archive verification gates.
func RecoverSourceHistory(ctx context.Context, db *sql.DB, cfg RecoveryConfig) (*RecoveryResult, error) {
	manifestBytes, err := os.ReadFile(cfg.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	sum := sha256.Sum256(manifestBytes)
	actualSHA := hex.EncodeToString(sum[:])

	if strings.TrimSpace(cfg.ExpectedManifestSHA256) == "" {
		return nil, fmt.Errorf("expected manifest sha256 checksum is required (fail-closed)")
	}
	if !strings.EqualFold(actualSHA, cfg.ExpectedManifestSHA256) {
		return nil, fmt.Errorf("manifest sha256 mismatch: got %s, want %s (fail-closed)", actualSHA, cfg.ExpectedManifestSHA256)
	}

	var manifest RecoveryManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	// Check table existence of node_source_history
	var historyTableExists int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='node_source_history';").Scan(&historyTableExists); err != nil {
		return nil, fmt.Errorf("failed to inspect database schema: %w", err)
	}

	var existingHistoryRecords int
	if historyTableExists > 0 {
		_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM node_source_history;").Scan(&existingHistoryRecords)
	} else if !cfg.DryRun {
		return nil, fmt.Errorf("database schema is not at migration 16: table node_source_history does not exist; apply migrations first before running recovery (fail-closed)")
	}

	// Stat target DB file for dry-run verification
	var dbFileInitialSize int64
	var dbFileInitialModTime time.Time
	if cfg.DryRun && cfg.TargetDBPath != "" {
		if fi, fiErr := os.Stat(cfg.TargetDBPath); fiErr == nil {
			dbFileInitialSize = fi.Size()
			dbFileInitialModTime = fi.ModTime()
		}
	}

	// Compute baseline asset fingerprint
	baseFingerprint, baseline, baseHeads, err := computeAssetsFingerprint(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("pre-flight invariants check failed: %w", err)
	}

	// Map modern subscriptions by canonical URL lineage and by ID
	type subRecord struct {
		ID                  string
		Name                string
		CanonicalLineageURL string
	}
	subMapByID := make(map[string]subRecord)
	var subList []subRecord

	sRows, err := db.QueryContext(ctx, "SELECT id, name, source_url_secret_ref FROM subscriptions;")
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions in target database: %w", err)
	}
	defer sRows.Close()
	for sRows.Next() {
		var sid, sname, surl string
		if sRows.Scan(&sid, &sname, &surl) == nil {
			rec := subRecord{
				ID:                  sid,
				Name:                sname,
				CanonicalLineageURL: CanonicalSourceURL(surl),
			}
			subMapByID[sid] = rec
			subList = append(subList, rec)
		}
	}
	sRows.Close()

	findModernSubscription := func(archiveSourceURL, archiveSourceName string, originalCandidateID string) (matchedID string, matchedName string) {
		canonicalArc := CanonicalSourceURL(archiveSourceURL)
		if canonicalArc == "" {
			return "", ""
		}
		var matches []subRecord
		for _, s := range subList {
			if s.CanonicalLineageURL != "" && s.CanonicalLineageURL == canonicalArc {
				matches = append(matches, s)
			}
		}
		if len(matches) == 1 {
			return matches[0].ID, matches[0].Name
		}
		if len(matches) > 1 {
			// If identical URL exists across multiple current subscriptions, only link if
			// verified by trusted original imported target subscription ID; otherwise leave unmapped.
			if originalCandidateID != "" {
				for _, m := range matches {
					if m.ID == originalCandidateID {
						return m.ID, m.Name
					}
				}
			}
			// Never arbitrarily pick one
			return "", ""
		}
		// len(matches) == 0: Name is for display only, NO fallback to name!
		return "", ""
	}

	// Archive DB connection cache
	archiveDBCache := make(map[string]*sql.DB)
	defer func() {
		for _, adb := range archiveDBCache {
			_ = adb.Close()
		}
	}()

	openArchiveReadonly := func(rawPath string) (*sql.DB, error) {
		resolved := resolveArchivePath(cfg.ArchiveDir, rawPath)
		if adb, ok := archiveDBCache[resolved]; ok {
			return adb, nil
		}
		if _, err := os.Stat(resolved); err != nil {
			return nil, fmt.Errorf("archive file %s not found: %w", resolved, err)
		}
		adb, err := sqlite.OpenReadOnly(resolved)
		if err != nil {
			return nil, fmt.Errorf("failed to open archive %s in read-only mode: %w", resolved, err)
		}
		archiveDBCache[resolved] = adb
		return adb, nil
	}

	// Verify cold archive file integrity if specified
	if manifest.Metadata.ColdArchive.Path != "" {
		coldPath := resolveArchivePath(cfg.ArchiveDir, manifest.Metadata.ColdArchive.Path)
		if arcBytes, readErr := os.ReadFile(coldPath); readErr == nil {
			arcSum := sha256.Sum256(arcBytes)
			arcActualSHA := hex.EncodeToString(arcSum[:])
			if manifest.Metadata.ColdArchive.SHA256 != "" && !strings.EqualFold(arcActualSHA, manifest.Metadata.ColdArchive.SHA256) {
				return nil, fmt.Errorf("cold archive sha256 mismatch for %s: got %s, want %s (fail-closed)",
					coldPath, arcActualSHA, manifest.Metadata.ColdArchive.SHA256)
			}
		}
	}

	primaryEdges := 0
	secondaryEdges := 0
	discardedCandidates := 0

	type historyRowToInsert struct {
		nodeLogicalID      string
		subIDPtr           any
		sourceIdentity     string
		sourceLabel        string
		connRev            any
		relationState      string
		cause              string
		firstObs           any
		lastObs            any
		evidenceKind       string
		evidenceKey        string
		evidenceJSON       string
	}
	var rowsToInsert []historyRowToInsert

	for _, n := range manifest.Nodes {
		var (
			dbProto    string
			dbServer   string
			dbPort     int
			dbConfig   string
			dbActive   int
			dbConnRev  int64
		)
		err := db.QueryRowContext(ctx, `
			SELECT n.protocol, n.server, n.port, n.config_json, n.active,
			       COALESCE(h.connection_revision, n.connection_revision, 1)
			FROM nodes n
			LEFT JOIN node_connection_heads h ON n.logical_id = h.logical_id
			WHERE n.logical_id = ?;`, n.NodeLogicalID).Scan(&dbProto, &dbServer, &dbPort, &dbConfig, &dbActive, &dbConnRev)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("manifest node %s does not exist in target database (fail-closed)", n.NodeLogicalID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to query node %s in database: %w", n.NodeLogicalID, err)
		}

		if !strings.EqualFold(dbProto, n.Protocol) || !strings.EqualFold(dbServer, n.Server) || dbPort != n.Port {
			return nil, fmt.Errorf("node %s endpoint mismatch between manifest and database (fail-closed)", n.NodeLogicalID)
		}

		// Fetch historical versions from node_connection_versions if present
		histVersions := make(map[int64]domain.InboundProtocolCredential)
		vRows, vErr := db.QueryContext(ctx, "SELECT connection_revision, effective_config_json FROM node_connection_versions WHERE node_logical_id = ?;", n.NodeLogicalID)
		if vErr == nil {
			for vRows.Next() {
				var rev int64
				var effJSON string
				if vRows.Scan(&rev, &effJSON) == nil {
					var verObj struct {
						Credentials domain.InboundProtocolCredential `json:"credentials"`
					}
					if err := json.Unmarshal([]byte(effJSON), &verObj); err == nil {
						histVersions[rev] = verObj.Credentials
					}
				}
			}
			vRows.Close()
		}

		var currCreds domain.InboundProtocolCredential
		_ = json.Unmarshal([]byte(dbConfig), &currCreds)

		if n.ProvenanceOrigin == "legacy_cold_archive" {
			coldPath := resolveArchivePath(cfg.ArchiveDir, manifest.Metadata.ColdArchive.Path)
			arcDB, arcErr := openArchiveReadonly(coldPath)
			if arcErr != nil {
				return nil, fmt.Errorf("failed to open cold archive for node %s: %w", n.NodeLogicalID, arcErr)
			}

			// 1. Primary verified source
			ps := n.PrimaryHistoricalSource
			if ps == nil {
				return nil, fmt.Errorf("node %s has origin legacy_cold_archive but missing primary source (fail-closed)", n.NodeLogicalID)
			}
			if ps.RelationState != string(domain.RelationStateVerified) {
				return nil, fmt.Errorf("primary source for node %s is not verified (got %s)", n.NodeLogicalID, ps.RelationState)
			}

			var (
				arcNodeID      int64
				arcProto       string
				arcServer      string
				arcPort        int
				arcPayload     string
				arcLinkID      int64
				arcRevID       int64
				arcLinkCreated string
				arcSourceID    int64
				arcFetchedAt   sql.NullString
				arcSrcID       int64
				arcSrcName     string
				arcSrcURL      string
			)

			query := `
				SELECT n.id, n.protocol, n.server, n.port, n.normalized_payload,
				       l.id, l.source_revision_id, l.created_at,
				       r.source_id, r.fetched_at,
				       s.id, s.name, s.url
				FROM nodes n
				JOIN node_source_links l ON l.node_id = n.id AND l.id = ?
				JOIN source_revisions r ON r.id = l.source_revision_id
				JOIN sources s ON s.id = r.source_id
				WHERE n.id = ?;`

			rowErr := arcDB.QueryRowContext(ctx, query, ps.LegacyLinkPK, ps.LegacyNodePK).Scan(
				&arcNodeID, &arcProto, &arcServer, &arcPort, &arcPayload,
				&arcLinkID, &arcRevID, &arcLinkCreated,
				&arcSourceID, &arcFetchedAt,
				&arcSrcID, &arcSrcName, &arcSrcURL,
			)
			if rowErr != nil {
				return nil, fmt.Errorf("node %s: archive source link chain not found for pk=%d link_pk=%d (fail-closed)", n.NodeLogicalID, ps.LegacyNodePK, ps.LegacyLinkPK)
			}

			if arcSrcID != ps.LegacySourceID {
				return nil, fmt.Errorf("node %s: archive source ID mismatch: got %d, want %d (fail-closed)", n.NodeLogicalID, arcSrcID, ps.LegacySourceID)
			}

			// Canonical credentials equality check
			arcCreds, pErr := extractArchiveCredentials(arcProto, arcPayload)
			if pErr != nil {
				return nil, fmt.Errorf("node %s: failed to extract archive credentials: %w", n.NodeLogicalID, pErr)
			}

			matched := credentialsMatch(currCreds, arcCreds, domain.Protocol(n.Protocol))
			if !matched {
				// Check whether any historical version matches
				for _, hCreds := range histVersions {
					if credentialsMatch(hCreds, arcCreds, domain.Protocol(n.Protocol)) {
						matched = true
						break
					}
				}
			}
			if !matched {
				return nil, fmt.Errorf("node %s: configuration mismatch with historical archive evidence (fail-closed)", n.NodeLogicalID)
			}

			// Modern subscription URL lineage mapping
			modernSubID, modernSubName := findModernSubscription(arcSrcURL, arcSrcName, ps.TargetSubscriptionID)
			var subIDPtr any = nil
			sourceLabel := arcSrcName
			if modernSubID != "" {
				subIDPtr = modernSubID
				sourceLabel = modernSubName
			}

			var firstObs any = nil
			if trimmed := strings.TrimSpace(arcLinkCreated); trimmed != "" {
				firstObs = trimmed
			}

			evidenceKey := fmt.Sprintf("%s:pk:%d:link:%d", n.ProvenanceOrigin, ps.LegacyNodePK, ps.LegacyLinkPK)
			evidenceData := map[string]any{
				"provenance_origin": n.ProvenanceOrigin,
				"legacy_node_pk":    ps.LegacyNodePK,
				"legacy_link_pk":    ps.LegacyLinkPK,
				"legacy_source_id":  ps.LegacySourceID,
				"historical_source": arcSrcName,
				"match_type":        n.NodeMatchType,
			}
			if len(n.Evidence) > 0 {
				evidenceData["archive_sha256"] = n.Evidence[0].SHA256
				evidenceData["table_keys"] = n.Evidence[0].TableKeys
			}
			evJSONBytes, _ := json.Marshal(evidenceData)

			rowsToInsert = append(rowsToInsert, historyRowToInsert{
				nodeLogicalID:  n.NodeLogicalID,
				subIDPtr:       subIDPtr,
				sourceIdentity: fmt.Sprintf("legacy:src:%d", ps.LegacySourceID),
				sourceLabel:    sourceLabel,
				connRev:        nil, // 975 legacy records have unverified NULL connection revision
				relationState:  string(domain.RelationStateVerified),
				cause:          string(domain.CauseLegacyImport),
				firstObs:       firstObs,
				lastObs:        firstObs,
				evidenceKind:   n.ProvenanceOrigin,
				evidenceKey:    evidenceKey,
				evidenceJSON:   string(evJSONBytes),
			})
			primaryEdges++

			// 2. Secondary same canonical sources (15 edges)
			for _, ss := range n.SecondarySameCanonicalSources {
				if ss.RelationState != string(domain.RelationStateVerified) {
					return nil, fmt.Errorf("secondary source for node %s is not verified (got %s)", n.NodeLogicalID, ss.RelationState)
				}

				var (
					sArcNodeID      int64
					sArcProto       string
					sArcServer      string
					sArcPort        int
					sArcPayload     string
					sArcLinkID      int64
					sArcRevID       int64
					sArcLinkCreated string
					sArcSourceID    int64
					sArcFetchedAt   sql.NullString
					sArcSrcID       int64
					sArcSrcName     string
					sArcSrcURL      string
				)

				sRowErr := arcDB.QueryRowContext(ctx, query, ss.LegacyLinkPK, ss.LegacyNodePK).Scan(
					&sArcNodeID, &sArcProto, &sArcServer, &sArcPort, &sArcPayload,
					&sArcLinkID, &sArcRevID, &sArcLinkCreated,
					&sArcSourceID, &sArcFetchedAt,
					&sArcSrcID, &sArcSrcName, &sArcSrcURL,
				)
				if sRowErr != nil {
					return nil, fmt.Errorf("node %s: secondary archive link chain not found for pk=%d link_pk=%d (fail-closed)", n.NodeLogicalID, ss.LegacyNodePK, ss.LegacyLinkPK)
				}

				sArcCreds, sErr := extractArchiveCredentials(sArcProto, sArcPayload)
				if sErr != nil {
					return nil, fmt.Errorf("node %s: failed to extract secondary archive credentials: %w", n.NodeLogicalID, sErr)
				}
				if !credentialsMatch(arcCreds, sArcCreds, domain.Protocol(n.Protocol)) {
					return nil, fmt.Errorf("node %s: secondary source candidate has conflicting credentials with primary (fail-closed)", n.NodeLogicalID)
				}

				sModernSubID, sModernSubName := findModernSubscription(sArcSrcURL, sArcSrcName, ss.TargetSubscriptionID)
				var sSubIDPtr any = nil
				sSourceLabel := sArcSrcName
				if sModernSubID != "" {
					sSubIDPtr = sModernSubID
					sSourceLabel = sModernSubName
				}

				var sFirstObs any = nil
				if trimmed := strings.TrimSpace(sArcLinkCreated); trimmed != "" {
					sFirstObs = trimmed
				}

				sEvidenceKey := fmt.Sprintf("%s:pk:%d:link:%d", n.ProvenanceOrigin, ss.LegacyNodePK, ss.LegacyLinkPK)
				sEvidenceData := map[string]any{
					"provenance_origin": n.ProvenanceOrigin,
					"relationship_kind": ss.RelationshipKind,
					"legacy_node_pk":    ss.LegacyNodePK,
					"legacy_link_pk":    ss.LegacyLinkPK,
					"legacy_source_id":  ss.LegacySourceID,
					"historical_source": sArcSrcName,
				}
				if len(n.Evidence) > 0 {
					sEvidenceData["archive_sha256"] = n.Evidence[0].SHA256
					sEvidenceData["table_keys"] = n.Evidence[0].TableKeys
				}
				sEvJSONBytes, _ := json.Marshal(sEvidenceData)

				rowsToInsert = append(rowsToInsert, historyRowToInsert{
					nodeLogicalID:  n.NodeLogicalID,
					subIDPtr:       sSubIDPtr,
					sourceIdentity: fmt.Sprintf("legacy:src:%d", ss.LegacySourceID),
					sourceLabel:    sSourceLabel,
					connRev:        nil,
					relationState:  string(domain.RelationStateVerified),
					cause:          string(domain.CauseLegacyImport),
					firstObs:       sFirstObs,
					lastObs:        sFirstObs,
					evidenceKind:   n.ProvenanceOrigin,
					evidenceKey:    sEvidenceKey,
					evidenceJSON:   string(sEvJSONBytes),
				})
				secondaryEdges++
			}

			// 3. Discarded collision notes (isolated, must NOT be attached to node)
			for _, d := range n.DiscardedCollisionNotes {
				if d.RelationshipKind == "discarded_conflicting_credential_candidate" || strings.Contains(d.RelationState, "discarded") {
					discardedCandidates++
				}
			}

		} else if n.ProvenanceOrigin == "v1_subscription_refresh" {
			// v1 backup snapshot verification
			ps := n.PrimaryHistoricalSource
			if ps == nil {
				return nil, fmt.Errorf("node %s missing v1 primary source", n.NodeLogicalID)
			}

			verifiedInBackup := false
			var observedTimeStr string
			var backupSubName string
			var backupSubURL string
			var backupSubFound bool

			for _, ev := range n.Evidence {
				if ev.BackupFile == "" {
					continue
				}
				bPath := resolveArchivePath(cfg.ArchiveDir, ev.BackupFile)
				bDB, bErr := openArchiveReadonly(bPath)
				if bErr != nil {
					continue
				}

				var lastSeenFetchID sql.NullString
				err := bDB.QueryRowContext(ctx, "SELECT last_seen_fetch_id FROM node_sources WHERE node_logical_id = ? AND subscription_id = ?;", n.NodeLogicalID, ps.TargetSubscriptionID).Scan(&lastSeenFetchID)
				if err == nil {
					verifiedInBackup = true
					if lastSeenFetchID.Valid && lastSeenFetchID.String != "" {
						var fetchedAt sql.NullString
						_ = bDB.QueryRowContext(ctx, "SELECT fetched_at FROM subscription_fetches WHERE id = ?;", lastSeenFetchID.String).Scan(&fetchedAt)
						if fetchedAt.Valid && strings.TrimSpace(fetchedAt.String) != "" {
							observedTimeStr = strings.TrimSpace(fetchedAt.String)
						}
					}
					// Runtime schema check: verify target subscription exists in backup DB subscriptions table
					var bName, bURL string
					if sErr := bDB.QueryRowContext(ctx, "SELECT name, source_url_secret_ref FROM subscriptions WHERE id = ?;", ps.TargetSubscriptionID).Scan(&bName, &bURL); sErr == nil {
						backupSubName = bName
						backupSubURL = bURL
						backupSubFound = true
					}
					break
				}
			}

			if !verifiedInBackup {
				return nil, fmt.Errorf("node %s: v1 subscription link not verified in any backup file (fail-closed)", n.NodeLogicalID)
			}
			if !backupSubFound {
				return nil, fmt.Errorf("node %s: v1 target subscription %s not found in backup subscriptions table (fail-closed)", n.NodeLogicalID, ps.TargetSubscriptionID)
			}

			var subIDPtr any = nil
			sourceLabel := ps.HistoricalSourceName
			if backupSubName != "" {
				sourceLabel = backupSubName
			}

			// Verify against modern database:
			if subRec, exists := subMapByID[ps.TargetSubscriptionID]; exists {
				if CanonicalSourceURL(subRec.CanonicalLineageURL) == CanonicalSourceURL(backupSubURL) {
					subIDPtr = subRec.ID
					sourceLabel = subRec.Name
				} else {
					// Actual source ID exists in modern DB but URL does not match:
					// Unmapped historical identity instead of falsely linking!
					subIDPtr = nil
				}
			} else {
				// Target subscription ID does not exist in modern DB
				matchedID, matchedName := findModernSubscription(backupSubURL, backupSubName, ps.TargetSubscriptionID)
				if matchedID != "" {
					subIDPtr = matchedID
					sourceLabel = matchedName
				} else {
					subIDPtr = nil
				}
			}

			var firstObs any = nil
			if observedTimeStr != "" {
				firstObs = observedTimeStr
			}

			evidenceKey := fmt.Sprintf("%s:node:%s:sub:%s", n.ProvenanceOrigin, n.NodeLogicalID, ps.TargetSubscriptionID)
			evidenceData := map[string]any{
				"provenance_origin":   n.ProvenanceOrigin,
				"relationship_kind":   ps.RelationshipKind,
				"target_subscription": ps.TargetSubscriptionName,
				"match_type":          n.NodeMatchType,
			}
			if len(n.Evidence) > 0 {
				evidenceData["backup_file"] = n.Evidence[0].BackupFile
				evidenceData["backup_sha256"] = n.Evidence[0].SHA256
				evidenceData["table_keys"] = n.Evidence[0].TableKeys
			}
			evJSONBytes, _ := json.Marshal(evidenceData)

			rowsToInsert = append(rowsToInsert, historyRowToInsert{
				nodeLogicalID:  n.NodeLogicalID,
				subIDPtr:       subIDPtr,
				sourceIdentity: fmt.Sprintf("v1:sub:%s", ps.TargetSubscriptionID),
				sourceLabel:    sourceLabel,
				connRev:        n.ConnectionRevision, // 23 v1 refresh nodes have real snapshot version (1)
				relationState:  string(domain.RelationStateVerified),
				cause:          string(domain.CauseRefreshRemoved),
				firstObs:       firstObs,
				lastObs:        firstObs,
				evidenceKind:   n.ProvenanceOrigin,
				evidenceKey:    evidenceKey,
				evidenceJSON:   string(evJSONBytes),
			})
			primaryEdges++
		}
	}

	totalEdgesToInsert := primaryEdges + secondaryEdges

	res := &RecoveryResult{
		DryRun:                      cfg.DryRun,
		ManifestPath:                cfg.ManifestPath,
		ManifestSHA256:              actualSHA,
		ExistingHistoryRecords:      existingHistoryRecords,
		TotalCoveredOrphans:         len(manifest.Nodes),
		PrimaryVerifiedEdges:        primaryEdges,
		SecondarySameCanonicalEdges: secondaryEdges,
		DiscardedCandidates:         discardedCandidates,
		TotalEdgesToInsert:          totalEdgesToInsert,
		BaselineInvariants:          baseline,
		InvariantsVerified:          true,
	}

	type existingRecord struct {
		evidenceJSON   string
		sourceIdentity string
		relationState  string
		cause          string
	}

	existingInDB := 0
	if historyTableExists > 0 {
		eRows, err := db.QueryContext(ctx, "SELECT node_logical_id, evidence_key, evidence_json, source_identity, relation_state, cause FROM node_source_history;")
		if err == nil {
			dryExistingMap := make(map[string]existingRecord)
			for eRows.Next() {
				var nID, eKey, eJSON, sID, rState, c string
				if err := eRows.Scan(&nID, &eKey, &eJSON, &sID, &rState, &c); err == nil {
					dryExistingMap[nID+"::"+eKey] = existingRecord{
						evidenceJSON:   eJSON,
						sourceIdentity: sID,
						relationState:  rState,
						cause:          c,
					}
				}
			}
			eRows.Close()

			for _, r := range rowsToInsert {
				mapKey := r.nodeLogicalID + "::" + r.evidenceKey
				if existing, found := dryExistingMap[mapKey]; found {
					if existing.sourceIdentity != r.sourceIdentity || existing.relationState != r.relationState || existing.cause != r.cause {
						return nil, fmt.Errorf("node %s: historical evidence conflict for key %s (existing content differs from recovery candidate: fail-closed)", r.nodeLogicalID, r.evidenceKey)
					}
					existingInDB++
				}
			}
		}
	}

	if cfg.DryRun {
		if historyTableExists == 0 {
			res.PlannedSchema = "migration_16_required"
		} else {
			res.PlannedSchema = "schema_16_ready"
		}

		// Verify 0 writes occurred during dry-run
		if cfg.TargetDBPath != "" && dbFileInitialSize > 0 {
			if fi, fiErr := os.Stat(cfg.TargetDBPath); fiErr == nil {
				if fi.Size() != dbFileInitialSize || !fi.ModTime().Equal(dbFileInitialModTime) {
					return nil, fmt.Errorf("CRITICAL: dry-run modified target database file metadata (fail-closed)")
				}
			}
		}

		res.Status = "dry_run_success"
		res.InsertedRecords = 0
		res.NewlyInserted = 0
		res.ExistingRecords = existingInDB
		res.TotalRecords = totalEdgesToInsert
		res.PredictedNewRecords = totalEdgesToInsert - existingInDB
		res.PostInvariants = baseline
		return res, nil
	}

	// Apply mode: execute in a single strict transaction with invariant guards
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// Pre-query existing records within transaction to guarantee accurate insert counting and immutability
	txExistingMap := make(map[string]existingRecord)
	txERows, err := tx.QueryContext(ctx, "SELECT node_logical_id, evidence_key, evidence_json, source_identity, relation_state, cause FROM node_source_history;")
	if err != nil {
		return nil, fmt.Errorf("failed to query existing history records in transaction: %w", err)
	}
	for txERows.Next() {
		var nID, eKey, eJSON, sID, rState, c string
		if err := txERows.Scan(&nID, &eKey, &eJSON, &sID, &rState, &c); err == nil {
			txExistingMap[nID+"::"+eKey] = existingRecord{
				evidenceJSON:   eJSON,
				sourceIdentity: sID,
				relationState:  rState,
				cause:          c,
			}
		}
	}
	txERows.Close()

	const insertHistorySQL = `
	INSERT INTO node_source_history (
		id,
		node_logical_id,
		subscription_id,
		source_identity,
		source_label,
		connection_revision,
		relation_state,
		cause,
		first_observed_at,
		last_observed_at,
		evidence_kind,
		evidence_key,
		evidence_json,
		created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(node_logical_id, evidence_key) DO NOTHING;`

	stmt, err := tx.PrepareContext(ctx, insertHistorySQL)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare history statement: %w", err)
	}
	defer stmt.Close()

	nowStr := domain.NowUTC().Format(time.RFC3339)
	newlyInserted := 0
	existingCount := 0

	for _, r := range rowsToInsert {
		mapKey := r.nodeLogicalID + "::" + r.evidenceKey
		if existing, found := txExistingMap[mapKey]; found {
			// Evidence key already exists: verify immutable evidence
			if existing.sourceIdentity != r.sourceIdentity || existing.relationState != r.relationState || existing.cause != r.cause {
				return nil, fmt.Errorf("node %s: historical evidence conflict for key %s (existing content differs from recovery candidate, immutable evidence cannot be overwritten: fail-closed)", r.nodeLogicalID, r.evidenceKey)
			}
			existingCount++
			continue
		}

		histID := domain.MustNewUUIDv7()
		execRes, err := stmt.ExecContext(ctx,
			histID,
			r.nodeLogicalID,
			r.subIDPtr,
			r.sourceIdentity,
			r.sourceLabel,
			r.connRev,
			r.relationState,
			r.cause,
			r.firstObs,
			r.lastObs,
			r.evidenceKind,
			r.evidenceKey,
			r.evidenceJSON,
			nowStr,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to insert history record for node %s: %w", r.nodeLogicalID, err)
		}
		rowsAff, _ := execRes.RowsAffected()
		if rowsAff > 0 {
			newlyInserted++
			txExistingMap[mapKey] = existingRecord{
				evidenceJSON:   r.evidenceJSON,
				sourceIdentity: r.sourceIdentity,
				relationState:  r.relationState,
				cause:          r.cause,
			}
		} else {
			existingCount++
		}
	}

	// Post-flight asset invariant check INSIDE transaction before commit
	postFingerprint, postRep, postHeads, err := computeAssetsFingerprint(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("post-flight invariants check failed: %w", err)
	}

	if postFingerprint != baseFingerprint {
		return nil, fmt.Errorf("CRITICAL INVARIANT BREACH: original business assets modified during recovery (fail-closed rollback)")
	}

	if postRep.TotalNodes != baseline.TotalNodes {
		return nil, fmt.Errorf("INVARIANT BREACH: total nodes changed from %d to %d (fail-closed rollback)", baseline.TotalNodes, postRep.TotalNodes)
	}
	if postRep.ActiveNodes != baseline.ActiveNodes {
		return nil, fmt.Errorf("INVARIANT BREACH: active nodes changed from %d to %d (fail-closed rollback)", baseline.ActiveNodes, postRep.ActiveNodes)
	}
	if postRep.NodeSources != baseline.NodeSources {
		return nil, fmt.Errorf("INVARIANT BREACH: node_sources changed from %d to %d (fail-closed rollback)", baseline.NodeSources, postRep.NodeSources)
	}
	if !reflect.DeepEqual(baseline.ScopeENodes, postRep.ScopeENodes) {
		return nil, fmt.Errorf("INVARIANT BREACH: Scope E membership set changed (fail-closed rollback)")
	}
	if len(baseHeads) > 0 && !reflect.DeepEqual(baseHeads, postHeads) {
		return nil, fmt.Errorf("INVARIANT BREACH: node_connection_heads changed (fail-closed rollback)")
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit recovery transaction: %w", err)
	}
	tx = nil

	res.Status = "success"
	res.InsertedRecords = newlyInserted
	res.NewlyInserted = newlyInserted
	res.ExistingRecords = existingCount
	res.TotalRecords = len(rowsToInsert)
	res.PostInvariants = postRep

	return res, nil
}
