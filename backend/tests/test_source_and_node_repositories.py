from __future__ import annotations

import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import AsyncSession

from tests.conftest import TestSession
from app.repositories.node_repository import NodeRepository
from app.repositories.source_repository import SourceRepository


@pytest_asyncio.fixture
async def async_session():
    async with TestSession() as session:
        yield session


@pytest.mark.asyncio
async def test_source_repository_crud(async_session: AsyncSession):
    # 创建源
    src = await SourceRepository.create_source(
        session=async_session,
        name="测试订阅源",
        kind="subscription",
        url="https://example.com/sub",
    )
    assert src.logical_id is not None
    assert src.name == "测试订阅源"
    assert src.kind == "subscription"

    # 查询源
    found = await SourceRepository.get_by_logical_id(async_session, src.logical_id)
    assert found is not None
    assert found.id == src.id

    # 创建修订
    rev1 = await SourceRepository.create_revision(
        session=async_session,
        source_id=src.id,
        status="active",
        payload_hash="hash_v1",
        node_count=10,
    )
    assert rev1.revision_id is not None
    assert rev1.status == "active"

    # 获取当前活动修订
    active_rev = await SourceRepository.get_active_revision(async_session, src.id)
    assert active_rev is not None
    assert active_rev.id == rev1.id

    # 创建并切换到新修订
    rev2 = await SourceRepository.create_revision(
        session=async_session,
        source_id=src.id,
        status="pending",
        payload_hash="hash_v2",
        node_count=12,
    )
    await SourceRepository.set_active_revision(async_session, src.id, rev2.id)

    # 验证修订状态更新
    active_rev2 = await SourceRepository.get_active_revision(async_session, src.id)
    assert active_rev2 is not None
    assert active_rev2.id == rev2.id

    old_rev = await SourceRepository.get_revision_by_id(async_session, rev1.revision_id)
    assert old_rev is not None
    assert old_rev.status == "superseded"


@pytest.mark.asyncio
async def test_node_identity_is_stable_across_renames(async_session: AsyncSession):
    payload = {"server": "1.2.3.4", "port": 443, "cipher": "aes-128-gcm", "password": "secret"}
    fingerprint = "fp_sha256_mock_001"

    # 首次入库
    node1, is_created = await NodeRepository.upsert_by_fingerprint(
        session=async_session,
        name="初始节点名",
        protocol="ss",
        server="1.2.3.4",
        port=443,
        normalized_payload=payload,
        payload_fingerprint=fingerprint,
    )
    assert is_created is True
    initial_logical_id = node1.logical_id
    assert initial_logical_id is not None

    # 重命名但指纹相同的节点再次入库
    node2, is_created2 = await NodeRepository.upsert_by_fingerprint(
        session=async_session,
        name="修改后的节点名",
        protocol="ss",
        server="1.2.3.4",
        port=443,
        normalized_payload=payload,
        payload_fingerprint=fingerprint,
    )
    assert is_created2 is False
    # 稳定 logical_id 与主键必须保持一致
    assert node2.logical_id == initial_logical_id
    assert node2.id == node1.id
    assert node2.name == "修改后的节点名"


@pytest.mark.asyncio
async def test_multi_source_links_to_same_canonical_node(async_session: AsyncSession):
    # 创建两个独立的订阅源
    src_a = await SourceRepository.create_source(async_session, name="源A")
    src_b = await SourceRepository.create_source(async_session, name="源B")

    rev_a = await SourceRepository.create_revision(async_session, src_a.id, "active", "hash_a", 1)
    rev_b = await SourceRepository.create_revision(async_session, src_b.id, "active", "hash_b", 1)

    # 相同 payload 的节点
    payload = {"server": "common.example.com", "port": 8443, "uuid": "abc-123"}
    fingerprint = "fp_common_node_01"

    node, _ = await NodeRepository.upsert_by_fingerprint(
        session=async_session,
        name="公共节点",
        protocol="vless",
        server="common.example.com",
        port=8443,
        normalized_payload=payload,
        payload_fingerprint=fingerprint,
    )

    # 两个源分别关联该节点（显示名各不相同）
    await NodeRepository.link_nodes_to_revision(
        async_session, rev_a.id, [(node.id, "源A展示名", 0)]
    )
    await NodeRepository.link_nodes_to_revision(
        async_session, rev_b.id, [(node.id, "源B展示名", 0)]
    )

    # 校验关联记录
    nodes_a = await NodeRepository.get_nodes_for_revision(async_session, rev_a.id)
    assert len(nodes_a) == 1
    assert nodes_a[0][0].logical_id == node.logical_id
    assert nodes_a[0][1].display_name == "源A展示名"

    nodes_b = await NodeRepository.get_nodes_for_revision(async_session, rev_b.id)
    assert len(nodes_b) == 1
    assert nodes_b[0][0].logical_id == node.logical_id
    assert nodes_b[0][1].display_name == "源B展示名"

    # 全局查询活动节点与来源追溯
    active_links = await NodeRepository.get_active_nodes_with_provenance(async_session)
    assert len(active_links) == 2
    sources_in_links = {link[3].name for link in active_links}
    assert sources_in_links == {"源A", "源B"}
