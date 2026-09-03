from __future__ import annotations

import sqlite3

from httpx import ASGITransport, AsyncClient
import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from app.database import _configure_sqlite
import app.main as app_main
from app.services.schema_readiness import (
    check_database_readiness,
    check_sqlite_readiness_readonly,
    get_target_head_revision,
)


@pytest.mark.asyncio
async def test_readiness_reports_migration_required_when_alembic_version_missing():
    """未创建 alembic_version 表时返回 migration_required"""
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    session_factory = async_sessionmaker(engine, expire_on_commit=False)

    async with session_factory() as session:
        report = await check_database_readiness(session)
        assert report["ready"] is False
        assert report["status"] == "migration_required"
        assert report["current_revision"] is None
        assert report["required_revision"] == get_target_head_revision()

    await engine.dispose()


@pytest.mark.asyncio
async def test_readiness_reports_migration_required_when_behind_head():
    """revision 落后于最新 head 时返回 migration_required"""
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as conn:
        await conn.execute(
            text("CREATE TABLE alembic_version (version_num VARCHAR(32) NOT NULL PRIMARY KEY)")
        )
        await conn.execute(
            text("INSERT INTO alembic_version VALUES ('csp_legacy_schema_baseline')")
        )

    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    async with session_factory() as session:
        report = await check_database_readiness(session)
        assert report["ready"] is False
        assert report["status"] == "migration_required"
        assert report["current_revision"] == "csp_legacy_schema_baseline"
        assert report["required_revision"] == get_target_head_revision()

    await engine.dispose()


@pytest.mark.asyncio
async def test_readiness_reports_ready_when_at_head():
    """数据库处于最新 head 时返回 ready"""
    target = get_target_head_revision()
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as conn:
        await conn.execute(
            text("CREATE TABLE alembic_version (version_num VARCHAR(32) NOT NULL PRIMARY KEY)")
        )
        await conn.execute(
            text(f"INSERT INTO alembic_version VALUES ('{target}')")
        )

    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    async with session_factory() as session:
        report = await check_database_readiness(session)
        assert report["ready"] is True
        assert report["status"] == "ready"
        assert report["current_revision"] == target

    await engine.dispose()


@pytest.mark.asyncio
async def test_ready_endpoint_returns_503_when_migration_required():
    """HTTP /ready 接口在需要迁移时返回 503 状态码"""
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    session_factory = async_sessionmaker(engine, expire_on_commit=False)

    async def override_db():
        async with session_factory() as session:
            yield session

    app_main.app.dependency_overrides[app_main.get_db] = override_db
    try:
        transport = ASGITransport(app=app_main.app)
        async with AsyncClient(transport=transport, base_url="http://test") as client:
            resp = await client.get("/ready")
            assert resp.status_code == 503
            body = resp.json()
            assert body["status"] == "migration_required"
    finally:
        app_main.app.dependency_overrides.pop(app_main.get_db, None)
        await engine.dispose()


def test_sqlite_readiness_readonly_helper(tmp_path):
    """只读 helper 检查文件数据库"""
    db_file = tmp_path / "test_ro.sqlite"
    conn = sqlite3.connect(db_file)
    target = get_target_head_revision()
    conn.execute("CREATE TABLE alembic_version (version_num VARCHAR(32) PRIMARY KEY)")
    conn.execute(f"INSERT INTO alembic_version VALUES ('{target}')")
    conn.commit()
    conn.close()

    report = check_sqlite_readiness_readonly(db_file)
    assert report["ready"] is True
    assert report["status"] == "ready"
    assert report["current_revision"] == target


@pytest.mark.asyncio
async def test_init_db_executes_no_schema_mutation():
    """验证 init_db 不会私自执行 create_all 或 ALTER TABLE"""
    engine = create_async_engine("sqlite+aiosqlite:///:memory:")
    async with engine.begin() as conn:
        await _configure_sqlite(conn)
        tables_before = await conn.run_sync(
            lambda sync_conn: set(
                row[0]
                for row in sync_conn.execute(
                    text("SELECT name FROM sqlite_master WHERE type = 'table'")
                ).fetchall()
            )
        )

    # 验证没有自动创建任何业务表
    assert "subscriptions" not in tables_before
    assert "node_groups" not in tables_before
    await engine.dispose()
