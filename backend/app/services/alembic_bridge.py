from __future__ import annotations

import argparse
import json
from pathlib import Path
import sqlite3

from alembic import command
from alembic.config import Config

from app.services.migration_backup import BackupArtifactError, inspect_sqlite_backup
from app.services.migration_preflight import preflight_sqlite_backup


class AlembicBridgeError(RuntimeError):
    pass


LEGACY_STAMP_REVISION = "8f5c2d1a7b90"
LEGACY_BRIDGE_HEAD = "csp_legacy_bootstrap_bridge"
LEGACY_SCHEMA_V1 = {
    "config_snapshots": {"id", "label", "description", "snapshot_data", "created_at"},
    "dns_config": {"id", "raw_yaml", "enabled"},
    "generate_config": {"id", "enabled", "subscriptions", "node_groups", "rules", "dns", "exclude_node_proxies"},
    "node_groups": {"id", "name", "kind", "group_type", "sort_order", "regex_rules", "filter_min_speed_mbps", "filter_media_unlock", "include_nodes", "include_group_ids", "include_group_nodes_ids", "include_entries", "add_fallback", "exclude_nodes", "exclude_group_ids", "url_test_config", "load_balance_config", "fallback_config"},
    "node_probe_results": {"id", "node_key", "name", "server", "port", "type", "status", "latency_ms", "speed_mbps", "ip", "country", "asn", "organization", "media", "error", "checked_at"},
    "probe_config": {"id", "probe_enabled", "probe_interval_minutes", "speedtest_enabled", "speedtest_url", "speedtest_timeout_s", "speedtest_max_bytes", "speedtest_min_speed_mbps", "media_check_enabled", "media_platforms", "media_timeout_s", "probe_concurrency", "probe_timeout_ms"},
    "proxy_chain_bindings": {"id", "target_type", "target_id", "target_name", "dialer_type", "dialer_ref", "enabled", "sort_order", "note"},
    "rule_categories": {"id", "name", "sort_order"},
    "rules": {"id", "name", "category", "type", "value", "proxy", "options", "sort_order", "enabled"},
    "security_settings": {"id", "auth_enabled", "protect_frontend", "protect_api", "protect_exports", "token_hash", "fetch_proxy_enabled", "fetch_proxy_url"},
    "subscriptions": {"id", "name", "url", "update_interval", "is_primary", "enabled", "node_prefix", "filter_regex", "filter_min_speed_mbps", "filter_media_unlock", "include_node_names", "exclude_node_names", "node_renames", "proxy_chain", "node_proxy_chains", "source_nodes", "manual_nodes", "raw_nodes", "last_fetched_at", "last_fetch_error", "fetch_failed_count", "fetch_comments", "subscription_userinfo", "profile_update_interval", "profile_web_page_url"},
}
LEGACY_INDEXES_V1 = {
    "ix_config_snapshots_id",
    "ix_node_groups_id",
    "ix_node_probe_results_checked_at",
    "ix_node_probe_results_name",
    "ix_node_probe_results_node_key",
    "ix_node_probe_results_server",
    "ix_proxy_chain_bindings_id",
    "ix_proxy_chain_bindings_target_id",
    "ix_proxy_chain_bindings_target_type",
    "ix_rule_categories_id",
    "ix_subscriptions_id",
}


def bridge_legacy_sqlite_database(
    database_path: Path | str,
    backup_path: Path | str,
) -> dict[str, object]:
    database = Path(database_path)
    backup = Path(backup_path)
    _validate_backup(database, backup)
    report = preflight_sqlite_backup(database)
    if report["blocking"]:
        raise AlembicBridgeError("Legacy database preflight reported blocking diagnostics")
    _validate_legacy_schema(database)

    config = _alembic_config(database)
    try:
        command.stamp(config, LEGACY_STAMP_REVISION)
        command.upgrade(config, LEGACY_BRIDGE_HEAD)
    except Exception as exc:
        raise AlembicBridgeError("Alembic bridge failed; restore the verified backup before retrying") from exc

    revision = _read_revision(database)
    if revision != LEGACY_BRIDGE_HEAD:
        raise AlembicBridgeError("Alembic bridge did not reach the expected revision")
    return {"revision": revision, "tables": sorted(LEGACY_SCHEMA_V1)}


def _validate_backup(database: Path, backup: Path) -> None:
    if database.resolve() == backup.resolve():
        raise AlembicBridgeError("Backup artifact must be separate from the source database")
    try:
        source_manifest = inspect_sqlite_backup(database)
        backup_manifest = inspect_sqlite_backup(backup)
    except BackupArtifactError as exc:
        raise AlembicBridgeError("Bridge requires a recoverable source and backup artifact") from exc
    if source_manifest.tables != backup_manifest.tables:
        raise AlembicBridgeError("Backup artifact row counts do not match the source database")


def _validate_legacy_schema(database: Path) -> None:
    connection = _open_readonly(database)
    try:
        schema = _read_schema(connection)
        indexes = _read_indexes(connection)
    finally:
        connection.close()
    if "alembic_version" in schema:
        raise AlembicBridgeError("Database already has Alembic revision metadata")
    if schema != LEGACY_SCHEMA_V1 or indexes != LEGACY_INDEXES_V1:
        raise AlembicBridgeError("Database schema does not match the supported legacy v1 start state")


def _alembic_config(database: Path) -> Config:
    backend_root = Path(__file__).resolve().parents[2]
    config = Config(str(backend_root / "alembic.ini"))
    config.set_main_option("script_location", str(backend_root / "alembic"))
    config.set_main_option("sqlalchemy.url", f"sqlite+aiosqlite:///{database.resolve()}")
    return config


def _open_readonly(database: Path) -> sqlite3.Connection:
    try:
        return sqlite3.connect(f"{database.resolve().as_uri()}?mode=ro", uri=True)
    except sqlite3.Error as exc:
        raise AlembicBridgeError("Legacy database cannot be read for bridge validation") from exc


def _read_schema(connection: sqlite3.Connection) -> dict[str, set[str]]:
    rows = connection.execute(
        "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'"
    ).fetchall()
    schema: dict[str, set[str]] = {}
    for (name,) in rows:
        table_name = str(name)
        quoted_name = table_name.replace('"', '""')
        columns = connection.execute(f'PRAGMA table_info("{quoted_name}")').fetchall()
        schema[table_name] = {str(column[1]) for column in columns}
    return schema


def _read_indexes(connection: sqlite3.Connection) -> set[str]:
    rows = connection.execute(
        "SELECT name FROM sqlite_master "
        "WHERE type = 'index' AND name NOT LIKE 'sqlite_autoindex%'"
    ).fetchall()
    return {str(name) for (name,) in rows}


def _read_revision(database: Path) -> str | None:
    connection = sqlite3.connect(database)
    try:
        row = connection.execute("SELECT version_num FROM alembic_version").fetchone()
    except sqlite3.Error as exc:
        raise AlembicBridgeError("Alembic bridge did not create revision metadata") from exc
    finally:
        connection.close()
    return None if row is None else str(row[0])


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--database", required=True)
    parser.add_argument("--backup", required=True)
    args = parser.parse_args()
    try:
        result = bridge_legacy_sqlite_database(args.database, args.backup)
    except AlembicBridgeError as exc:
        parser.exit(2, f"bridge failed: {exc}\n")
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
