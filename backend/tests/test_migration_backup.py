from __future__ import annotations

import sqlite3

import pytest

from app.services.migration_backup import (
    BackupArtifactError,
    inspect_sqlite_backup,
    restore_and_verify_sqlite_backup,
)


def _create_sqlite_backup(path) -> None:
    connection = sqlite3.connect(path)
    try:
        connection.execute("CREATE TABLE subscriptions (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
        connection.execute("CREATE TABLE node_groups (id INTEGER PRIMARY KEY, name TEXT NOT NULL)")
        connection.execute("INSERT INTO subscriptions (name) VALUES ('fixture-subscription')")
        connection.execute("INSERT INTO node_groups (name) VALUES ('fixture-group')")
        connection.commit()
    finally:
        connection.close()


def test_backup_manifest_is_redacted_and_counts_recoverable_rows(tmp_path) -> None:
    backup_path = tmp_path / "legacy-backup.sqlite"
    _create_sqlite_backup(backup_path)

    manifest = inspect_sqlite_backup(backup_path).to_dict()

    assert manifest == {
        "artifact_type": "sqlite",
        "byte_size": backup_path.stat().st_size,
        "integrity_check": "ok",
        "tables": {"node_groups": 1, "subscriptions": 1},
    }
    assert str(backup_path) not in str(manifest)


def test_restore_verification_copies_and_rechecks_artifact(tmp_path) -> None:
    backup_path = tmp_path / "legacy-backup.sqlite"
    restored_path = tmp_path / "restored.sqlite"
    _create_sqlite_backup(backup_path)

    source, restored = restore_and_verify_sqlite_backup(backup_path, restored_path)

    assert source.tables == restored.tables
    assert restored_path.is_file()
    assert restored.integrity_check == "ok"


def test_backup_manifest_refuses_missing_source_artifact(tmp_path) -> None:
    with pytest.raises(BackupArtifactError, match="does not exist"):
        inspect_sqlite_backup(tmp_path / "missing.sqlite")


def test_backup_manifest_refuses_unrecoverable_source_artifact(tmp_path) -> None:
    corrupt_path = tmp_path / "corrupt.sqlite"
    corrupt_path.write_bytes(b"not-a-sqlite-backup")

    with pytest.raises(BackupArtifactError, match="cannot be inspected"):
        inspect_sqlite_backup(corrupt_path)


def test_restore_verification_refuses_existing_destination(tmp_path) -> None:
    backup_path = tmp_path / "legacy-backup.sqlite"
    restored_path = tmp_path / "restored.sqlite"
    _create_sqlite_backup(backup_path)
    restored_path.write_bytes(b"do-not-overwrite")

    with pytest.raises(BackupArtifactError, match="already exists"):
        restore_and_verify_sqlite_backup(backup_path, restored_path)
