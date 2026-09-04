"""Run a read-only SQLite migration rehearsal and write an audit manifest.

The safeguarded source database is opened with SQLite's ``mode=ro`` URI. The
rehearsal copies it to an independently writable target using SQLite's backup
API, so the source volume is never a migration target. The audit compares
schemas and all source table row counts, verifies relationship integrity, and
confirms that the canonical compiler renders every supported target.
"""
from __future__ import annotations

import argparse
import asyncio
from contextlib import closing
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import sqlite3
import sys
from typing import Any

REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
BACKEND_ROOT = REPOSITORY_ROOT / "backend"
if str(BACKEND_ROOT) not in sys.path:
    sys.path.insert(0, str(BACKEND_ROOT))

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine  # noqa: E402

from app.services.canonical_compiler import CanonicalGraphResolver  # noqa: E402
from app.services.compiler_target_adapters import render_compiled_target  # noqa: E402
from app.services.legacy_config_importer import build_logical_bundle_from_db  # noqa: E402

DEFAULT_SOURCE = (
    REPOSITORY_ROOT
    / "data-safe-snapshots"
    / "clash_sub_parser_prod_safeguard_20260904_004403.db"
)
DEFAULT_TARGET = REPOSITORY_ROOT / "tmp" / "full-migration-rehearsal.db"
DEFAULT_MANIFEST = REPOSITORY_ROOT / "tmp" / "full-migration-audit-manifest.json"

REQUIRED_MINIMUMS = {
    "sources": 4,
    "subscriptions": 8,
    "nodes": 2080,
    "node_source_links": 5609,
    "node_groups": 29,
    "rules": 470,
    "node_probe_results": 362,
    "config_snapshots": 50,
}
SUPPORTED_TARGETS = ("clash", "mihomo", "stash", "shadowrocket", "sing-box")


class MigrationAuditError(RuntimeError):
    """Raised when the rehearsal cannot prove data continuity."""


@dataclass(frozen=True)
class DatabaseSnapshot:
    """Immutable source or target database audit facts."""

    sha256: str
    sqlite_version: str
    tables: dict[str, int]
    schema: dict[str, list[str]]


