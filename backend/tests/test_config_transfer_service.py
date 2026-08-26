"""config_transfer_service 的领域行为测试：直接打 service 层，不经过 HTTP。

覆盖导出敏感字段剥离、导入 token_hash 保留、整体回滚、重置与预检。
"""
from __future__ import annotations

import pytest
import pytest_asyncio
from fastapi import HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from app.database import Base
from app.models.dns import DnsConfig
from app.models.generate_config import GenerateConfig
from app.models.node_group import NodeGroup
from app.models.rule_category import RuleCategory
from app.models.security_settings import SecuritySettings
from app.services.config_transfer_service import (
    export_data,
    import_data,
    reset_data,
    validate_import_payload,
)


@pytest_asyncio.fixture()
async def db_session():
    engine = create_async_engine("sqlite+aiosqlite:///:memory:", future=True)
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    maker = async_sessionmaker(engine, expire_on_commit=False)
    async with maker() as session:
        yield session
    await engine.dispose()


def _seed(db) -> None:
    """一组带父子关系的最小数据：分组挂在分类下，外加 DNS 与安全配置。"""
    db.add(RuleCategory(name="cat1"))
    db.add(NodeGroup(name="grp1"))
    db.add(DnsConfig(id=1, raw_yaml="nameserver: 1.1.1.1", enabled=True))
    db.add(SecuritySettings(id=1, token_hash="h123", auth_enabled=True))
    db.add(GenerateConfig(id=1))


@pytest.mark.asyncio
async def test_export_strips_token_hash(db_session):
    _seed(db_session)
    await db_session.commit()

    data = await export_data(db_session)
    assert data["version"] == 1
    sec = data["tables"]["security_settings"]
    assert len(sec) == 1 and "token_hash" not in sec[0]
    assert data["tables"]["node_groups"][0]["name"] == "grp1"


@pytest.mark.asyncio
async def test_export_exclude_subscriptions(db_session):
    _seed(db_session)
    await db_session.commit()

    data = await export_data(db_session, include_subscriptions=False)
    assert "subscriptions" not in data["tables"]
    assert "node_groups" in data["tables"]


@pytest.mark.asyncio
async def test_import_roundtrip_preserves_token_hash(db_session):
    _seed(db_session)
    await db_session.commit()

    doc = await export_data(db_session)
    # 导入文档不含 token_hash，导入后现有凭据必须原样保留
    imported = await import_data(db_session, {"tables": doc["tables"]})
    assert imported["security_settings"] == 1

    row = await db_session.get(SecuritySettings, 1)
    assert row.token_hash == "h123"
    assert (await db_session.get(DnsConfig, 1)).raw_yaml == "nameserver: 1.1.1.1"
    assert len((await db_session.execute(select(NodeGroup))).scalars().all()) == 1


@pytest.mark.asyncio
async def test_import_rolls_back_on_bad_row(db_session):
    _seed(db_session)
    await db_session.commit()

    tables = {
        "node_groups": [{"name": None}],  # NOT NULL 违例
        "dns_config": [{"id": 1, "raw_yaml": "x", "enabled": True}],
    }
    with pytest.raises(HTTPException) as exc:
        await import_data(db_session, {"tables": tables})
    assert exc.value.status_code == 400
    # 整体回滚：dns_config 的行不应存在
    rows = (await db_session.execute(select(DnsConfig))).scalars().all()
    assert len(rows) == 1 and rows[0].raw_yaml == "nameserver: 1.1.1.1"


@pytest.mark.asyncio
async def test_reset_restores_initial_rows(db_session):
    _seed(db_session)
    await db_session.commit()

    await reset_data(db_session)
    for model in (NodeGroup, RuleCategory, DnsConfig, GenerateConfig, SecuritySettings):
        count = len((await db_session.execute(select(model))).scalars().all())
        if model in (DnsConfig, GenerateConfig, SecuritySettings):
            assert count == 1
        else:
            assert count == 0


def test_validate_import_payload_reports_errors():
    summary, errors = validate_import_payload({
        "node_groups": [{"name": "a"}],
        "bogus_table": [],
        "rules": "not-a-list",
    })
    assert summary == {"node_groups": 1}
    assert set(errors) == {"bogus_table", "rules"}
