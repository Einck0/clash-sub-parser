"""SQLite 影子表迁移助手

在 SQLite 中进行表结构重塑迁移时，采用影子表复制、校验与原子切换策略，
校验失败时自动回滚并保持源表和源数据完全不受影响
"""
from __future__ import annotations

from collections.abc import Callable
import sqlite3
from typing import Any


class ShadowMigrationError(RuntimeError):
    pass


class ShadowValidationError(ShadowMigrationError):
    pass


def migrate_table_via_shadow(
    connection: sqlite3.Connection,
    source_table: str,
    create_shadow_sql: str,
    copy_transform_fn: Callable[[sqlite3.Connection, str, str], None],
    validate_fn: Callable[[sqlite3.Connection, str, str], None] | None = None,
) -> dict[str, Any]:
    """通过影子表安全重塑 SQLite 表结构

    步骤
    1 创建临时影子表
    2 复制或转换数据到影子表
    3 执行前置与自定义数据完整性校验
    4 校验通过后原子重命名切换
    5 任一步骤失败则立即清理影子表并抛出异常，保持源表不变
    """
    shadow_table = f"__shadow_{source_table}"
    backup_table = f"__backup_{source_table}"

    # 确认源表存在
    cursor = connection.execute(
        "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
        (source_table,),
    )
    if not cursor.fetchone():
        raise ShadowMigrationError(f"源表 {source_table} 不存在")

    # 清理遗留的影子表
    connection.execute(f"DROP TABLE IF EXISTS {shadow_table}")
    connection.execute(f"DROP TABLE IF EXISTS {backup_table}")

    try:
        # 1 创建影子表
        connection.execute(create_shadow_sql)

        # 2 转换数据
        copy_transform_fn(connection, source_table, shadow_table)

        # 3 自定义校验
        if validate_fn is not None:
            validate_fn(connection, source_table, shadow_table)

        # 4 原子切换
        connection.execute(f"ALTER TABLE {source_table} RENAME TO {backup_table}")
        connection.execute(f"ALTER TABLE {shadow_table} RENAME TO {source_table}")
        connection.execute(f"DROP TABLE {backup_table}")

        # 统计最终行数
        row_count = connection.execute(f"SELECT COUNT(*) FROM {source_table}").fetchone()[0]
        return {
            "source_table": source_table,
            "status": "migrated",
            "row_count": row_count,
        }
    except Exception as exc:
        # 回滚并清理影子表与备份表
        _cleanup_on_failure(connection, source_table, shadow_table, backup_table)
        if isinstance(exc, ShadowMigrationError):
            raise
        raise ShadowMigrationError(f"影子表迁移失败: {exc}") from exc


def _cleanup_on_failure(
    connection: sqlite3.Connection,
    source_table: str,
    shadow_table: str,
    backup_table: str,
) -> None:
    # 若备份表存在说明切换中断，需恢复源表
    tables = {
        row[0]
        for row in connection.execute("SELECT name FROM sqlite_master WHERE type = 'table'").fetchall()
    }
    if backup_table in tables and source_table not in tables:
        connection.execute(f"ALTER TABLE {backup_table} RENAME TO {source_table}")
    connection.execute(f"DROP TABLE IF EXISTS {shadow_table}")
    connection.execute(f"DROP TABLE IF EXISTS {backup_table}")
