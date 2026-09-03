from __future__ import annotations

from collections.abc import AsyncGenerator
import logging

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker, create_async_engine
from sqlalchemy.orm import DeclarativeBase

from app.config import get_settings
from app.services.schema_readiness import check_database_readiness

logger = logging.getLogger(__name__)


class Base(DeclarativeBase):
    pass


settings = get_settings()


def _sqlite_connect_args(database_url: str) -> dict:
    if database_url.startswith("sqlite"):
        return {"timeout": 30}
    return {}


engine = create_async_engine(
    settings.database_url,
    echo=False,
    connect_args=_sqlite_connect_args(settings.database_url),
)
AsyncSessionLocal = async_sessionmaker(
    engine, class_=AsyncSession, expire_on_commit=False
)


async def get_db() -> AsyncGenerator[AsyncSession, None]:
    async with AsyncSessionLocal() as session:
        yield session


async def _configure_sqlite(conn) -> None:
    """启用 SQLite WAL 模式与繁忙超时"""
    if conn.dialect.name != "sqlite":
        return
    await conn.execute(text("PRAGMA journal_mode=WAL"))
    await conn.execute(text("PRAGMA busy_timeout=30000"))
    await conn.execute(text("PRAGMA synchronous=NORMAL"))


async def init_db() -> None:
    """应用启动时初始化数据库连接并检查迁移状态，绝不执行 DDL 修改"""
    async with engine.begin() as conn:
        await _configure_sqlite(conn)

    async with AsyncSessionLocal() as session:
        readiness = await check_database_readiness(session)
        if not readiness["ready"]:
            logger.warning(
                "数据库尚未迁移至应用所需版本: %s",
                readiness.get("detail", readiness["status"]),
            )
