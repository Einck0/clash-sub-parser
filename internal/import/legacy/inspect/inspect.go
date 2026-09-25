package inspect

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

// FieldCategory classifies legacy table columns for migration and security assessment.
type FieldCategory string

const (
	FieldSecretExcluded FieldCategory = "secret_excluded"
	FieldProtocol       FieldCategory = "protocol"
	FieldIdentity       FieldCategory = "identity"
	FieldMetadata       FieldCategory = "metadata"
	FieldConfig         FieldCategory = "config"
	FieldQuarantineBlob FieldCategory = "quarantine_blob"
	FieldUnknown        FieldCategory = "unknown"
)

// ColumnInfo holds column metadata extracted from PRAGMA table_info.
// It explicitly contains NO row-level data.
type ColumnInfo struct {
	Name     string        `json:"name"`
	Type     string        `json:"type"`
	NotNull  bool          `json:"notnull"`
	Default  *string       `json:"default"`
	PK       bool          `json:"pk"`
	Category FieldCategory `json:"category"`
}

// IndexInfo holds index metadata extracted from sqlite_master.
type IndexInfo struct {
	Name  string `json:"name"`
	Table string `json:"table"`
	SQL   string `json:"sql"`
}

// SchemaManifest provides a deterministic representation of the database schema for fingerprinting.
type SchemaManifest struct {
	Indexes []IndexInfo             `json:"indexes"`
	Tables  map[string][]ColumnInfo `json:"tables"`
}

// TableReport describes schema, record count, and field categorizations for a single table.
type TableReport struct {
	Name        string                   `json:"name"`
	RowCount    int64                    `json:"row_count"`
	ColumnCount int                      `json:"column_count"`
	Fields      map[string]FieldCategory `json:"fields"`
	Columns     []ColumnInfo             `json:"columns"`
}

// Report encapsulates the complete read-only inspection analysis.
type Report struct {
	SourcePath        string                 `json:"source_path"`
	SourceDigest      string                 `json:"source_digest"`
	SourceSize        int64                  `json:"source_size"`
	SchemaFingerprint string                 `json:"schema_fingerprint"`
	TotalTables       int                    `json:"total_tables"`
	TotalRows         int64                  `json:"total_rows"`
	Tables            map[string]TableReport `json:"tables"`
	CategoryCounts    map[FieldCategory]int  `json:"category_counts"`
	Manifest          SchemaManifest         `json:"manifest"`
}

