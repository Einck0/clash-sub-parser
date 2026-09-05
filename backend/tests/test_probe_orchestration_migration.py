from __future__ import annotations

from pathlib import Path
import sqlite3

from alembic import command
from alembic.config import Config
from alembic.script import ScriptDirectory
import pytest
from sqlalchemy import create_engine, inspect

BACKEND_ROOT = Path(__file__).resolve().parents[1]
TARGET_HEAD_REVISION = "probe_orchestration_config"


def _alembic_config(db_file: Path) -> Config:
    config = Config(str(BACKEND_ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    config.set_main_option("sqlalchemy.url", f"sqlite+aiosqlite:///{db_file.resolve()}")
    return config


def _columns(engine, table_name: str) -> dict[str, dict]:
    inspector = inspect(engine)
    cols = inspector.get_columns(table_name)
    return {c["name"]: {"type": str(c["type"]).upper(), "nullable": c["nullable"], "default": c.get("default")} for c in cols}


def test_probe_orchestration_head_in_script_directory() -> None:
    config = Config(str(BACKEND_ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    script = ScriptDirectory.from_config(config)
    assert TARGET_HEAD_REVISION in script.get_heads()


def test_sqlite_upgrade_to_head_creates_probe_orchestration_columns(tmp_path) -> None:
    db_file = tmp_path / "probe_clean.sqlite"
    config = _alembic_config(db_file)
    command.upgrade(config, "head")

    engine = create_engine(f"sqlite:///{db_file.resolve()}")
    try:
        cols = _columns(engine, "probe_config")
        assert "probe_concurrency" in cols
        assert "probe_service_timeout_ms" in cols
        assert "probe_cron_enabled" in cols
        assert "probe_cron_interval_minutes" in cols
    finally:
        engine.dispose()


def test_migration_preserves_existing_concurrency_and_probe_results(tmp_path) -> None:
    db_file = tmp_path / "probe_upgrade_existing.sqlite"
    config = _alembic_config(db_file)

    # Upgrade to previous head (csp_target_domain_schema)
    command.upgrade(config, "csp_target_domain_schema")

    # Populate probe_config with concurrency=7 (custom saved value) and a probe result
    conn = sqlite3.connect(db_file)
    try:
        conn.execute(
            "INSERT INTO probe_config (id, probe_enabled, probe_interval_minutes, speedtest_enabled, "
            "speedtest_url, speedtest_timeout_s, speedtest_max_bytes, speedtest_min_speed_mbps, "
            "media_check_enabled, media_platforms, media_timeout_s, probe_concurrency, probe_timeout_ms) "
            "VALUES (1, 1, 30, 0, 'http://test', 5, 1024, 0.0, 1, '[]', 5, 7, 3000)"
        )
        conn.execute(
            "INSERT INTO node_probe_results (id, node_key, name, server, port, type, status, "
            "latency_ms, speed_mbps, ip, country, asn, organization, media, error, checked_at) "
            "VALUES (1, 'k1', 'node1', '1.1.1.1', 443, 'ss', 'ok', 50, 10.5, '1.1.1.1', 'US', 1234, 'Org', '{}', NULL, 1000)"
        )
        conn.commit()
    finally:
        conn.close()

    # Upgrade to head
    command.upgrade(config, "head")

    # Verify custom concurrency is preserved, new defaults applied, and node_probe_results intact
    conn = sqlite3.connect(db_file)
    try:
        row = conn.execute(
            "SELECT probe_concurrency, probe_service_timeout_ms, probe_cron_enabled, probe_cron_interval_minutes "
            "FROM probe_config WHERE id = 1"
        ).fetchone()
        assert row is not None
        assert row[0] == 7, "Explicitly saved concurrency must be preserved"
        assert row[1] == 2000, "probe_service_timeout_ms default must be 2000"
        assert row[2] in (1, True), "probe_cron_enabled default must be True/1"
        assert row[3] == 60, "probe_cron_interval_minutes default must be 60"

        res_count = conn.execute("SELECT COUNT(*) FROM node_probe_results").fetchone()[0]
        assert res_count == 1, "node_probe_results must be preserved"
    finally:
        conn.close()


def test_downgrade_and_reupgrade_cycle(tmp_path) -> None:
    db_file = tmp_path / "probe_downgrade.sqlite"
    config = _alembic_config(db_file)

    command.upgrade(config, "head")
    command.downgrade(config, "csp_target_domain_schema")

    engine = create_engine(f"sqlite:///{db_file.resolve()}")
    try:
        cols = _columns(engine, "probe_config")
        assert "probe_service_timeout_ms" not in cols
        assert "probe_cron_enabled" not in cols
        assert "probe_cron_interval_minutes" not in cols
    finally:
        engine.dispose()

    # Re-upgrade to head
    command.upgrade(config, "head")
    engine = create_engine(f"sqlite:///{db_file.resolve()}")
    try:
        cols = _columns(engine, "probe_config")
        assert "probe_service_timeout_ms" in cols
        assert "probe_cron_enabled" in cols
        assert "probe_cron_interval_minutes" in cols
    finally:
        engine.dispose()
