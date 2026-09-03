from __future__ import annotations

import sqlite3

import pytest

from app.services.sqlite_shadow_migration import (
    ShadowMigrationError,
    ShadowValidationError,
    migrate_table_via_shadow,
)


def _init_db() -> sqlite3.Connection:
    conn = sqlite3.connect(":memory:")
    conn.execute("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)")
    conn.execute("INSERT INTO users VALUES (1, 'alice', 'alice@test.local')")
    conn.execute("INSERT INTO users VALUES (2, 'bob', 'bob@test.local')")
    conn.commit()
    return conn


def test_shadow_migration_succeeds_and_swaps_table() -> None:
    """成功执行影子表迁移并原子替换旧表"""
    conn = _init_db()
    try:
        create_sql = "CREATE TABLE __shadow_users (id INTEGER PRIMARY KEY, name TEXT, email TEXT, is_active INTEGER DEFAULT 1)"

        def copy_fn(c: sqlite3.Connection, src: str, dst: str) -> None:
            c.execute(f"INSERT INTO {dst} (id, name, email, is_active) SELECT id, name, email, 1 FROM {src}")

        def validate_fn(c: sqlite3.Connection, src: str, dst: str) -> None:
            src_count = c.execute(f"SELECT COUNT(*) FROM {src}").fetchone()[0]
            dst_count = c.execute(f"SELECT COUNT(*) FROM {dst}").fetchone()[0]
            if src_count != dst_count:
                raise ShadowValidationError("行数不一致")

        result = migrate_table_via_shadow(conn, "users", create_sql, copy_fn, validate_fn)
        assert result["status"] == "migrated"
        assert result["row_count"] == 2

        # 检查新表结构和数据
        rows = conn.execute("SELECT id, name, is_active FROM users ORDER BY id").fetchall()
        assert rows == [(1, "alice", 1), (2, "bob", 1)]
    finally:
        conn.close()


def test_forced_validation_failure_leaves_source_table_and_data_intact() -> None:
    """校验失败时保持源表与数据完全不变，且清理影子表"""
    conn = _init_db()
    try:
        before_rows = conn.execute("SELECT * FROM users ORDER BY id").fetchall()
        create_sql = "CREATE TABLE __shadow_users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)"

        def copy_fn(c: sqlite3.Connection, src: str, dst: str) -> None:
            c.execute(f"INSERT INTO {dst} SELECT id, name, email FROM {src}")

        def validate_fn(c: sqlite3.Connection, src: str, dst: str) -> None:
            raise ShadowValidationError("故意注入的校验失败")

        with pytest.raises(ShadowValidationError, match="故意注入的校验失败"):
            migrate_table_via_shadow(conn, "users", create_sql, copy_fn, validate_fn)

        # 验证源表与数据未受任何修改
        after_rows = conn.execute("SELECT * FROM users ORDER BY id").fetchall()
        assert after_rows == before_rows

        # 验证影子表已被清理
        tables = [r[0] for r in conn.execute("SELECT name FROM sqlite_master WHERE type = 'table'").fetchall()]
        assert "__shadow_users" not in tables
        assert "__backup_users" not in tables
        assert "users" in tables
    finally:
        conn.close()


def test_transform_failure_leaves_source_table_intact() -> None:
    """转换过程抛出异常时保持源表不变"""
    conn = _init_db()
    try:
        before_rows = conn.execute("SELECT * FROM users ORDER BY id").fetchall()
        create_sql = "CREATE TABLE __shadow_users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)"

        def copy_fn(c: sqlite3.Connection, src: str, dst: str) -> None:
            raise ValueError("转换逻辑异常")

        with pytest.raises(ShadowMigrationError, match="转换逻辑异常"):
            migrate_table_via_shadow(conn, "users", create_sql, copy_fn)

        after_rows = conn.execute("SELECT * FROM users ORDER BY id").fetchall()
        assert after_rows == before_rows
    finally:
        conn.close()


def test_missing_source_table_raises_error() -> None:
    """源表不存在时抛出 ShadowMigrationError"""
    conn = sqlite3.connect(":memory:")
    try:
        with pytest.raises(ShadowMigrationError, match="源表 nonexistent 不存在"):
            migrate_table_via_shadow(
                conn,
                "nonexistent",
                "CREATE TABLE __shadow_nonexistent (id INT)",
                lambda c, s, d: None,
            )
    finally:
        conn.close()
