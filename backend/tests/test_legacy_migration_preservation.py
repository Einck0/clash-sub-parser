from __future__ import annotations

import json
from pathlib import Path
import sqlite3

from alembic import command
from alembic.config import Config

from app.services.alembic_bridge import (
    LEGACY_BRIDGE_HEAD,
    bridge_legacy_sqlite_database,
)


BACKEND_ROOT = Path(__file__).parents[1]
SQL_FIXTURE_PATH = Path(__file__).parent / "fixtures" / "legacy-alembic-start-state-v1.sql"
SYSTEM_FIXTURE_PATH = Path(__file__).parent / "fixtures" / "legacy-system-v1.json"


def _alembic_config(db_file: Path) -> Config:
    config = Config(str(BACKEND_ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    config.set_main_option("sqlalchemy.url", f"sqlite+aiosqlite:///{db_file.resolve()}")
    return config


def _init_populated_legacy_db(db_path: Path) -> dict[str, int]:
    conn = sqlite3.connect(db_path)
    try:
        conn.executescript(SQL_FIXTURE_PATH.read_text(encoding="utf-8"))
        conn.commit()

        # 加载遗留测试数据并统计每张表的行数
        data = json.loads(SYSTEM_FIXTURE_PATH.read_text(encoding="utf-8"))
        counts: dict[str, int] = {}
        for table_name, rows in data["tables"].items():
            if not rows:
                counts[table_name] = 0
                continue
            
            # 补齐默认字段
            norm_rows = []
            for row in rows:
                r = dict(row)
                if table_name == "subscriptions":
                    r.setdefault("is_primary", True)
                    r.setdefault("enabled", True)
                    r.setdefault("filter_regex", "[]")
                    r.setdefault("filter_media_unlock", "[]")
                    r.setdefault("include_node_names", "[]")
                    r.setdefault("exclude_node_names", "[]")
                    r.setdefault("node_renames", "{}")
                    r.setdefault("proxy_chain", "[]")
                    r.setdefault("node_proxy_chains", "{}")
                    r.setdefault("source_nodes", "[]")
                    r.setdefault("manual_nodes", "[]")
                    r.setdefault("raw_nodes", "[]")
                    r.setdefault("fetch_failed_count", 0)
                    r.setdefault("fetch_comments", "[]")
                elif table_name == "node_groups":
                    r.setdefault("kind", "select")
                    r.setdefault("group_type", "select")
                    r.setdefault("sort_order", 0)
                    r.setdefault("regex_rules", "[]")
                    r.setdefault("filter_media_unlock", "[]")
                    r.setdefault("include_nodes", "[]")
                    r.setdefault("include_group_ids", "[]")
                    r.setdefault("include_group_nodes_ids", "[]")
                    r.setdefault("include_entries", "[]")
                    r.setdefault("add_fallback", False)
                    r.setdefault("exclude_nodes", "[]")
                    r.setdefault("exclude_group_ids", "[]")
                    r.setdefault("url_test_config", "{}")
                    r.setdefault("load_balance_config", "{}")
                    r.setdefault("fallback_config", "{}")
                norm_rows.append(r)

            cols = list(norm_rows[0].keys())
            placeholders = ", ".join("?" for _ in cols)
            col_names = ", ".join(cols)
            sql = f"INSERT INTO {table_name} ({col_names}) VALUES ({placeholders})"
            for r in norm_rows:
                values = [
                    json.dumps(r[c], ensure_ascii=False) if isinstance(r[c], (dict, list)) else r[c]
                    for c in cols
                ]
                conn.execute(sql, values)
            counts[table_name] = len(norm_rows)
        conn.commit()
        return counts
    finally:
        conn.close()


def _get_table_counts(db_path: Path) -> dict[str, int]:
    conn = sqlite3.connect(db_path)
    try:
        tables = [
            r[0]
            for r in conn.execute(
                "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'alembic_version'"
            ).fetchall()
        ]
        return {
            tbl: conn.execute(f"SELECT COUNT(*) FROM {tbl}").fetchone()[0]
            for tbl in sorted(tables)
        }
    finally:
        conn.close()


def test_full_upgrade_preserves_all_legacy_records(tmp_path) -> None:
    """从无版本遗留库桥接并升级到最新 head，所有已有表的数据行数必须 100% 完整保留"""
    db_file = tmp_path / "legacy_full.sqlite"
    backup_file = tmp_path / "legacy_full_backup.sqlite"

    expected_counts = _init_populated_legacy_db(db_file)

    # 制作备份副本
    src_conn = sqlite3.connect(db_file)
    dst_conn = sqlite3.connect(backup_file)
    try:
        src_conn.backup(dst_conn)
    finally:
        dst_conn.close()
        src_conn.close()

    # 1 执行桥接
    bridge_result = bridge_legacy_sqlite_database(db_file, backup_file)
    assert bridge_result["revision"] == LEGACY_BRIDGE_HEAD

    # 2 升级到最新 head
    config = _alembic_config(db_file)
    command.upgrade(config, "head")

    # 3 校验每张表的数据行数未发生丢失
    after_counts = _get_table_counts(db_file)
    for table_name, expected_row_count in expected_counts.items():
        assert after_counts.get(table_name) == expected_row_count, (
            f"表 {table_name} 升级后行数不一致: 预期 {expected_row_count}，实际 {after_counts.get(table_name)}"
        )


def test_downgrade_and_reupgrade_preserves_rows(tmp_path) -> None:
    """执行降级再重新升级，所有遗留表记录依然保持完整"""
    db_file = tmp_path / "downgrade_reup.sqlite"
    backup_file = tmp_path / "downgrade_reup_backup.sqlite"

    expected_counts = _init_populated_legacy_db(db_file)

    src_conn = sqlite3.connect(db_file)
    dst_conn = sqlite3.connect(backup_file)
    try:
        src_conn.backup(dst_conn)
    finally:
        dst_conn.close()
        src_conn.close()

    bridge_legacy_sqlite_database(db_file, backup_file)

    config = _alembic_config(db_file)
    command.upgrade(config, "head")

    # 降级到 bridge 节点
    command.downgrade(config, LEGACY_BRIDGE_HEAD)
    mid_counts = _get_table_counts(db_file)
    for table_name, expected_row_count in expected_counts.items():
        assert mid_counts.get(table_name) == expected_row_count

    # 再次升级到 head
    command.upgrade(config, "head")
    final_counts = _get_table_counts(db_file)
    for table_name, expected_row_count in expected_counts.items():
        assert final_counts.get(table_name) == expected_row_count


def test_quarantine_records_deterministic_creation(tmp_path) -> None:
    """隔离记录表可按分类和原因确定性记录无法映射的遗留数据"""
    db_file = tmp_path / "quarantine_test.sqlite"
    config = _alembic_config(db_file)
    command.upgrade(config, "head")

    conn = sqlite3.connect(db_file)
    try:
        conn.execute(
            "INSERT INTO quarantine_records (category, source_table, source_record_id, payload_json, reason, created_at) "
            "VALUES (?, ?, ?, ?, ?, datetime('now'))",
            ("unmappable_legacy_node", "subscriptions", "101", json.dumps({"raw": "corrupted"}), "缺少必要协议字段"),
        )
        conn.commit()

        row = conn.execute("SELECT category, source_table, source_record_id, reason FROM quarantine_records").fetchone()
        assert row == ("unmappable_legacy_node", "subscriptions", "101", "缺少必要协议字段")
    finally:
        conn.close()

