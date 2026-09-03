from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
import sqlite3


class BackupArtifactError(RuntimeError):
    pass


@dataclass(frozen=True)
class BackupManifest:
    artifact_type: str
    byte_size: int
    integrity_check: str
    tables: dict[str, int]

    def to_dict(self) -> dict:
        return {
            "artifact_type": self.artifact_type,
            "byte_size": self.byte_size,
            "integrity_check": self.integrity_check,
            "tables": self.tables,
        }


def inspect_sqlite_backup(artifact_path: Path | str) -> BackupManifest:
    artifact = Path(artifact_path)
    if not artifact.exists():
        raise BackupArtifactError("Backup artifact does not exist")
    if not artifact.is_file():
        raise BackupArtifactError("Backup artifact is not a file")
    if artifact.stat().st_size == 0:
        raise BackupArtifactError("Backup artifact is empty")

    source = open_sqlite_backup_readonly(artifact)
    try:
        integrity_check = _read_integrity_check(source)
        if integrity_check != "ok":
            raise BackupArtifactError("Backup artifact failed SQLite integrity check")
        tables = _read_table_counts(source)
        if not tables:
            raise BackupArtifactError("Backup artifact has no recoverable tables")
    except sqlite3.Error as exc:
        raise BackupArtifactError("Backup artifact cannot be inspected") from exc
    finally:
        source.close()

    return BackupManifest(
        artifact_type="sqlite",
        byte_size=artifact.stat().st_size,
        integrity_check=integrity_check,
        tables=tables,
    )


def restore_and_verify_sqlite_backup(
    artifact_path: Path | str,
    restore_path: Path | str,
) -> tuple[BackupManifest, BackupManifest]:
    source_manifest = inspect_sqlite_backup(artifact_path)
    artifact = Path(artifact_path)
    restored = Path(restore_path)
    if restored.exists():
        raise BackupArtifactError("Restore destination already exists")
    if not restored.parent.is_dir():
        raise BackupArtifactError("Restore destination parent does not exist")

    source = open_sqlite_backup_readonly(artifact)
    destination = sqlite3.connect(restored)
    try:
        source.backup(destination)
    except sqlite3.Error as exc:
        raise BackupArtifactError("Backup artifact could not be restored") from exc
    finally:
        destination.close()
        source.close()

    restored_manifest = inspect_sqlite_backup(restored)
    if source_manifest.tables != restored_manifest.tables:
        raise BackupArtifactError("Restored artifact row counts do not match source")
    return source_manifest, restored_manifest


def open_sqlite_backup_readonly(artifact: Path) -> sqlite3.Connection:
    try:
        return sqlite3.connect(f"{artifact.resolve().as_uri()}?mode=ro", uri=True)
    except sqlite3.Error as exc:
        raise BackupArtifactError("Backup artifact cannot be opened read-only") from exc


def _read_integrity_check(connection: sqlite3.Connection) -> str:
    result = connection.execute("PRAGMA integrity_check").fetchone()
    if result is None:
        raise BackupArtifactError("Backup artifact returned no SQLite integrity result")
    return str(result[0]).lower()


def _read_table_counts(connection: sqlite3.Connection) -> dict[str, int]:
    rows = connection.execute(
        "SELECT name FROM sqlite_master "
        "WHERE type = 'table' AND name NOT LIKE 'sqlite_%' "
        "ORDER BY name"
    ).fetchall()
    counts: dict[str, int] = {}
    for (name,) in rows:
        quoted_name = str(name).replace('"', '""')
        counts[str(name)] = int(
            connection.execute(f'SELECT COUNT(*) FROM "{quoted_name}"').fetchone()[0]
        )
    return counts
