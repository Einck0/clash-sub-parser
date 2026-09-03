from __future__ import annotations

import json
from pathlib import Path
import sqlite3
import subprocess
import sys

from app.services.migration_preflight import preflight_sqlite_backup


def _create_legacy_database(path, *, include_unknown_table: bool = False) -> None:
    connection = sqlite3.connect(path)
    try:
        connection.execute(
            "CREATE TABLE subscriptions (id INTEGER PRIMARY KEY, raw_nodes JSON NOT NULL)"
        )
        connection.execute(
            "CREATE TABLE node_groups (id INTEGER PRIMARY KEY, include_entries JSON NOT NULL)"
        )
        connection.execute(
            "INSERT INTO subscriptions (raw_nodes) VALUES ('[{\"name\": \"fixture\", \"type\": \"ss\"}]')"
        )
        connection.execute(
            "INSERT INTO node_groups (include_entries) VALUES ('[{\"type\": \"group\", \"value\": 99}]')"
        )
        if include_unknown_table:
            connection.execute("CREATE TABLE unrecognized_legacy_table (id INTEGER PRIMARY KEY)")
        connection.commit()
    finally:
        connection.close()


def test_preflight_is_read_only_and_reports_reference_blockers(tmp_path) -> None:
    artifact_path = tmp_path / "legacy.sqlite"
    _create_legacy_database(artifact_path)
    before = artifact_path.read_bytes()

    report = preflight_sqlite_backup(artifact_path)

    assert artifact_path.read_bytes() == before
    assert report["blocking"] is True
    assert report["artifact"]["tables"] == {"node_groups": 1, "subscriptions": 1}
    assert report["references"]["missing_group_reference_count"] == 1
    assert str(artifact_path) not in json.dumps(report)


def test_preflight_reports_unknown_schema_as_blocking_and_returns_nonzero(tmp_path) -> None:
    artifact_path = tmp_path / "unknown.sqlite"
    _create_legacy_database(artifact_path, include_unknown_table=True)

    report = preflight_sqlite_backup(artifact_path)
    result = subprocess.run(
        [
            sys.executable,
            "-m",
            "app.services.migration_preflight",
            "--database",
            str(artifact_path),
        ],
        capture_output=True,
        check=False,
        text=True,
        cwd=str(Path(__file__).parents[1]),
    )

    assert report["blocking"] is True
    assert {item["code"] for item in report["diagnostics"]} == {"unknown_tables", "missing_group_references"}
    assert result.returncode == 1
    assert json.loads(result.stdout)["blocking"] is True
    assert str(artifact_path) not in result.stdout


def test_preflight_reports_unmappable_node_shape_as_blocking(tmp_path) -> None:
    artifact_path = tmp_path / "unmappable.sqlite"
    _create_legacy_database(artifact_path)
    connection = sqlite3.connect(artifact_path)
    try:
        connection.execute("UPDATE subscriptions SET raw_nodes = '[{}]'")
        connection.commit()
    finally:
        connection.close()

    report = preflight_sqlite_backup(artifact_path)

    assert report["blocking"] is True
    assert {item["code"] for item in report["diagnostics"]} == {"missing_group_references", "unmappable_json_payloads"}
