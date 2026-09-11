package repository

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Options defines connection pool and tuning options for SQLite.
type Options struct {
	Path         string
	MaxOpenConns int
	MaxIdleConns int
	BusyTimeout  time.Duration
	ReadOnly     bool
}

// SQLiteDB manages the pure Go SQLite connection pool and repository factories.
type SQLiteDB struct {
	db   *sql.DB
	opts Options
}

// NewSQLiteDB initializes a SQLite database connection pool using modernc.org/sqlite.
// It enforces WAL journal mode, 30s busy timeout, and foreign key integrity.
func NewSQLiteDB(opts Options) (*SQLiteDB, error) {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		path = "file:clash_sub_parser.db"
	}

	busyMs := 30000
	if opts.BusyTimeout > 0 {
		busyMs = int(opts.BusyTimeout.Milliseconds())
	}

	dsn := path
	if opts.ReadOnly {
		// Ensure file URI format for strict read-only enforcement
		if !strings.HasPrefix(dsn, "file:") && dsn != ":memory:" {
			absPath, err := filepath.Abs(dsn)
			if err == nil {
				dsn = fmt.Sprintf("file:%s", filepath.ToSlash(absPath))
			} else {
				dsn = fmt.Sprintf("file:%s", dsn)
			}
		}
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}

	var pragmas []string
	if opts.ReadOnly && !strings.Contains(dsn, "mode=ro") {
		pragmas = append(pragmas, "mode=ro")
	}
	pragmas = append(pragmas,
		fmt.Sprintf("_pragma=busy_timeout(%d)", busyMs),
		"_pragma=journal_mode(WAL)",
		"_pragma=foreign_keys(ON)",
	)

	dsn = fmt.Sprintf("%s%s%s", dsn, separator, strings.Join(pragmas, "&"))

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite connection: %w", err)
	}

	maxOpen := 25
	if opts.MaxOpenConns > 0 {
		maxOpen = opts.MaxOpenConns
	}
	maxIdle := 5
	if opts.MaxIdleConns > 0 {
		maxIdle = opts.MaxIdleConns
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Hour)

	// Explicitly execute PRAGMAs to guarantee strict WAL mode and foreign keys
	if !opts.ReadOnly {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d;", busyMs)); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set busy_timeout: %w", err)
		}
		if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set journal_mode WAL: %w", err)
		}
		if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set foreign_keys ON: %w", err)
		}
	} else {
		// Read-only connections also benefit from busy_timeout and foreign_keys
		_, _ = db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d;", busyMs))
		_, _ = db.Exec("PRAGMA foreign_keys = ON;")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite db: %w", err)
	}

	return &SQLiteDB{
		db:   db,
		opts: opts,
	}, nil
}

// Close closes the underlying connection pool.
func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

// DB returns the underlying sql.DB instance.
func (s *SQLiteDB) DB() *sql.DB {
	return s.db
}