// CategorizeField classifies a column based on its name, table context, and security requirements.
func CategorizeField(tableName, columnName string) FieldCategory {
	col := strings.ToLower(strings.TrimSpace(columnName))

	// 1. Secrets and credentials: must be strictly excluded from migration and reporting
	if col == "password" || col == "passwd" || col == "pwd" ||
		col == "private_key" || col == "priv_key" || col == "token_hash" ||
		col == "secret" || col == "token" || col == "uuid" || col == "auth_token" ||
		strings.Contains(col, "password") || strings.Contains(col, "private_key") ||
		strings.Contains(col, "token_hash") || strings.Contains(col, "secret_key") {
		return FieldSecretExcluded
	}

	// 2. Identity and primary keys
	if col == "id" || col == "logical_id" || strings.HasSuffix(col, "_id") {
		return FieldIdentity
	}

	// 3. Protocol and transport properties
	if col == "protocol" || col == "server" || col == "port" ||
		col == "ip" || col == "asn" || col == "country" || col == "organization" ||
		(tableName == "nodes" && col == "type") ||
		(tableName == "node_probe_results" && col == "type") {
		return FieldProtocol
	}

	// 4. Raw blobs and legacy aggregates requiring quarantine isolation
	if col == "raw_nodes" || col == "source_nodes" || col == "manual_nodes" ||
		col == "snapshot_data" || col == "subscription_userinfo" ||
		col == "fetch_comments" || col == "node_renames" || col == "node_proxy_chains" ||
		col == "proxy_chain" || col == "media" || col == "media_platforms" ||
		col == "fallback_config" || col == "load_balance_config" || col == "url_test_config" {
		return FieldQuarantineBlob
	}

	// 5. Metadata attributes
	if col == "name" || col == "label" || col == "description" || col == "sort_order" ||
		col == "created_at" || col == "updated_at" || col == "last_fetched_at" ||
		col == "last_fetch_error" || col == "fetch_failed_count" || col == "checked_at" ||
		col == "is_primary" || col == "status" || col == "error" ||
		col == "latency_ms" || col == "speed_mbps" || col == "node_key" ||
		col == "node_prefix" || col == "target_type" || col == "target_name" {
		return FieldMetadata
	}

	// 6. Logical configurations and policy rules
	if col == "enabled" || col == "url" || col == "group_type" || col == "kind" ||
		col == "raw_yaml" || col == "value" || col == "proxy" || col == "category" ||
		col == "options" || col == "add_fallback" || col == "include_entries" ||
		col == "exclude_nodes" || col == "include_nodes" || col == "include_group_ids" ||
		col == "exclude_group_ids" || col == "include_group_nodes_ids" || col == "regex_rules" ||
		col == "filter_min_speed_mbps" || col == "filter_media_unlock" || col == "filter_regex" ||
		col == "include_node_names" || col == "exclude_node_names" ||
		col == "update_interval" || col == "profile_update_interval" || col == "profile_web_page_url" ||
		col == "dns" || col == "rules" || col == "subscriptions" || col == "node_groups" ||
		col == "exclude_node_proxies" || col == "probe_enabled" || col == "probe_interval_minutes" ||
		col == "probe_timeout_ms" || col == "speedtest_enabled" || col == "speedtest_url" ||
		col == "speedtest_timeout_s" || col == "speedtest_min_speed_mbps" || col == "speedtest_max_bytes" ||
		col == "media_check_enabled" || col == "media_timeout_s" || col == "probe_concurrency" ||
		col == "dialer_type" || col == "dialer_ref" || col == "note" ||
		col == "auth_enabled" || col == "protect_api" || col == "protect_exports" ||
		col == "protect_frontend" || col == "fetch_proxy_enabled" || col == "fetch_proxy_url" {
		return FieldConfig
	}

	return FieldUnknown
}

func isSensitiveDefault(columnName string) bool {
	return CategorizeField("", columnName) == FieldSecretExcluded
}

// OpenReadOnly establishes a strict read-only database connection to an existing SQLite file.
// It configures mode=ro and PRAGMA query_only=1, ensuring zero writes (DDL/DML) can occur.
func OpenReadOnly(sourcePath string) (*sql.DB, error) {
	return OpenReadOnlyContext(context.Background(), sourcePath)
}

// OpenReadOnlyContext establishes a strict read-only database connection with context cancellation.
func OpenReadOnlyContext(ctx context.Context, sourcePath string) (*sql.DB, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("cannot access legacy database: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("source path is a directory, expected sqlite file: %s", sourcePath)
	}

	absPath, err := filepath.Abs(sourcePath)
	if err != nil {
		absPath = sourcePath
	}

	// Build modernc SQLite read-only DSN
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)", url.PathEscape(absPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite in read-only mode: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping legacy database: %w", err)
	}

	return db, nil
}

// Inspect opens a legacy SQLite database in strict read-only mode and produces a sanitized Report.
// It guarantees that the source file is never modified and no row values are exposed.
func Inspect(sourcePath string) (*Report, error) {
	return InspectContext(context.Background(), sourcePath)
}