def sha256_file(path: Path) -> str:
    """Return the SHA-256 digest for a database artifact."""
    digest = hashlib.sha256()
    with path.open("rb") as artifact:
        for chunk in iter(lambda: artifact.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def open_readonly(path: Path) -> sqlite3.Connection:
    """Open a SQLite database with SQLite-enforced read-only semantics."""
    return sqlite3.connect(f"file:{path.resolve()}?mode=ro", uri=True)


def quote_identifier(identifier: str) -> str:
    """Quote a SQLite identifier after reading it from SQLite metadata."""
    escaped_identifier = identifier.replace('"', '""')
    return f'"{escaped_identifier}"'


def inspect_database(path: Path, *, readonly: bool) -> DatabaseSnapshot:
    """Collect table row counts and column names without altering the database."""
    connection = open_readonly(path) if readonly else sqlite3.connect(path)
    try:
        table_names = [
            str(row[0])
            for row in connection.execute(
                "SELECT name FROM sqlite_master "
                "WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
            )
        ]
        tables = {
            name: int(connection.execute(f"SELECT COUNT(*) FROM {quote_identifier(name)}").fetchone()[0])
            for name in table_names
        }
        schema = {
            name: [
                str(column[1])
                for column in connection.execute(f"PRAGMA table_info({quote_identifier(name)})")
            ]
            for name in table_names
        }
        return DatabaseSnapshot(
            sha256=sha256_file(path),
            sqlite_version=str(connection.execute("SELECT sqlite_version()").fetchone()[0]),
            tables=tables,
            schema=schema,
        )
    finally:
        connection.close()


def copy_database(source_path: Path, target_path: Path) -> None:
    """Copy a read-only source through SQLite's backup API to a fresh target."""
    target_path.parent.mkdir(parents=True, exist_ok=True)
    target_path.unlink(missing_ok=True)
    with closing(open_readonly(source_path)) as source, closing(sqlite3.connect(target_path)) as target:
        try:
            source.backup(target)
            target.commit()
        except sqlite3.Error as exc:
            target.rollback()
            target_path.unlink(missing_ok=True)
            raise MigrationAuditError("SQLite backup rehearsal failed and was rolled back") from exc


def validate_table_continuity(source: DatabaseSnapshot, target: DatabaseSnapshot) -> None:
    """Require exact schema and row-count preservation for every source table."""
    if source.schema != target.schema:
        raise MigrationAuditError("Target schema differs from the safeguarded source")
    if source.tables != target.tables:
        raise MigrationAuditError("Target row counts differ from the safeguarded source")
    for table_name, required_minimum in REQUIRED_MINIMUMS.items():
        actual = target.tables.get(table_name, 0)
        if actual < required_minimum:
            raise MigrationAuditError(
                f"{table_name} has {actual} rows, below required minimum {required_minimum}"
            )


def validate_id_mappings(source_path: Path, target_path: Path) -> dict[str, int]:
    """Prove that every source table with an ID preserved its primary-key set."""
    with closing(open_readonly(source_path)) as source, closing(sqlite3.connect(target_path)) as target:
        table_names = [
            str(row[0])
            for row in source.execute(
                "SELECT name FROM sqlite_master "
                "WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
            )
        ]
        mappings: dict[str, int] = {}
        for table_name in table_names:
            columns = {
                str(row[1])
                for row in source.execute(f"PRAGMA table_info({quote_identifier(table_name)})")
            }
            if "id" not in columns:
                continue
            query = f"SELECT id FROM {quote_identifier(table_name)} ORDER BY id"
            source_ids = [row[0] for row in source.execute(query)]
            target_ids = [row[0] for row in target.execute(query)]
            if source_ids != target_ids:
                raise MigrationAuditError(f"Primary-key mapping differs for {table_name}")
            mappings[table_name] = len(source_ids)
    return mappings


def validate_relationships(target_path: Path) -> dict[str, int]:
    """Verify declared SQLite foreign keys and logical source-link relationships."""
    with closing(sqlite3.connect(target_path)) as connection:
        connection.execute("PRAGMA foreign_keys = ON")
        foreign_key_errors = connection.execute("PRAGMA foreign_key_check").fetchall()
        if foreign_key_errors:
            raise MigrationAuditError("SQLite foreign key check reported broken relationships")
        checks = {
            "node_source_links_missing_nodes": """
                SELECT COUNT(*) FROM node_source_links links
                LEFT JOIN nodes ON nodes.id = links.node_id
                WHERE nodes.id IS NULL
            """,
            "node_source_links_missing_revisions": """
                SELECT COUNT(*) FROM node_source_links links
                LEFT JOIN source_revisions revisions ON revisions.id = links.source_revision_id
                WHERE revisions.id IS NULL
            """,
            "source_revisions_missing_sources": """
                SELECT COUNT(*) FROM source_revisions revisions
                LEFT JOIN sources ON sources.id = revisions.source_id
                WHERE sources.id IS NULL
            """,
        }
        results = {name: int(connection.execute(query).fetchone()[0]) for name, query in checks.items()}
    if any(results.values()):
        raise MigrationAuditError("Logical foreign-key audit reported orphaned records")
    return results


def load_full_inventory(target_path: Path) -> list[dict[str, Any]]:
    """Load every normalized node payload for full-inventory compiler verification."""
    with closing(sqlite3.connect(target_path)) as connection:
        connection.row_factory = sqlite3.Row
        rows = connection.execute(
            "SELECT name, protocol, server, port, normalized_payload FROM nodes ORDER BY id"
        ).fetchall()
    inventory: list[dict[str, Any]] = []
    for row in rows:
        raw_payload = row["normalized_payload"]
        payload = json.loads(raw_payload) if isinstance(raw_payload, str) else raw_payload
        if not isinstance(payload, dict):
            raise MigrationAuditError("A normalized node payload is not a JSON object")
        node = dict(payload)
        node.update(
            {
                "name": row["name"],
                "type": row["protocol"],
                "server": row["server"],
                "port": row["port"],
            }
        )
        inventory.append(node)
    return inventory


async def validate_compiler(target_path: Path) -> dict[str, int]:
    """Compile the copied data to all supported client target formats."""
    inventory = load_full_inventory(target_path)
    engine = create_async_engine(f"sqlite+aiosqlite:///{target_path.resolve()}?mode=ro")
    session_factory = async_sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)
    try:
        async with session_factory() as session:
            bundle = await build_logical_bundle_from_db(session, bundle_name="migration-audit")
            result = CanonicalGraphResolver(bundle=bundle, inventory_nodes=inventory).compile()
            outputs = {
                target: len(render_compiled_target(target, result, bundle, inventory))
                for target in SUPPORTED_TARGETS
            }
    finally:
        await engine.dispose()
    if any(length <= 0 for length in outputs.values()):
        raise MigrationAuditError("A supported compiler target rendered an empty configuration")
    return outputs