// ExecContext executes a query without returning any rows.
func (s *SQLiteDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

// QueryContext executes a query that returns rows.
func (s *SQLiteDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query that is expected to return at most one row.
func (s *SQLiteDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// BeginTx starts a transaction with the given options.
func (s *SQLiteDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, opts)
}

// InitSchema ensures all required tables, indexes, and baseline seeds exist.
func (s *SQLiteDB) InitSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(120) NOT NULL UNIQUE,
		url TEXT NOT NULL,
		update_interval INTEGER,
		is_primary BOOLEAN NOT NULL DEFAULT 0,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		node_prefix VARCHAR(120),
		filter_regex TEXT NOT NULL DEFAULT '[]',
		filter_min_speed_mbps FLOAT,
		filter_media_unlock TEXT NOT NULL DEFAULT '[]',
		include_node_names TEXT NOT NULL DEFAULT '[]',
		exclude_node_names TEXT NOT NULL DEFAULT '[]',
		node_renames TEXT NOT NULL DEFAULT '{}',
		source_nodes TEXT NOT NULL DEFAULT '[]',
		manual_nodes TEXT NOT NULL DEFAULT '[]',
		raw_nodes TEXT NOT NULL DEFAULT '[]',
		last_fetched_at TIMESTAMP,
		last_fetch_error TEXT,
		fetch_failed_count INTEGER NOT NULL DEFAULT 0,
		fetch_comments TEXT NOT NULL DEFAULT '[]',
		subscription_userinfo TEXT,
		profile_update_interval VARCHAR(40),
		profile_web_page_url TEXT,
		proxy_chain TEXT,
		node_proxy_chains TEXT NOT NULL DEFAULT '{}'
	);
	CREATE INDEX IF NOT EXISTS ix_subscriptions_id ON subscriptions (id);
	CREATE INDEX IF NOT EXISTS ix_subscriptions_is_primary ON subscriptions (is_primary);
	CREATE INDEX IF NOT EXISTS ix_subscriptions_enabled ON subscriptions (enabled);

	CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		logical_id VARCHAR(36) NOT NULL UNIQUE,
		name VARCHAR(255) NOT NULL,
		protocol VARCHAR(64) NOT NULL,
		server VARCHAR(255) NOT NULL,
		port INTEGER NOT NULL,
		normalized_payload TEXT NOT NULL,
		payload_fingerprint VARCHAR(64) NOT NULL,
		lifecycle_state VARCHAR(32) NOT NULL DEFAULT 'active',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS ix_nodes_id ON nodes (id);
	CREATE INDEX IF NOT EXISTS ix_nodes_logical_id ON nodes (logical_id);
	CREATE INDEX IF NOT EXISTS ix_nodes_payload_fingerprint ON nodes (payload_fingerprint);

	CREATE TABLE IF NOT EXISTS node_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(120) NOT NULL UNIQUE,
		kind VARCHAR(20) NOT NULL DEFAULT 'manual',
		group_type VARCHAR(20) NOT NULL DEFAULT 'select',
		sort_order INTEGER NOT NULL DEFAULT 0,
		regex_rules TEXT NOT NULL DEFAULT '[]',
		filter_min_speed_mbps FLOAT,
		filter_media_unlock TEXT NOT NULL DEFAULT '[]',
		include_nodes TEXT NOT NULL DEFAULT '[]',
		include_group_ids TEXT NOT NULL DEFAULT '[]',
		include_group_nodes_ids TEXT NOT NULL DEFAULT '[]',
		include_entries TEXT NOT NULL DEFAULT '[]',
		add_fallback BOOLEAN NOT NULL DEFAULT 0,
		exclude_nodes TEXT NOT NULL DEFAULT '[]',
		exclude_group_ids TEXT NOT NULL DEFAULT '[]',
		url_test_config TEXT NOT NULL DEFAULT '{}',
		load_balance_config TEXT NOT NULL DEFAULT '{}',
		fallback_config TEXT NOT NULL DEFAULT '{}'
	);
	CREATE INDEX IF NOT EXISTS ix_node_groups_id ON node_groups (id);
	CREATE INDEX IF NOT EXISTS ix_node_groups_sort_order ON node_groups (sort_order);

	CREATE TABLE IF NOT EXISTS rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(160) NOT NULL DEFAULT '',
		category VARCHAR(80) NOT NULL DEFAULT 'default',
		type VARCHAR(40) NOT NULL,
		value VARCHAR(512) NOT NULL DEFAULT '',
		proxy VARCHAR(160) NOT NULL,
		options TEXT NOT NULL DEFAULT '[]',
		sort_order INTEGER NOT NULL DEFAULT 0,
		enabled BOOLEAN NOT NULL DEFAULT 1
	);
	CREATE INDEX IF NOT EXISTS ix_rules_id ON rules (id);
	CREATE INDEX IF NOT EXISTS ix_rules_category ON rules (category);
	CREATE INDEX IF NOT EXISTS ix_rules_sort_order ON rules (sort_order);

	CREATE TABLE IF NOT EXISTS node_probe_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		node_key VARCHAR(255) NOT NULL UNIQUE,
		name VARCHAR(255) NOT NULL,
		server VARCHAR(255) NOT NULL,
		port INTEGER,
		type VARCHAR(64) NOT NULL,
		status VARCHAR(32) NOT NULL,
		latency_ms INTEGER,
		speed_mbps FLOAT,
		ip VARCHAR(128),
		country VARCHAR(32),
		asn INTEGER,
		organization VARCHAR(255),
		media TEXT NOT NULL DEFAULT '{}',
		error TEXT,
		checked_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS ix_node_probe_results_id ON node_probe_results (id);
	CREATE INDEX IF NOT EXISTS ix_node_probe_results_node_key ON node_probe_results (node_key);
	CREATE INDEX IF NOT EXISTS ix_node_probe_results_name ON node_probe_results (name);
	CREATE INDEX IF NOT EXISTS ix_node_probe_results_server ON node_probe_results (server);
	CREATE INDEX IF NOT EXISTS ix_node_probe_results_checked_at ON node_probe_results (checked_at);

	CREATE TABLE IF NOT EXISTS generate_config (
		id INTEGER PRIMARY KEY,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		subscriptions BOOLEAN NOT NULL DEFAULT 1,
		node_groups BOOLEAN NOT NULL DEFAULT 1,
		rules BOOLEAN NOT NULL DEFAULT 1,
		dns BOOLEAN NOT NULL DEFAULT 1,
		exclude_node_proxies BOOLEAN NOT NULL DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS dns_config (
		id INTEGER PRIMARY KEY,
		raw_yaml TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1
	);

	CREATE TABLE IF NOT EXISTS rule_categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name VARCHAR(80) NOT NULL UNIQUE,
		sort_order INTEGER NOT NULL DEFAULT 0
	);

	-- Baseline default configuration records if absent
	INSERT OR IGNORE INTO generate_config (id, enabled, subscriptions, node_groups, rules, dns, exclude_node_proxies)
	VALUES (1, 1, 1, 1, 1, 1, 1);

	INSERT OR IGNORE INTO dns_config (id, raw_yaml, enabled)
	VALUES (1, '', 1);
	`

	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}

	return nil
}

// Factory methods

func (s *SQLiteDB) NewSubscriptionRepository() SubscriptionRepository {
	return &sqliteSubscriptionRepository{db: s}
}

func (s *SQLiteDB) NewNodeRepository() NodeRepository {
	return &sqliteNodeRepository{db: s}
}

func (s *SQLiteDB) NewNodeGroupRepository() NodeGroupRepository {
	return &sqliteNodeGroupRepository{db: s}
}

func (s *SQLiteDB) NewRuleRepository() RuleRepository {
	return &sqliteRuleRepository{db: s}
}

func (s *SQLiteDB) NewProbeRepository() ProbeRepository {
	return &sqliteProbeRepository{db: s}
}

func (s *SQLiteDB) NewGenerateConfigRepository() GenerateConfigRepository {
	return &sqliteGenerateConfigRepository{db: s}
}

func (s *SQLiteDB) NewDNSConfigRepository() DNSConfigRepository {
	return &sqliteDNSConfigRepository{db: s}
}

func (s *SQLiteDB) Repositories() *Repositories {
	return &Repositories{
		Subscriptions:  s.NewSubscriptionRepository(),
		Nodes:          s.NewNodeRepository(),
		NodeGroups:     s.NewNodeGroupRepository(),
		Rules:          s.NewRuleRepository(),
		Probes:         s.NewProbeRepository(),
		GenerateConfig: s.NewGenerateConfigRepository(),
		DNSConfig:      s.NewDNSConfigRepository(),
	}
}
