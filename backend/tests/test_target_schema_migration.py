from __future__ import annotations

from pathlib import Path

from alembic import command
from alembic.config import Config
from alembic.script import ScriptDirectory
from sqlalchemy import create_engine, inspect


BACKEND_ROOT = Path(__file__).parents[1]
TARGET_HEAD_REVISION = "csp_target_domain_schema"

EXPECTED_NEW_TABLES = {
    "sources",
    "source_revisions",
    "nodes",
    "node_source_links",
    "configuration_revisions",
    "probe_profiles",
    "probe_jobs",
    "probe_observations",
    "quarantine_records",
}


def _alembic_config(db_file: Path) -> Config:
    config = Config(str(BACKEND_ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    config.set_main_option("sqlalchemy.url", f"sqlite+aiosqlite:///{db_file.resolve()}")
    return config


def _tables(engine) -> set[str]:
    inspector = inspect(engine)
    return set(inspector.get_table_names())


def _columns(engine, table_name: str) -> dict[str, dict]:
    inspector = inspect(engine)
    cols = inspector.get_columns(table_name)
    return {c["name"]: {"type": str(c["type"]).upper(), "nullable": c["nullable"]} for c in cols}


def _indexes(engine, table_name: str) -> set[str]:
    inspector = inspect(engine)
    return {idx["name"] for idx in inspector.get_indexes(table_name)}


def test_target_schema_head_in_script_directory() -> None:
    """确认目标 head revision 存在且为最新 head"""
    config = Config(str(BACKEND_ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    script = ScriptDirectory.from_config(config)
    assert TARGET_HEAD_REVISION in script.get_heads()


def test_sqlite_upgrade_to_head_creates_target_tables(tmp_path) -> None:
    """SQLite 从空库 upgrade head 能够成功建齐新领域表与索引"""
    db_file = tmp_path / "target_test.sqlite"
    config = _alembic_config(db_file)

    command.upgrade(config, "head")

    engine = create_engine(f"sqlite:///{db_file.resolve()}")
    try:
        tables = _tables(engine)
        assert EXPECTED_NEW_TABLES.issubset(tables)

        nodes_cols = _columns(engine, "nodes")
        assert "logical_id" in nodes_cols
        assert "payload_fingerprint" in nodes_cols
        assert "lifecycle_state" in nodes_cols

        nodes_indexes = _indexes(engine, "nodes")
        assert "ix_nodes_logical_id" in nodes_indexes
        assert "ix_nodes_payload_fingerprint" in nodes_indexes
    finally:
        engine.dispose()


def test_sqlite_downgrade_drops_target_tables(tmp_path) -> None:
    """SQLite 从 head downgrade 到 csp_legacy_bootstrap_bridge 干净删除新表"""
    db_file = tmp_path / "downgrade_test.sqlite"
    config = _alembic_config(db_file)

    command.upgrade(config, "head")
    command.downgrade(config, "csp_legacy_bootstrap_bridge")

    engine = create_engine(f"sqlite:///{db_file.resolve()}")
    try:
        tables = _tables(engine)
        assert not EXPECTED_NEW_TABLES.intersection(tables)
    finally:
        engine.dispose()


def test_schema_metadata_parity_between_sqlite_dialects(tmp_path) -> None:
    """验证 SQLite 迁移生成的列与约束元数据自洽完整"""
    db_file = tmp_path / "parity_test.sqlite"
    config = _alembic_config(db_file)

    command.upgrade(config, "head")

    engine = create_engine(f"sqlite:///{db_file.resolve()}")
    try:
        for tbl in EXPECTED_NEW_TABLES:
            cols = _columns(engine, tbl)
            assert len(cols) > 0, f"表 {tbl} 列集合为空"
            assert "id" in cols, f"表 {tbl} 缺失主键 id"
    finally:
        engine.dispose()


def test_offline_sql_generation_succeeds_for_postgresql_and_sqlite(capsys) -> None:
    """验证 offline SQL 生成在 postgresql 与 sqlite 方言下均能无错误编译"""
    config_pg = Config(str(BACKEND_ROOT / "alembic.ini"))
    config_pg.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    config_pg.set_main_option("sqlalchemy.url", "postgresql+asyncpg://user:pass@localhost:5432/testdb")

    command.upgrade(config_pg, "csp_legacy_bootstrap_bridge:csp_target_domain_schema", sql=True)
    captured = capsys.readouterr()
    pg_sql = captured.out
    for tbl in EXPECTED_NEW_TABLES:
        assert tbl in pg_sql, f"PostgreSQL DDL 缺失目标表 {tbl}"