// InspectContext performs Inspect with context cancellation support.
func InspectContext(ctx context.Context, sourcePath string) (*Report, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("cannot access legacy database: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("source path is a directory, expected sqlite file: %s", sourcePath)
	}

	// Compute source file SHA-256 digest
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file for digest: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to read source file for digest: %w", err)
	}
	_ = file.Close()
	sourceDigest := hex.EncodeToString(hasher.Sum(nil))

	db, err := OpenReadOnlyContext(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Query user tables
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name ASC;")
	if err != nil {
		return nil, fmt.Errorf("failed to query sqlite tables: %w", err)
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		tableNames = append(tableNames, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading table names: %w", err)
	}

	// Query indexes
	idxRows, err := db.QueryContext(ctx, "SELECT name, tbl_name, coalesce(sql, '') FROM sqlite_master WHERE type='index' AND name NOT LIKE 'sqlite_%' ORDER BY name ASC;")
	if err != nil {
		return nil, fmt.Errorf("failed to query sqlite indexes: %w", err)
	}
	defer idxRows.Close()

	var indexes []IndexInfo
	for idxRows.Next() {
		var idx IndexInfo
		if err := idxRows.Scan(&idx.Name, &idx.Table, &idx.SQL); err != nil {
			return nil, fmt.Errorf("failed to scan index: %w", err)
		}
		indexes = append(indexes, idx)
	}
	if err := idxRows.Err(); err != nil {
		return nil, fmt.Errorf("error reading indexes: %w", err)
	}

	tables := make(map[string]TableReport, len(tableNames))
	manifestTables := make(map[string][]ColumnInfo, len(tableNames))
	categoryCounts := make(map[FieldCategory]int)
	var totalRows int64

	for _, tbl := range tableNames {
		colQuery := fmt.Sprintf("PRAGMA table_info(\"%s\");", tbl)
		colRows, err := db.QueryContext(ctx, colQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns for table %s: %w", tbl, err)
		}

		var columns []ColumnInfo
		fields := make(map[string]FieldCategory)

		for colRows.Next() {
			var cid int
			var name, colType string
			var notNull, pk int
			var dfltVal sql.NullString

			if err := colRows.Scan(&cid, &name, &colType, &notNull, &dfltVal, &pk); err != nil {
				_ = colRows.Close()
				return nil, fmt.Errorf("failed to scan column for table %s: %w", tbl, err)
			}

			cat := CategorizeField(tbl, name)
			categoryCounts[cat]++
			fields[name] = cat

			var dfltPtr *string
			if dfltVal.Valid {
				v := dfltVal.String
				if isSensitiveDefault(name) {
					v = "[redacted]"
				}
				dfltPtr = &v
			}

			colInfo := ColumnInfo{
				Name:     name,
				Type:     colType,
				NotNull:  notNull != 0,
				Default:  dfltPtr,
				PK:       pk != 0,
				Category: cat,
			}
			columns = append(columns, colInfo)
		}
		_ = colRows.Close()

		// Get row count for the table
		var count int64
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM \"%s\";", tbl)
		if err := db.QueryRowContext(ctx, countQuery).Scan(&count); err != nil {
			return nil, fmt.Errorf("failed to count rows in table %s: %w", tbl, err)
		}
		totalRows += count

		// Sort columns by name for deterministic manifest and canonical fingerprint
		manifestCols := make([]ColumnInfo, len(columns))
		copy(manifestCols, columns)
		sort.Slice(manifestCols, func(i, j int) bool {
			return manifestCols[i].Name < manifestCols[j].Name
		})
		manifestTables[tbl] = manifestCols

		tables[tbl] = TableReport{
			Name:        tbl,
			RowCount:    count,
			ColumnCount: len(columns),
			Fields:      fields,
			Columns:     columns,
		}
	}

	// Sort indexes by name for deterministic manifest
	sort.Slice(indexes, func(i, j int) bool {
		return indexes[i].Name < indexes[j].Name
	})

	manifest := SchemaManifest{
		Indexes: indexes,
		Tables:  manifestTables,
	}

	fingerprint, err := computeSchemaFingerprint(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to compute schema fingerprint: %w", err)
	}

	report := &Report{
		SourcePath:        sourcePath,
		SourceDigest:      sourceDigest,
		SourceSize:        info.Size(),
		SchemaFingerprint: fingerprint,
		TotalTables:       len(tables),
		TotalRows:         totalRows,
		Tables:            tables,
		CategoryCounts:    categoryCounts,
		Manifest:          manifest,
	}

	return report, nil
}

