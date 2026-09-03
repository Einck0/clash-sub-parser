from __future__ import annotations

import json
from pathlib import Path

from alembic.config import Config
from alembic.script import ScriptDirectory

from app.database import Base
from app.models import (  # noqa: F401
    config_snapshot,
    dns,
    generate_config,
    node_group,
    node_probe_result,
    probe_config,
    proxy_chain,
    rule,
    rule_category,
    security_settings,
    subscription,
)


MAP_PATH = Path(__file__).parent / "fixtures" / "legacy-schema-revision-map.json"
BACKEND_ROOT = Path(__file__).parents[1]
BRIDGE_REVISION_PATH = (
    BACKEND_ROOT
    / "alembic"
    / "versions"
    / "csp_legacy_bootstrap_bridge.py"
)


def _alembic_script_directory() -> ScriptDirectory:
    config = Config(str(BACKEND_ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    return ScriptDirectory.from_config(config)


def _alembic_revision_ids() -> set[str]:
    """从 Alembic ScriptDirectory 动态解析所有 revision 标识"""
    script = _alembic_script_directory()
    return {rev.revision for rev in script.walk_revisions()}


def _metadata_columns() -> dict[str, list[str]]:
    mapping = json.loads(MAP_PATH.read_text(encoding="utf-8"))
    legacy_tables = set(mapping["metadata_create_all"]["tables"].keys())
    return {
        table.name: [column.name for column in table.columns]
        for table in sorted(Base.metadata.tables.values(), key=lambda table: table.name)
        if table.name in legacy_tables
    }


def _bootstrap_columns() -> dict[str, list[str]]:
    mapping = json.loads(MAP_PATH.read_text(encoding="utf-8"))
    return {
        table_name: sorted(cols)
        for table_name, cols in mapping["bootstrap_schema"]["columns"].items()
    }


def test_legacy_schema_revision_map_covers_metadata_and_runtime_ddl() -> None:
    mapping = json.loads(MAP_PATH.read_text(encoding="utf-8"))

    assert mapping["metadata_create_all"]["revision"] == "csp_legacy_schema_baseline"
    assert mapping["metadata_create_all"]["tables"] == _metadata_columns()
    assert mapping["bootstrap_schema"]["revision"] == "csp_legacy_bootstrap_bridge"
    assert mapping["bootstrap_schema"]["columns"] == _bootstrap_columns()
    assert mapping["bootstrap_schema"]["backfills"] == ["rule_categories"]


def test_revision_ids_exist_in_alembic_script_directory() -> None:
    """map 中引用的 revision 必须能在 Alembic 链中找到"""
    mapping = json.loads(MAP_PATH.read_text(encoding="utf-8"))
    real_revisions = _alembic_revision_ids()

    referenced = {
        mapping["metadata_create_all"]["revision"],
        mapping["bootstrap_schema"]["revision"],
    }
    missing = referenced - real_revisions
    assert not missing, f"revision map 引用了不存在于 Alembic 链的 revision: {missing}"


def test_alembic_chain_contains_bridge_revision() -> None:
    """Alembic 链中必须包含 legacy bridge revision"""
    real_revisions = _alembic_revision_ids()
    assert "csp_legacy_bootstrap_bridge" in real_revisions
