from __future__ import annotations

import pytest
import pytest_asyncio
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from tests.conftest import TestSession
from app.models.quarantine import QuarantineRecord
from app.repositories.node_repository import NodeRepository
from app.repositories.source_repository import SourceRepository
from app.services.source_refresh_reconciler import reconcile_source_refresh


@pytest_asyncio.fixture
async def async_session():
    async with TestSession() as session:
        yield session


@pytest.mark.asyncio
async def test_successful_refresh_publishes_active_generation(async_session: AsyncSession):
    src = await SourceRepository.create_source(async_session, name="订阅A", kind="subscription")
    await async_session.commit()

    yaml_content = """
proxies:
  - name: "HK-01"
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: pass1
  - name: "US-01"
    type: trojan
    server: 2.2.2.2
    port: 8443
    password: pass2
"""

    result = await reconcile_source_refresh(
        session=async_session,
        source_logical_id=src.logical_id,
        raw_content=yaml_content,
    )
    assert result["success"] is True
    assert result["node_count"] == 2

    # 验证活动修订已生成且关联 2 个节点
    active_rev = await SourceRepository.get_active_revision(async_session, src.id)
    assert active_rev is not None
    assert active_rev.revision_id == result["revision_id"]
    assert active_rev.node_count == 2

    nodes = await NodeRepository.get_nodes_for_revision(async_session, active_rev.id)
    assert len(nodes) == 2
    assert nodes[0][1].display_name == "HK-01"
    assert nodes[1][1].display_name == "US-01"


@pytest.mark.asyncio
async def test_failed_refresh_retains_previous_active_generation(async_session: AsyncSession):
    src = await SourceRepository.create_source(async_session, name="订阅B", kind="subscription")
    await async_session.commit()

    valid_yaml = """
proxies:
  - name: "SG-01"
    type: ss
    server: 3.3.3.3
    port: 443
    cipher: aes-128-gcm
    password: pass1
"""
    res1 = await reconcile_source_refresh(async_session, src.logical_id, raw_content=valid_yaml)
    assert res1["success"] is True
    initial_rev_id = res1["revision_id"]

    # 模拟抓取失败（返回错误或空内容）
    res2 = await reconcile_source_refresh(
        async_session,
        src.logical_id,
        raw_content=None,
        error="502 Bad Gateway",
    )
    assert res2["success"] is False
    assert res2["error"] == "502 Bad Gateway"

    # 模拟格式损坏内容
    res3 = await reconcile_source_refresh(
        async_session,
        src.logical_id,
        raw_content="corrupted invalid node binary garbage @@!!",
    )
    assert res3["success"] is False

    # 验证当前活动修订依然是最初成功的修订
    current_active = await SourceRepository.get_active_revision(async_session, src.id)
    assert current_active is not None
    assert current_active.revision_id == initial_rev_id
    assert current_active.status == "active"


@pytest.mark.asyncio
async def test_quarantine_record_for_malformed_node(async_session: AsyncSession):
    src = await SourceRepository.create_source(async_session, name="订阅C", kind="subscription")
    await async_session.commit()

    mixed_yaml = """
proxies:
  - name: "正常节点"
    type: ss
    server: 4.4.4.4
    port: 443
    cipher: aes-128-gcm
    password: pass
  - name: "坏节点无端口"
    type: ss
    server: 5.5.5.5
    port: 0
    cipher: aes-128-gcm
    password: pass
"""
    result = await reconcile_source_refresh(async_session, src.logical_id, raw_content=mixed_yaml)
    assert result["success"] is True
    assert result["node_count"] == 1

    # 验证隔离区记录
    q_res = await async_session.execute(select(QuarantineRecord))
    quarantined = list(q_res.scalars().all())
    assert len(quarantined) == 1
    assert quarantined[0].category == "unmappable_node"
    assert quarantined[0].payload_json.get("name") == "坏节点无端口"
