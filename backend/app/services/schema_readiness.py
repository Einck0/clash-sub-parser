"""数据库版本与就绪状态检查服务

应用启动和 /ready 探针仅做只读版本检查，绝不执行 DDL 修改
"""
from __future__ import annotations

from pathlib import Path
import sqlite3
from typing import Any

from alembic.config import Config
from alembic.script import ScriptDirectory
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession


BACKEND_ROOT = Path(__file__).resolve().parents[2]


def get_target_head_revision() -> str:
    """获取当前代码库声明的最新 Alembic head revision"""
    config = Config(str(BACKEND_ROOT / "alembic.ini"))
    config.set_main_option("script_location", str(BACKEND_ROOT / "alembic"))
    script = ScriptDirectory.from_config(config)
    heads = script.get_heads()
    if not heads:
        raise RuntimeError("Alembic 迁移链未找到任何 head revision")
    return heads[0]


async def check_database_readiness(session: AsyncSession) -> dict[str, Any]:
    """异步检查数据库迁移版本与就绪状态"""
    target_revision = get_target_head_revision()

    # 检查 alembic_version 表是否存在
    try:
        result = await session.execute(text("SELECT version_num FROM alembic_version LIMIT 1"))
        row = result.fetchone()
        current_revision = str(row[0]) if row else None
    except Exception:
        # 表不存在说明数据库未初始化或处于遗留未迁移状态
        return {
            "ready": False,
            "status": "migration_required",
            "current_revision": None,
            "required_revision": target_revision,
            "detail": "数据库尚未建立 alembic_version 迁移元数据",
        }

    if current_revision != target_revision:
        return {
            "ready": False,
            "status": "migration_required",
            "current_revision": current_revision,
            "required_revision": target_revision,
            "detail": f"数据库版本 ({current_revision}) 落后于应用要求版本 ({target_revision})",
        }

    return {
        "ready": True,
        "status": "ready",
        "current_revision": current_revision,
        "required_revision": target_revision,
    }


def check_sqlite_readiness_readonly(database_path: Path | str) -> dict[str, Any]:
    """通过只读 SQLite 连接检查迁移就绪状态"""
    db_path = Path(database_path)
    target_revision = get_target_head_revision()

    try:
        conn = sqlite3.connect(f"{db_path.resolve().as_uri()}?mode=ro", uri=True)
    except sqlite3.Error as exc:
        return {
            "ready": False,
            "status": "unreachable",
            "current_revision": None,
            "required_revision": target_revision,
            "detail": f"无法以只读模式连接数据库: {exc}",
        }

    try:
        cursor = conn.execute(
            "SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'alembic_version'"
        )
        if not cursor.fetchone():
            return {
                "ready": False,
                "status": "migration_required",
                "current_revision": None,
                "required_revision": target_revision,
                "detail": "数据库尚未建立 alembic_version 迁移元数据",
            }

        row = conn.execute("SELECT version_num FROM alembic_version LIMIT 1").fetchone()
        current_revision = str(row[0]) if row else None
        if current_revision != target_revision:
            return {
                "ready": False,
                "status": "migration_required",
                "current_revision": current_revision,
                "required_revision": target_revision,
                "detail": f"数据库版本 ({current_revision}) 落后于应用要求版本 ({target_revision})",
            }

        return {
            "ready": True,
            "status": "ready",
            "current_revision": current_revision,
            "required_revision": target_revision,
        }
    finally:
        conn.close()