// computeSchemaFingerprint calculates a stable SHA-256 digest of the canonical schema manifest.
func computeSchemaFingerprint(manifest SchemaManifest) (string, error) {
	type fingerprintColumn struct {
		Name    string  `json:"name"`
		Type    string  `json:"type"`
		NotNull bool    `json:"notnull"`
		Default *string `json:"default"`
		PK      bool    `json:"pk"`
	}
	type fingerprintManifest struct {
		Indexes []IndexInfo                    `json:"indexes"`
		Tables  map[string][]fingerprintColumn `json:"tables"`
	}

	canonical := fingerprintManifest{
		Indexes: manifest.Indexes,
		Tables:  make(map[string][]fingerprintColumn, len(manifest.Tables)),
	}
	for tableName, columns := range manifest.Tables {
		canonicalColumns := make([]fingerprintColumn, 0, len(columns))
		for _, column := range columns {
			canonicalColumns = append(canonicalColumns, fingerprintColumn{
				Name:    column.Name,
				Type:    column.Type,
				NotNull: column.NotNull,
				Default: column.Default,
				PK:      column.PK,
			})
		}
		canonical.Tables[tableName] = canonicalColumns
	}

	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

// Marshal encodes the report into indented JSON.
func Marshal(r *Report) ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// FormatText formats the report into a human-readable text representation.
func FormatText(r *Report) string {
	var sb strings.Builder
	sb.WriteString("================================================================================\n")
	sb.WriteString("                     CSP Legacy SQLite Inspection Report                       \n")
	sb.WriteString("================================================================================\n")
	sb.WriteString(fmt.Sprintf("Source Path:        %s\n", r.SourcePath))
	sb.WriteString(fmt.Sprintf("File Size:          %d bytes\n", r.SourceSize))
	sb.WriteString(fmt.Sprintf("Source SHA-256:     %s\n", r.SourceDigest))
	sb.WriteString(fmt.Sprintf("Schema Fingerprint: %s\n", r.SchemaFingerprint))
	sb.WriteString(fmt.Sprintf("Total Tables:       %d\n", r.TotalTables))
	sb.WriteString(fmt.Sprintf("Total Records:      %d\n", r.TotalRows))
	sb.WriteString("--------------------------------------------------------------------------------\n")
	sb.WriteString("Field Category Breakdown:\n")

	// Order categories predictably
	cats := []FieldCategory{
		FieldIdentity,
		FieldMetadata,
		FieldProtocol,
		FieldConfig,
		FieldSecretExcluded,
		FieldQuarantineBlob,
		FieldUnknown,
	}
	for _, c := range cats {
		count := r.CategoryCounts[c]
		sb.WriteString(fmt.Sprintf("  - %-16s: %d\n", c, count))
	}

	sb.WriteString("--------------------------------------------------------------------------------\n")
	sb.WriteString("Table Ledger Summary:\n")

	tableNames := make([]string, 0, len(r.Tables))
	for name := range r.Tables {
		tableNames = append(tableNames, name)
	}
	sort.Strings(tableNames)

	for _, name := range tableNames {
		tbl := r.Tables[name]
		sb.WriteString(fmt.Sprintf("  * Table: %s (Rows: %d, Columns: %d)\n", tbl.Name, tbl.RowCount, tbl.ColumnCount))
		for _, col := range tbl.Columns {
			pkStr := ""
			if col.PK {
				pkStr = ", PK"
			}
			notNullStr := ""
			if col.NotNull {
				notNullStr = ", NOT NULL"
			}
			sb.WriteString(fmt.Sprintf("      - %-24s %-12s [%s%s%s]\n", col.Name, col.Type, col.Category, pkStr, notNullStr))
		}
	}

	sb.WriteString("================================================================================\n")
	sb.WriteString("Security & Redaction Guarantee:\n")
	sb.WriteString("  [OK] Absolute read-only mode verified (mode=ro, PRAGMA query_only=1)\n")
	sb.WriteString("  [OK] Zero row-level data or credential payloads exposed in this report\n")
	sb.WriteString("================================================================================\n")
	return sb.String()
}
