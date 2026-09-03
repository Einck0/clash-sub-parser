from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest


FIXTURE_PATH = Path(__file__).parent / "fixtures" / "legacy-schema-fingerprint-v1.json"


@pytest.fixture(scope="module")
def fingerprint_data() -> dict:
    return json.loads(FIXTURE_PATH.read_text(encoding="utf-8"))


def test_fingerprint_is_sha256_of_canonical_manifest(fingerprint_data) -> None:
    """指纹等于 manifest 的确定性 JSON 序列化的 SHA-256"""
    manifest = fingerprint_data["manifest"]
    canonical = json.dumps(manifest, ensure_ascii=False, sort_keys=True, indent=None)
    expected = hashlib.sha256(canonical.encode()).hexdigest()
    assert fingerprint_data["fingerprint"] == expected


def test_manifest_tables_are_sorted_by_column_name(fingerprint_data) -> None:
    """每张表的列按 name 升序排列，保证指纹稳定"""
    for table_name, columns in fingerprint_data["manifest"]["tables"].items():
        names = [c["name"] for c in columns]
        assert names == sorted(names), f"{table_name} 的列未按名称排序"


def test_manifest_indexes_are_sorted_by_name(fingerprint_data) -> None:
    """索引按 name 升序排列"""
    names = [idx["name"] for idx in fingerprint_data["manifest"]["indexes"]]
    assert names == sorted(names)


def test_manifest_covers_all_eleven_legacy_tables(fingerprint_data) -> None:
    """manifest 覆盖全部 11 张遗留业务表"""
    expected_tables = {
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
    assert set(fingerprint_data["manifest"]["tables"]) == expected_tables


def test_each_column_has_required_metadata_fields(fingerprint_data) -> None:
    """每列元数据包含 name, type, notnull, default, pk 五个字段"""
    required = {"name", "type", "notnull", "default", "pk"}
    for table_name, columns in fingerprint_data["manifest"]["tables"].items():
        for col in columns:
            assert set(col) == required, f"{table_name}.{col.get('name', '?')} 字段缺失"


def test_each_index_has_required_metadata_fields(fingerprint_data) -> None:
    """每条索引记录包含 name, table, sql 三个字段"""
    required = {"name", "table", "sql"}
    for idx in fingerprint_data["manifest"]["indexes"]:
        assert set(idx) == required, f"索引 {idx.get('name', '?')} 字段缺失"
