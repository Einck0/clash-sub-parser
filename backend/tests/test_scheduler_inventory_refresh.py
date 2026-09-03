from __future__ import annotations

from unittest.mock import AsyncMock, patch
import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import AsyncSession

from tests.conftest import TestSession
from app.models.subscription import Subscription
from app.repositories.node_repository import NodeRepository
from app.repositories.source_repository import SourceRepository
from app.services.subscription_service import fetch_due_subscriptions


@pytest_asyncio.fixture
async def async_session():
    async with TestSession() as session:
        yield session


@pytest.mark.asyncio
async def test_scheduler_refresh_publishes_generation_and_preserves_on_failure(async_session: AsyncSession):
    # 创建遗留订阅用于调度抓取
    sub = Subscription(
        name="调度订阅源",
        url="https://sub.scheduled.invalid/nodes",
        update_interval=10,
        is_primary=True,
        enabled=True,
        source_nodes=[],
        manual_nodes=[],
        raw_nodes=[],
        fetch_failed_count=0,
        fetch_comments=[],
        filter_regex=[],
        filter_media_unlock=[],
        include_node_names=[],
        exclude_node_names=[],
        node_renames={},
        last_fetched_at=None,
    )
    async_session.add(sub)
    await async_session.commit()

    valid_yaml = """
proxies:
  - name: "定时香港01"
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: pass
"""

    # 1 模拟首次定时抓取成功
    with patch(
        "app.services.subscription_service._fetch_subscription_text",
        new=AsyncMock(return_value=(AsyncMock(headers={}), valid_yaml)),
    ):
        fetched = await fetch_due_subscriptions(async_session)
        assert fetched == 1

    # 验证新库存已生成活动修订
    src = await SourceRepository.get_by_name(async_session, "调度订阅源")
    assert src is not None
    active_rev1 = await SourceRepository.get_active_revision(async_session, src.id)
    assert active_rev1 is not None
    assert active_rev1.node_count == 1

    nodes1 = await NodeRepository.get_nodes_for_revision(async_session, active_rev1.id)
    assert len(nodes1) == 1
    assert nodes1[0][1].display_name == "定时香港01"

    # 2 模拟下一次抓取发生网络故障
    # 强制使 update_interval 满足到期条件
    sub.last_fetched_at = None
    async_session.add(sub)
    await async_session.commit()

    with patch(
        "app.services.subscription_service._fetch_subscription_text",
        side_effect=Exception("Connection Timeout"),
    ):
        fetched_err = await fetch_due_subscriptions(async_session)
        assert fetched_err == 0

    # 验证故障后活动修订与节点未被清空或删除
    active_rev2 = await SourceRepository.get_active_revision(async_session, src.id)
    assert active_rev2 is not None
    assert active_rev2.id == active_rev1.id
    assert active_rev2.node_count == 1

    nodes2 = await NodeRepository.get_nodes_for_revision(async_session, active_rev2.id)
    assert len(nodes2) == 1
    assert nodes2[0][1].display_name == "定时香港01"
