package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"clash-sub-parser/internal/domain"
)

type sqliteGenerateConfigRepository struct {
	db *SQLiteDB
}

func (r *sqliteGenerateConfigRepository) Get(ctx context.Context) (*domain.GenerateConfig, error) {
	query := `SELECT id, enabled, subscriptions, node_groups, rules, dns, exclude_node_proxies FROM generate_config WHERE id = 1`
	var (
		cfg                                                           domain.GenerateConfig
		enabledInt, subsInt, groupsInt, rulesInt, dnsInt, excludeInt int64
	)

	err := r.db.QueryRowContext(ctx, query).Scan(
		&cfg.ID,
		&enabledInt,
		&subsInt,
		&groupsInt,
		&rulesInt,
		&dnsInt,
		&excludeInt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.DefaultGenerateConfig(), nil
		}
		return nil, fmt.Errorf("get generate config: %w", err)
	}

	cfg.Enabled = intToBool(enabledInt)
	cfg.Subscriptions = intToBool(subsInt)
	cfg.NodeGroups = intToBool(groupsInt)
	cfg.Rules = intToBool(rulesInt)
	cfg.DNS = intToBool(dnsInt)
	cfg.ExcludeNodeProxies = intToBool(excludeInt)

	return &cfg, nil
}

func (r *sqliteGenerateConfigRepository) Update(ctx context.Context, config *domain.GenerateConfig) error {
	if config == nil {
		return ErrNilEntity
	}

	query := `
		INSERT INTO generate_config (id, enabled, subscriptions, node_groups, rules, dns, exclude_node_proxies)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			enabled = excluded.enabled,
			subscriptions = excluded.subscriptions,
			node_groups = excluded.node_groups,
			rules = excluded.rules,
			dns = excluded.dns,
			exclude_node_proxies = excluded.exclude_node_proxies
	`

	_, err := r.db.ExecContext(ctx, query,
		boolToInt(config.Enabled),
		boolToInt(config.Subscriptions),
		boolToInt(config.NodeGroups),
		boolToInt(config.Rules),
		boolToInt(config.DNS),
		boolToInt(config.ExcludeNodeProxies),
	)
	if err != nil {
		return fmt.Errorf("update generate config: %w", err)
	}

	return nil
}

type sqliteDNSConfigRepository struct {
	db *SQLiteDB
}

func (r *sqliteDNSConfigRepository) Get(ctx context.Context) (*domain.DNSConfig, error) {
	query := `SELECT id, raw_yaml, enabled FROM dns_config WHERE id = 1`
	var (
		cfg        domain.DNSConfig
		enabledInt int64
	)

	err := r.db.QueryRowContext(ctx, query).Scan(&cfg.ID, &cfg.RawYAML, &enabledInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.DefaultDNSConfig(), nil
		}
		return nil, fmt.Errorf("get dns config: %w", err)
	}

	cfg.Enabled = intToBool(enabledInt)
	return &cfg, nil
}

func (r *sqliteDNSConfigRepository) Update(ctx context.Context, config *domain.DNSConfig) error {
	if config == nil {
		return ErrNilEntity
	}

	query := `
		INSERT INTO dns_config (id, raw_yaml, enabled)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			raw_yaml = excluded.raw_yaml,
			enabled = excluded.enabled
	`

	_, err := r.db.ExecContext(ctx, query,
		config.RawYAML,
		boolToInt(config.Enabled),
	)
	if err != nil {
		return fmt.Errorf("update dns config: %w", err)
	}

	return nil
}
