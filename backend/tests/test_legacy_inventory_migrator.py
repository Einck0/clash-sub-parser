from __future__ import annotations

import pytest
import pytest_asyncio
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from tests.conftest import TestSession
from app.models.node import Node
from app.models.quarantine import QuarantineRecord
from app.models.subscription import Subscription
from app.services.legacy_inventory_migrator import migrate_legacy_subscriptions_to_inventory


@pytest_asyncio.fixture
async def async_session():
    async with TestSession() as session:
        yield session


@pytest.mark.asyncio
async def test_legacy_subscription_migration(async_session: AsyncSession):
    # 构造遗留数据
    sub1 = Subscription(
        name="遗留订阅1",
        url="https://sub.example.com/api?token=secret123456",
        update_interval=60,
        is_primary=True,
        enabled=True,
        source_nodes=[
            {
                "name": "香港 01",
                "type": "ss",
                "server": "1.1.1.1",
                "port": 443,
                "cipher": "aes-128-gcm",
                "password": "p1",
            }
        ],
        manual_nodes=[
            {
                "name": "备用节点",
                "type": "trojan",
                "server": "2.2.2.2",
                "port": 443,
                "password": "p2",
            }
        ],
        raw_nodes=[],
        fetch_failed_count=0,
        fetch_comments=[],
        filter_regex=[],
        filter_media_unlock=[],
        include_node_names=[],
        exclude_node_names=[],
        node_renames={},
    )
    sub2 = Subscription(
        name="损坏数据源",
        url="manual://nodes",
        update_interval=None,
        is_primary=False,
        enabled=True,
        source_nodes=[],
        manual_nodes=[
            {
                "name": "损坏节点",
                "type": "unknown",
                "server": "",
                "port": 0,
            }
        ],
        raw_nodes=[],
        fetch_failed_count=0,
        fetch_comments=[],
        filter_regex=[],
        filter_media_unlock=[],
        include_node_names=[],
        exclude_node_names=[],
        node_renames={},
    )
    async_session.add_all([sub1, sub2])
    await async_session.commit()

    report = await migrate_legacy_subscriptions_to_inventory(async_session)

    assert report["sources_migrated"] == 2
    assert report["revisions_created"] == 2
    assert report["nodes_migrated"] == 2
    assert report["nodes_quarantined"] == 1

    # 验证正常节点与来源关联
    nodes_res = await async_session.execute(select(Node))
    nodes = list(nodes_res.scalars().all())
    assert len(nodes) == 2

    # 验证隔离区记录
    q_res = await async_session.execute(select(QuarantineRecord))
    quarantined = list(q_res.scalars().all())
    assert len(quarantined) == 1
    assert quarantined[0].category == "unmappable_legacy_node"
    assert quarantined[0].payload_json.get("name") == "损坏节点"
