from __future__ import annotations

from pathlib import Path
import sqlite3

from alembic import command
from alembic.config import Config
import pytest

from app.services.alembic_bridge import (
    AlembicBridgeError,
    LEGACY_BRIDGE_HEAD,
    bridge_legacy_sqlite_database,
)


FIXTURE_PATH = Path(__file__).parent / "fixtures" / "legacy-alembic-start-state-v1.sql"


def _create_legacy_start_state(path: Path) -> None:
    connection = sqlite3.connect(path)
    try:
        connection.executescript(FIXTURE_PATH.read_text(encoding="utf-8"))
        connection.commit()
    finally:
        connection.close()


def _copy_sqlite(source: Path, destination: Path) -> None:
    source_connection = sqlite3.connect(source)
    destination_connection = sqlite3.connect(destination)
    try:
        source_connection.backup(destination_connection)
    finally:
        destination_connection.close()
        source_connection.close()


def _tables(path: Path) -> set[str]:
    connection = sqlite3.connect(path)
    try:
        return {
            row[0]
            for row in connection.execute(
                "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name"
            )
        }
    finally:
        connection.close()


def _indexes(path: Path) -> set[str]:
    connection = sqlite3.connect(path)
    try:
        return {
            row[0]
            for row in connection.execute(
                "SELECT name FROM sqlite_master "
                "WHERE type = 'index' AND name NOT LIKE 'sqlite_autoindex%'"
            )
        }
    finally:
        connection.close()


def _schema(path: Path) -> dict[str, set[str]]:
    connection = sqlite3.connect(path)
    try:
        tables = _tables(path)
        return {
            table_name: {
                row[1]
                for row in connection.execute(f'PRAGMA table_info("{table_name}")')
            }
            for table_name in tables
            if table_name != "alembic_version"
        }
    finally:
        connection.close()


def _revision(path: Path) -> str | None:
    connection = sqlite3.connect(path)
    try:
        if "alembic_version" not in _tables(path):
            return None
        row = connection.execute("SELECT version_num FROM alembic_version").fetchone()
        return None if row is None else str(row[0])
    finally:
        connection.close()


def _alembic_config(path: Path) -> Config:
    backend_root = Path(__file__).parents[1]
    config = Config(str(backend_root / "alembic.ini"))
    config.set_main_option("script_location", str(backend_root / "alembic"))
    config.set_main_option("sqlalchemy.url", f"sqlite+aiosqlite:///{path.resolve()}")
    return config


def test_clean_database_upgrade_reaches_legacy_bridge_head(tmp_path) -> None:
    database_path = tmp_path / "clean.sqlite"
    fixture_path = tmp_path / "fixture.sqlite"
    _create_legacy_start_state(fixture_path)

    command.upgrade(_alembic_config(database_path), LEGACY_BRIDGE_HEAD)

    assert _revision(database_path) == LEGACY_BRIDGE_HEAD
    assert _tables(database_path) == _tables(fixture_path) | {"alembic_version"}


def test_bridge_upgrades_legacy_start_state_to_deterministic_head(tmp_path) -> None:
    database_path = tmp_path / "legacy.sqlite"
    backup_path = tmp_path / "legacy-backup.sqlite"
    _create_legacy_start_state(database_path)
    _copy_sqlite(database_path, backup_path)

    result = bridge_legacy_sqlite_database(database_path, backup_path)

    assert result["revision"] == LEGACY_BRIDGE_HEAD
    assert _revision(database_path) == LEGACY_BRIDGE_HEAD
    assert _tables(database_path) == _tables(backup_path) | {"alembic_version"}


def test_bridge_rejects_unknown_schema_without_writing_version_table(tmp_path) -> None:
    database_path = tmp_path / "unknown.sqlite"
    backup_path = tmp_path / "unknown-backup.sqlite"
    _create_legacy_start_state(database_path)
    connection = sqlite3.connect(database_path)
    try:
        connection.execute("CREATE TABLE unknown_legacy_table (id INTEGER PRIMARY KEY)")
        connection.commit()
    finally:
        connection.close()
    _copy_sqlite(database_path, backup_path)
    before = database_path.read_bytes()

    with pytest.raises(AlembicBridgeError):
        bridge_legacy_sqlite_database(database_path, backup_path)

    assert database_path.read_bytes() == before
    assert _revision(database_path) is None


def test_bridge_rejects_legacy_shape_with_missing_index_before_stamping(tmp_path) -> None:
    database_path = tmp_path / "missing-index.sqlite"
    backup_path = tmp_path / "missing-index-backup.sqlite"
    _create_legacy_start_state(database_path)
    connection = sqlite3.connect(database_path)
    try:
        connection.execute("DROP INDEX ix_subscriptions_id")
        connection.commit()
    finally:
        connection.close()
    _copy_sqlite(database_path, backup_path)
    before = database_path.read_bytes()

    with pytest.raises(AlembicBridgeError):
        bridge_legacy_sqlite_database(database_path, backup_path)

    assert database_path.read_bytes() == before
    assert _revision(database_path) is None


def test_bridge_rejects_backup_with_different_row_counts_before_stamping(tmp_path) -> None:
    database_path = tmp_path / "source.sqlite"
    backup_path = tmp_path / "outdated-backup.sqlite"
    _create_legacy_start_state(database_path)
    _copy_sqlite(database_path, backup_path)
    connection = sqlite3.connect(database_path)
    try:
        connection.execute(
            "INSERT INTO config_snapshots (id, snapshot_data) VALUES (1, '{}')"
        )
        connection.commit()
    finally:
        connection.close()
    before = database_path.read_bytes()

    with pytest.raises(AlembicBridgeError):
        bridge_legacy_sqlite_database(database_path, backup_path)

    assert database_path.read_bytes() == before
    assert _revision(database_path) is None
