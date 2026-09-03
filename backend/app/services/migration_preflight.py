from __future__ import annotations

import argparse
import json
from pathlib import Path
import sqlite3

from app.services.migration_backup import (
    BackupArtifactError,
    inspect_sqlite_backup,
    open_sqlite_backup_readonly,
)


KNOWN_TABLES = {
    "config_snapshots",
    "dns_config",
    "generate_config",
    "node_groups",
    "node_probe_results",
    "probe_config",
    "proxy_chain_bindings",
    "rule_categories",
    "rules",
    "security_settings",
    "subscriptions",
}
SYSTEM_TABLES = {"alembic_version"}
GROUP_REFERENCE_FIELDS = {
    "include_group_ids",
    "include_group_nodes_ids",
    "exclude_group_ids",
}
GROUP_ENTRY_TYPES = {"group", "group_nodes", "exclude_group_nodes"}


def preflight_sqlite_backup(artifact_path: Path | str) -> dict:
    artifact = Path(artifact_path)
    manifest = inspect_sqlite_backup(artifact)
    source = open_sqlite_backup_readonly(artifact)
    try:
        schema = _read_schema(source)
        references, malformed_payload_count = _read_reference_summary(source, schema)
        malformed_payload_count += _read_node_shape_errors(source, schema)
    except sqlite3.Error as exc:
        raise BackupArtifactError("Backup artifact preflight could not inspect schema") from exc
    finally:
        source.close()

    unknown_tables = sorted(set(schema) - KNOWN_TABLES - SYSTEM_TABLES)
    diagnostics: list[dict[str, int | str]] = []
    if unknown_tables:
        diagnostics.append({"code": "unknown_tables", "count": len(unknown_tables)})
    if references["missing_group_reference_count"]:
        diagnostics.append(
            {
                "code": "missing_group_references",
                "count": references["missing_group_reference_count"],
            }
        )
    if malformed_payload_count:
        diagnostics.append(
            {"code": "unmappable_json_payloads", "count": malformed_payload_count}
        )

    return {
        "artifact": manifest.to_dict(),
        "schema": {"tables": schema},
        "references": references,
        "diagnostics": diagnostics,
        "blocking": bool(diagnostics),
    }


def _read_schema(connection: sqlite3.Connection) -> dict[str, list[str]]:
    table_rows = connection.execute(
        "SELECT name FROM sqlite_master "
        "WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name"
    ).fetchall()
    schema: dict[str, list[str]] = {}
    for (name,) in table_rows:
        table_name = str(name)
        quoted_name = table_name.replace('"', '""')
        columns = connection.execute(f'PRAGMA table_info("{quoted_name}")').fetchall()
        schema[table_name] = sorted(str(column[1]) for column in columns)
    return schema


def _read_reference_summary(
    connection: sqlite3.Connection,
    schema: dict[str, list[str]],
) -> tuple[dict[str, int], int]:
    if "node_groups" not in schema or "id" not in schema["node_groups"]:
        return {"group_reference_count": 0, "missing_group_reference_count": 0}, 0

    group_ids = {
        int(row[0])
        for row in connection.execute("SELECT id FROM node_groups").fetchall()
    }
    references: list[int] = []
    malformed = 0
    group_columns = set(schema["node_groups"])
    for field in GROUP_REFERENCE_FIELDS & group_columns:
        values = connection.execute(f"SELECT {field} FROM node_groups").fetchall()
        for (raw_value,) in values:
            parsed, valid = _json_list(raw_value)
            if not valid:
                malformed += 1
                continue
            for value in parsed:
                try:
                    references.append(int(value))
                except (TypeError, ValueError):
                    malformed += 1

    if "include_entries" in group_columns:
        values = connection.execute("SELECT include_entries FROM node_groups").fetchall()
        for (raw_value,) in values:
            parsed, valid = _json_list(raw_value)
            if not valid:
                malformed += 1
                continue
            for entry in parsed:
                if not isinstance(entry, dict):
                    malformed += 1
                    continue
                if entry.get("type") not in GROUP_ENTRY_TYPES:
                    continue
                entry_value = entry.get("value")
                if entry_value is None:
                    malformed += 1
                    continue
                try:
                    references.append(int(entry_value))
                except (TypeError, ValueError):
                    malformed += 1

    missing = sum(reference not in group_ids for reference in references)
    return {
        "group_reference_count": len(references),
        "missing_group_reference_count": missing,
    }, malformed


def _read_node_shape_errors(
    connection: sqlite3.Connection,
    schema: dict[str, list[str]],
) -> int:
    if "subscriptions" not in schema or "raw_nodes" not in schema["subscriptions"]:
        return 0

    malformed = 0
    values = connection.execute("SELECT raw_nodes FROM subscriptions").fetchall()
    for (raw_value,) in values:
        parsed, valid = _json_list(raw_value)
        if not valid:
            malformed += 1
            continue
        for node in parsed:
            if not isinstance(node, dict):
                malformed += 1
                continue
            name = node.get("name")
            protocol_type = node.get("type")
            if not isinstance(name, str) or not name.strip():
                malformed += 1
                continue
            if not isinstance(protocol_type, str) or not protocol_type.strip():
                malformed += 1

    return malformed


def _json_list(raw_value) -> tuple[list, bool]:
    if raw_value is None:
        return [], True
    try:
        parsed = json.loads(raw_value) if isinstance(raw_value, str) else raw_value
    except (TypeError, json.JSONDecodeError):
        return [], False
    return (parsed, True) if isinstance(parsed, list) else ([], False)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--database", required=True)
    args = parser.parse_args()
    try:
        report = preflight_sqlite_backup(args.database)
    except BackupArtifactError as exc:
        parser.exit(2, f"preflight failed: {exc}\n")
    print(json.dumps(report, ensure_ascii=False, sort_keys=True))
    return 1 if report["blocking"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