def copy_manifest(
    source: DatabaseSnapshot,
    target: DatabaseSnapshot,
    relationships: dict[str, int],
    id_mappings: dict[str, int],
    compiler_outputs: dict[str, int],
    source_hash_unchanged: bool,
) -> dict[str, Any]:
    """Build a JSON-serializable manifest without recording sensitive payloads."""
    return {
        "completed_at": datetime.now(timezone.utc).isoformat(),
        "source": asdict(source),
        "target": asdict(target),
        "source_hash_unchanged": source_hash_unchanged,
        "relationship_orphans": relationships,
        "identity_id_mappings": id_mappings,
        "compiler_output_bytes": compiler_outputs,
        "required_minimums": REQUIRED_MINIMUMS,
        "result": "passed",
    }


def run_audit(source_path: Path, target_path: Path, manifest_path: Path) -> dict[str, Any]:
    """Run the full read-only rehearsal and atomically save its audit manifest."""
    if not source_path.is_file():
        raise MigrationAuditError("The safeguarded SQLite source artifact does not exist")

    source = inspect_database(source_path, readonly=True)
    copy_database(source_path, target_path)
    target = inspect_database(target_path, readonly=False)
    validate_table_continuity(source, target)
    id_mappings = validate_id_mappings(source_path, target_path)
    relationships = validate_relationships(target_path)
    compiler_outputs = asyncio.run(validate_compiler(target_path))

    source_hash_unchanged = source.sha256 == sha256_file(source_path)
    manifest = copy_manifest(
        source,
        target,
        relationships,
        id_mappings,
        compiler_outputs,
        source_hash_unchanged,
    )
    if not source_hash_unchanged:
        raise MigrationAuditError("The read-only source hash changed during the rehearsal")

    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    temp_manifest = manifest_path.with_suffix(f"{manifest_path.suffix}.tmp")
    temp_manifest.write_text(json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True), encoding="utf-8")
    temp_manifest.replace(manifest_path)
    return manifest


def parse_args() -> argparse.Namespace:
    """Parse paths while retaining repository-relative safe defaults."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", type=Path, default=DEFAULT_SOURCE)
    parser.add_argument("--target", type=Path, default=DEFAULT_TARGET)
    parser.add_argument("--manifest", type=Path, default=DEFAULT_MANIFEST)
    return parser.parse_args()


def main() -> int:
    """Execute the rehearsal and emit a non-sensitive summary."""
    args = parse_args()
    try:
        manifest = run_audit(args.source, args.target, args.manifest)
    except MigrationAuditError as exc:
        print(f"migration audit failed: {exc}", file=sys.stderr)
        return 1
    print("migration audit passed")
    print(json.dumps({"tables": manifest["target"]["tables"], "compiler_output_bytes": manifest["compiler_output_bytes"]}, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
